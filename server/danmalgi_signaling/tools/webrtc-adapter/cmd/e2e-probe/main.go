// e2e-probe는 index.html 브라우저 클라이언트를 그대로 흉내내는 진단용 CLI다.
// N개의 참가자를 어댑터 WS(/signaling?userId=..)에 붙여 각자가 실제로 몇 개의
// 원격 트랙(=다른 참가자)을 수신하는지 센다. 로컬 수동 검증 재현용이며 빌드 산출물이 아니다.
package main

import (
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

type Message struct {
	Event string `json:"event"`
	Value string `json:"data"`
}

var anonymous bool

type probe struct {
	userID int
	mu     sync.Mutex
	tracks map[string]string // trackID -> streamID(=상대 userId)
}

func main() {
	addr := flag.String("addr", "wss://localhost:8888/signaling", "어댑터 WS 주소")
	count := flag.Int("count", 3, "참가자 수")
	baseID := flag.Int("base-user-id", 101, "첫 참가자 userId")
	stagger := flag.Duration("stagger", 3*time.Second, "참가자 간 접속 간격")
	hold := flag.Duration("hold", 20*time.Second, "마지막 참가자 접속 후 유지 시간")
	anon := flag.Bool("anonymous", false, "userId 쿼리 없이(=index.html에서 userId를 비운 채) 접속")
	flag.Parse()
	anonymous = *anon

	probes := make([]*probe, 0, *count)
	for i := 0; i < *count; i++ {
		p := &probe{userID: *baseID + i, tracks: map[string]string{}}
		probes = append(probes, p)
		if err := p.connect(*addr); err != nil {
			log.Fatalf("[user %d] 접속 실패: %v", p.userID, err)
		}
		log.Printf("[user %d] 접속 시작", p.userID)
		time.Sleep(*stagger)
	}

	time.Sleep(*hold)

	fmt.Println("\n===== 결과 =====")
	for _, p := range probes {
		p.mu.Lock()
		fmt.Printf("user %d: 원격 트랙 %d개 (기대 %d개)", p.userID, len(p.tracks), *count-1)
		for id, stream := range p.tracks {
			fmt.Printf("\n    - track=%s stream(owner userId)=%s", id, stream)
		}
		fmt.Println()
		p.mu.Unlock()
	}
}

// summarizeSDP는 offer의 m-line별 (mid, direction, msid)를 한 줄로 요약한다.
func summarizeSDP(sdp string) string {
	var out []string
	var cur []string
	flush := func() {
		if len(cur) > 0 {
			out = append(out, "{"+strings.Join(cur, " ")+"}")
			cur = nil
		}
	}
	for _, raw := range strings.Split(sdp, "\n") {
		line := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(line, "m="):
			flush()
			cur = []string{strings.Fields(line)[0]}
		case strings.HasPrefix(line, "a=mid:"):
			cur = append(cur, "mid="+strings.TrimPrefix(line, "a=mid:"))
		case strings.HasPrefix(line, "a=msid:"):
			cur = append(cur, "msid="+strings.TrimPrefix(line, "a=msid:"))
		case line == "a=sendonly" || line == "a=recvonly" || line == "a=sendrecv" || line == "a=inactive":
			cur = append(cur, strings.TrimPrefix(line, "a="))
		}
	}
	flush()
	return strings.Join(out, " ")
}

func (p *probe) connect(addr string) error {
	dialer := *websocket.DefaultDialer
	dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	url := fmt.Sprintf("%s?userId=%d", addr, p.userID)
	if anonymous {
		url = addr // userId 미입력(=index.html 기본 상태) → 고정 JWT_TOKEN fallback
	}
	ws, _, err := dialer.Dial(url, nil)
	if err != nil {
		return err
	}

	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		return err
	}

	pc.OnTrack(func(t *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		p.mu.Lock()
		p.tracks[t.ID()] = t.StreamID()
		n := len(p.tracks)
		p.mu.Unlock()
		log.Printf("[user %d] ONTRACK track=%s stream=%s (누적 %d)", p.userID, t.ID(), t.StreamID(), n)
	})

	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf("[user %d] pc state=%s", p.userID, s)
	})

	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		log.Printf("[user %d] ice state=%s", p.userID, s)
	})

	var wsMu sync.Mutex
	send := func(m Message) {
		wsMu.Lock()
		defer wsMu.Unlock()
		if err := ws.WriteJSON(m); err != nil {
			log.Printf("[user %d] ws write 실패: %v", p.userID, err)
		}
	}

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		b, _ := json.Marshal(c.ToJSON())
		send(Message{Event: "candidate", Value: string(b)})
	})

	// 마이크 대용 오디오 트랙 (서버 OnTrack을 띄우려면 실제 RTP가 흘러야 한다)
	track, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus},
		fmt.Sprintf("audio-%d", p.userID),
		fmt.Sprintf("stream-%d", p.userID),
	)
	if err != nil {
		return err
	}
	if _, err := pc.AddTrack(track); err != nil {
		return err
	}

	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		silence := make([]byte, 80)
		for range ticker.C {
			if err := track.WriteSample(media.Sample{Data: silence, Duration: 20 * time.Millisecond}); err != nil {
				return
			}
		}
	}()

	go func() {
		for {
			var m Message
			if err := ws.ReadJSON(&m); err != nil {
				log.Printf("[user %d] ws read 종료: %v", p.userID, err)
				return
			}

			switch m.Event {
			case "offer":
				var sdp webrtc.SessionDescription
				if err := json.Unmarshal([]byte(m.Value), &sdp); err != nil {
					log.Printf("[user %d] offer 파싱 실패: %v", p.userID, err)
					continue
				}
				log.Printf("[user %d] OFFER 수신 (state=%s) m-lines=%s", p.userID, pc.SignalingState(), summarizeSDP(sdp.SDP))
				if err := pc.SetRemoteDescription(sdp); err != nil {
					log.Printf("[user %d] SetRemoteDescription(offer) 실패: %v", p.userID, err)
					continue
				}
				answer, err := pc.CreateAnswer(nil)
				if err != nil {
					log.Printf("[user %d] CreateAnswer 실패: %v", p.userID, err)
					continue
				}
				if err := pc.SetLocalDescription(answer); err != nil {
					log.Printf("[user %d] SetLocalDescription(answer) 실패: %v", p.userID, err)
					continue
				}
				b, _ := json.Marshal(answer)
				send(Message{Event: "answer", Value: string(b)})
			case "candidate":
				var c webrtc.ICECandidateInit
				if err := json.Unmarshal([]byte(m.Value), &c); err != nil {
					continue
				}
				log.Printf("[user %d] 서버 candidate 수신: %s", p.userID, c.Candidate)
				if err := pc.AddICECandidate(c); err != nil {
					log.Printf("[user %d] AddICECandidate 실패: %v", p.userID, err)
				}
			}
		}
	}()

	return nil
}
