package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	signalingv1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/public/signaling/v1"
	"github.com/danmalgi-project/danmalgi-webrtc-server/pkg/util/token"
	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
} // use default options

// Message은 브라우저(WS 클라이언트)와 어댑터 사이에서 주고받는 시그널링 이벤트다.
type Message struct {
	Event string `json:"event"`
	Value string `json:"data"`
}

// config는 어댑터 기동에 필요한 값들을 담는다. 값은 flag가 우선이고,
// flag가 비어 있으면 환경변수, 그마저도 없으면 기본값을 사용한다.
type config struct {
	grpcAddr   string
	listenAddr string
	channelID  string
	token      string
	jwtSecret  string
	tlsCert    string
	tlsKey     string
}

func loadConfig() (*config, error) {
	grpcAddrFlag := flag.String("grpc-addr", "", "signaling gRPC 서버 주소 (env GRPC_ADDR, 기본 localhost:8083)")
	listenAddrFlag := flag.String("listen-addr", "", "어댑터 HTTPS 리슨 주소 (env LISTEN_ADDR, 기본 :8888)")
	channelIDFlag := flag.String("channel-id", "", "시그널링 채널 ID (env CHANNEL_ID, 기본 0)")
	tokenFlag := flag.String("token", "", "signaling 서버 인증에 사용할 고정 JWT (env JWT_TOKEN, userId 쿼리 미전달 시 fallback)")
	jwtSecretFlag := flag.String("jwt-secret", "", "userId 기반 연결별 토큰 발급에 쓸 JWT_SECRET (env JWT_SECRET, 서버 .env와 동일해야 함)")
	tlsCertFlag := flag.String("tls-cert", "", "어댑터 HTTPS 인증서 경로 (env TLS_CERT, 기본 server.crt)")
	tlsKeyFlag := flag.String("tls-key", "", "어댑터 HTTPS 개인키 경로 (env TLS_KEY, 기본 server.key)")
	flag.Parse()

	cfg := &config{
		grpcAddr:   firstNonEmpty(*grpcAddrFlag, os.Getenv("GRPC_ADDR"), "localhost:8083"),
		listenAddr: firstNonEmpty(*listenAddrFlag, os.Getenv("LISTEN_ADDR"), ":8888"),
		channelID:  firstNonEmpty(*channelIDFlag, os.Getenv("CHANNEL_ID"), "0"),
		token:      firstNonEmpty(*tokenFlag, os.Getenv("JWT_TOKEN"), ""),
		jwtSecret:  firstNonEmpty(*jwtSecretFlag, os.Getenv("JWT_SECRET"), ""),
		tlsCert:    firstNonEmpty(*tlsCertFlag, os.Getenv("TLS_CERT"), "server.crt"),
		tlsKey:     firstNonEmpty(*tlsKeyFlag, os.Getenv("TLS_KEY"), "server.key"),
	}

	if cfg.token == "" && cfg.jwtSecret == "" {
		return nil, errors.New("JWT_TOKEN(고정 토큰) 또는 JWT_SECRET(userId 기반 발급) 중 하나가 필요합니다")
	}

	if _, err := strconv.ParseInt(cfg.channelID, 10, 64); err != nil {
		return nil, errors.Wrapf(err, "channel-id는 정수여야 합니다: %q", cfg.channelID)
	}

	return cfg, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	// token.Generate/Validate는 호출 시점에 os.Getenv("JWT_SECRET")을 읽으므로,
	// --jwt-secret 플래그로 넘어온 값도 반영되도록 env를 정규화한다 (gen-token과 동일 패턴).
	if cfg.jwtSecret != "" {
		if err := os.Setenv("JWT_SECRET", cfg.jwtSecret); err != nil {
			log.Fatal(err)
		}
	}

	http.Handle("/", http.FileServer(http.Dir("./")))
	http.HandleFunc("/signaling", func(w http.ResponseWriter, r *http.Request) {
		Signaling(w, r, cfg)
	})

	log.Printf("webrtc-adapter listening on %s (signaling gRPC: %s, channel: %s)", cfg.listenAddr, cfg.grpcAddr, cfg.channelID)
	if err := http.ListenAndServeTLS(cfg.listenAddr, cfg.tlsCert, cfg.tlsKey, nil); err != nil {
		log.Fatal(err)
	}
}

func Signaling(w http.ResponseWriter, r *http.Request, cfg *config) {
	webConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println(errors.WithStack(err))
		return
	}
	defer webConn.Close()

	userIDParam := r.URL.Query().Get("userId")
	authToken, err := resolveAuthToken(cfg, userIDParam, token.Generate)
	if err != nil {
		log.Println(errors.WithStack(err))
		return
	}

	conn, err := grpc.NewClient(
		cfg.grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Println(errors.WithStack(err))
		return
	}
	defer conn.Close()

	client := signalingv1.NewSignalingServiceClient(conn)

	md := metadata.New(map[string]string{
		"Authorization": "Bearer " + authToken,
		"channel_id":    cfg.channelID,
	})
	ctx := metadata.NewOutgoingContext(context.Background(), md)

	stream, err := client.Signaling(ctx)
	if err != nil {
		log.Println(errors.WithStack(err))
		return
	}
	defer stream.CloseSend()

	errChan := make(chan error)

	go func() {
		for {
			_, data, err := webConn.ReadMessage()
			if err != nil {
				errChan <- errors.WithStack(err)
				return
			}

			message := &Message{}
			if err := json.Unmarshal(data, message); err != nil {
				errChan <- errors.WithStack(err)
				return
			}

			request, err := buildSignalingRequest(message)
			if err != nil {
				errChan <- err
				return
			}

			if err := stream.Send(request); err != nil {
				errChan <- errors.WithStack(err)
				return
			}
		}
	}()

	go func() {
		for {
			response, err := stream.Recv()
			if err != nil {
				errChan <- errors.WithStack(err)
				return
			}

			message, err := buildWSMessage(response)
			if err != nil {
				errChan <- err
				return
			}

			if err := webConn.WriteJSON(message); err != nil {
				errChan <- errors.WithStack(err)
				return
			}
		}
	}()

	log.Println(<-errChan)
}

// resolveAuthToken은 요청의 userId 쿼리에 따라 signaling gRPC에 쓸 JWT를 고른다.
// userId가 오면 sign으로 연결별 토큰을 발급(deviceId="web-<userId>")하고,
// 없으면 cfg.token(고정 토큰)으로 하위호환 동작한다. sign은 token.Generate 주입용이다.
func resolveAuthToken(cfg *config, userIDParam string, sign func(userID int64, deviceID string) (string, error)) (string, error) {
	if userIDParam == "" {
		if cfg.token == "" {
			return "", errors.New("userId 쿼리가 없고 JWT_TOKEN도 설정되지 않았습니다")
		}
		return cfg.token, nil
	}

	userID, err := strconv.ParseInt(userIDParam, 10, 64)
	if err != nil {
		return "", errors.Wrapf(err, "userId는 정수여야 합니다: %q", userIDParam)
	}

	if cfg.jwtSecret == "" {
		return "", errors.New("userId 기반 토큰 발급에는 JWT_SECRET이 필요합니다")
	}

	return sign(userID, fmt.Sprintf("web-%d", userID))
}

// buildSignalingRequest는 브라우저에서 온 WS Message를 서버 signaling 계약의
// SignalingRequest로 변환한다. WS <-> gRPC 매핑을 담당하는 순수 함수로,
// 네트워크/전역 상태 없이 단위 테스트가 가능하다.
func buildSignalingRequest(message *Message) (*signalingv1.SignalingRequest, error) {
	switch message.Event {
	case "answer":
		var sessionDescription webrtc.SessionDescription
		if err := json.Unmarshal([]byte(message.Value), &sessionDescription); err != nil {
			return nil, errors.WithStack(err)
		}

		return &signalingv1.SignalingRequest{
			SignalingRequestInfo: &signalingv1.SignalingRequestInfo{
				Message: &signalingv1.SignalingRequestInfo_Answer{
					Answer: &signalingv1.Answer{
						Sdp: &signalingv1.SessionDescription{
							SdpType: sessionDescription.Type.String(),
							SdpData: sessionDescription.SDP,
						},
					},
				},
			},
		}, nil
	case "candidate":
		var candidate webrtc.ICECandidateInit
		if err := json.Unmarshal([]byte(message.Value), &candidate); err != nil {
			return nil, errors.WithStack(err)
		}

		var sdpMid string
		if candidate.SDPMid != nil {
			sdpMid = *candidate.SDPMid
		}

		var sdpMLineIndex int32
		if candidate.SDPMLineIndex != nil {
			sdpMLineIndex = int32(*candidate.SDPMLineIndex)
		}

		return &signalingv1.SignalingRequest{
			SignalingRequestInfo: &signalingv1.SignalingRequestInfo{
				Message: &signalingv1.SignalingRequestInfo_Candidate{
					Candidate: &signalingv1.Candidate{
						Candidate:     candidate.Candidate,
						SdpMid:        sdpMid,
						SdpMlineIndex: sdpMLineIndex,
					},
				},
			},
		}, nil
	default:
		return nil, errors.Errorf("지원하지 않는 이벤트입니다: %q", message.Event)
	}
}

// buildWSMessage는 서버로부터 받은 SignalingResponse를 브라우저용 WS Message로
// 변환한다. buildSignalingRequest와 대칭을 이루는 순수 함수다.
func buildWSMessage(response *signalingv1.SignalingResponse) (*Message, error) {
	info := response.GetSignalingResponseInfo()
	if info == nil {
		return nil, errors.New("signaling_response_info가 비어 있습니다")
	}

	switch payload := info.GetMessage().(type) {
	case *signalingv1.SignalingResponseInfo_Offer:
		if payload.Offer.GetSdp() == nil {
			return nil, errors.New("offer에 sdp가 없습니다")
		}
		offer := webrtc.SessionDescription{
			Type: webrtc.NewSDPType(payload.Offer.Sdp.SdpType),
			SDP:  payload.Offer.Sdp.SdpData,
		}

		value, err := json.Marshal(offer)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		return &Message{Event: "offer", Value: string(value)}, nil
	case *signalingv1.SignalingResponseInfo_Answer:
		if payload.Answer.GetSdp() == nil {
			return nil, errors.New("answer에 sdp가 없습니다")
		}
		answer := webrtc.SessionDescription{
			Type: webrtc.NewSDPType(payload.Answer.Sdp.SdpType),
			SDP:  payload.Answer.Sdp.SdpData,
		}

		value, err := json.Marshal(answer)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		return &Message{Event: "answer", Value: string(value)}, nil
	case *signalingv1.SignalingResponseInfo_Candidate:
		sdpMid := payload.Candidate.SdpMid
		sdpMLineIndex := uint16(payload.Candidate.SdpMlineIndex)
		candidate := webrtc.ICECandidateInit{
			Candidate:     payload.Candidate.Candidate,
			SDPMid:        &sdpMid,
			SDPMLineIndex: &sdpMLineIndex,
		}

		value, err := json.Marshal(candidate)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		return &Message{Event: "candidate", Value: string(value)}, nil
	default:
		return nil, errors.Errorf("지원하지 않는 응답입니다: %T", payload)
	}
}
