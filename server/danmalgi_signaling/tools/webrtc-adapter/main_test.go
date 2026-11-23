package main

import (
	"encoding/json"
	"testing"

	signalingv1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/public/signaling/v1"
)

func TestResolveAuthToken(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *config
		userIDParam string
		wantToken   string
		wantErr     bool
		wantSigned  bool
		wantUserID  int64
		wantDevice  string
	}{
		{
			name:        "userId 없음 + cfg.token 있음 -> cfg.token 반환, sign 미호출",
			cfg:         &config{token: "fixed-token"},
			userIDParam: "",
			wantToken:   "fixed-token",
			wantErr:     false,
			wantSigned:  false,
		},
		{
			name:        "userId 없음 + cfg.token 없음 -> 에러",
			cfg:         &config{},
			userIDParam: "",
			wantErr:     true,
			wantSigned:  false,
		},
		{
			name:        "userId=5 + jwtSecret 있음 -> sign(5, web-5) 호출, 반환값 전달",
			cfg:         &config{jwtSecret: "secret"},
			userIDParam: "5",
			wantToken:   "signed-token",
			wantErr:     false,
			wantSigned:  true,
			wantUserID:  5,
			wantDevice:  "web-5",
		},
		{
			name:        "userId=5 + jwtSecret 없음 -> 에러, sign 미호출",
			cfg:         &config{},
			userIDParam: "5",
			wantErr:     true,
			wantSigned:  false,
		},
		{
			name:        "userId=abc(비정수) -> 에러, sign 미호출",
			cfg:         &config{jwtSecret: "secret"},
			userIDParam: "abc",
			wantErr:     true,
			wantSigned:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signed := false
			var gotUserID int64
			var gotDevice string
			sign := func(userID int64, deviceID string) (string, error) {
				signed = true
				gotUserID = userID
				gotDevice = deviceID
				return "signed-token", nil
			}

			got, err := resolveAuthToken(tt.cfg, tt.userIDParam, sign)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (token=%q)", got)
				}
			} else if err != nil {
				t.Fatalf("resolveAuthToken returned unexpected error: %v", err)
			}

			if signed != tt.wantSigned {
				t.Errorf("sign called = %v, want %v", signed, tt.wantSigned)
			}

			if tt.wantSigned {
				if gotUserID != tt.wantUserID {
					t.Errorf("sign userID = %d, want %d", gotUserID, tt.wantUserID)
				}
				if gotDevice != tt.wantDevice {
					t.Errorf("sign deviceID = %q, want %q", gotDevice, tt.wantDevice)
				}
			}

			if !tt.wantErr && got != tt.wantToken {
				t.Errorf("token = %q, want %q", got, tt.wantToken)
			}
		})
	}
}

func TestBuildSignalingRequest_Answer(t *testing.T) {
	value, err := json.Marshal(map[string]string{
		"type": "answer",
		"sdp":  "v=0\r\n...answer-sdp...",
	})
	if err != nil {
		t.Fatalf("failed to marshal fixture: %v", err)
	}

	message := &Message{Event: "answer", Value: string(value)}

	request, err := buildSignalingRequest(message)
	if err != nil {
		t.Fatalf("buildSignalingRequest returned error: %v", err)
	}

	answer, ok := request.GetSignalingRequestInfo().GetMessage().(*signalingv1.SignalingRequestInfo_Answer)
	if !ok {
		t.Fatalf("expected SignalingRequestInfo_Answer, got %T", request.GetSignalingRequestInfo().GetMessage())
	}

	if got, want := answer.Answer.GetSdp().GetSdpType(), "answer"; got != want {
		t.Errorf("SdpType = %q, want %q", got, want)
	}
	if got, want := answer.Answer.GetSdp().GetSdpData(), "v=0\r\n...answer-sdp..."; got != want {
		t.Errorf("SdpData = %q, want %q", got, want)
	}
}

func TestBuildSignalingRequest_Candidate(t *testing.T) {
	sdpMid := "0"
	sdpMLineIndex := uint16(1)
	value, err := json.Marshal(struct {
		Candidate     string  `json:"candidate"`
		SDPMid        *string `json:"sdpMid"`
		SDPMLineIndex *uint16 `json:"sdpMLineIndex"`
	}{
		Candidate:     "candidate:1 1 UDP 2122260223 10.0.0.1 54321 typ host",
		SDPMid:        &sdpMid,
		SDPMLineIndex: &sdpMLineIndex,
	})
	if err != nil {
		t.Fatalf("failed to marshal fixture: %v", err)
	}

	message := &Message{Event: "candidate", Value: string(value)}

	request, err := buildSignalingRequest(message)
	if err != nil {
		t.Fatalf("buildSignalingRequest returned error: %v", err)
	}

	candidate, ok := request.GetSignalingRequestInfo().GetMessage().(*signalingv1.SignalingRequestInfo_Candidate)
	if !ok {
		t.Fatalf("expected SignalingRequestInfo_Candidate, got %T", request.GetSignalingRequestInfo().GetMessage())
	}

	if got, want := candidate.Candidate.GetCandidate(), "candidate:1 1 UDP 2122260223 10.0.0.1 54321 typ host"; got != want {
		t.Errorf("Candidate = %q, want %q", got, want)
	}
	if got, want := candidate.Candidate.GetSdpMid(), "0"; got != want {
		t.Errorf("SdpMid = %q, want %q", got, want)
	}
	if got, want := candidate.Candidate.GetSdpMlineIndex(), int32(1); got != want {
		t.Errorf("SdpMlineIndex = %d, want %d", got, want)
	}
}

func TestBuildSignalingRequest_UnsupportedEvent(t *testing.T) {
	if _, err := buildSignalingRequest(&Message{Event: "unknown"}); err == nil {
		t.Fatal("expected error for unsupported event, got nil")
	}
}

func TestBuildWSMessage_Offer(t *testing.T) {
	response := &signalingv1.SignalingResponse{
		SignalingResponseInfo: &signalingv1.SignalingResponseInfo{
			Message: &signalingv1.SignalingResponseInfo_Offer{
				Offer: &signalingv1.Offer{
					Sdp: &signalingv1.SessionDescription{
						SdpType: "offer",
						SdpData: "v=0\r\n...offer-sdp...",
					},
				},
			},
		},
	}

	message, err := buildWSMessage(response)
	if err != nil {
		t.Fatalf("buildWSMessage returned error: %v", err)
	}

	if got, want := message.Event, "offer"; got != want {
		t.Errorf("Event = %q, want %q", got, want)
	}

	var decoded map[string]string
	if err := json.Unmarshal([]byte(message.Value), &decoded); err != nil {
		t.Fatalf("failed to unmarshal Value: %v", err)
	}
	if got, want := decoded["type"], "offer"; got != want {
		t.Errorf("type = %q, want %q", got, want)
	}
	if got, want := decoded["sdp"], "v=0\r\n...offer-sdp..."; got != want {
		t.Errorf("sdp = %q, want %q", got, want)
	}
}

func TestBuildWSMessage_Candidate(t *testing.T) {
	response := &signalingv1.SignalingResponse{
		SignalingResponseInfo: &signalingv1.SignalingResponseInfo{
			Message: &signalingv1.SignalingResponseInfo_Candidate{
				Candidate: &signalingv1.Candidate{
					Candidate:     "candidate:1 1 UDP 2122260223 10.0.0.1 54321 typ host",
					SdpMid:        "0",
					SdpMlineIndex: 1,
				},
			},
		},
	}

	message, err := buildWSMessage(response)
	if err != nil {
		t.Fatalf("buildWSMessage returned error: %v", err)
	}

	if got, want := message.Event, "candidate"; got != want {
		t.Errorf("Event = %q, want %q", got, want)
	}

	var decoded struct {
		Candidate     string  `json:"candidate"`
		SDPMid        *string `json:"sdpMid"`
		SDPMLineIndex *uint16 `json:"sdpMLineIndex"`
	}
	if err := json.Unmarshal([]byte(message.Value), &decoded); err != nil {
		t.Fatalf("failed to unmarshal Value: %v", err)
	}
	if got, want := decoded.Candidate, "candidate:1 1 UDP 2122260223 10.0.0.1 54321 typ host"; got != want {
		t.Errorf("candidate = %q, want %q", got, want)
	}
	if decoded.SDPMid == nil || *decoded.SDPMid != "0" {
		t.Errorf("sdpMid = %v, want \"0\"", decoded.SDPMid)
	}
	if decoded.SDPMLineIndex == nil || *decoded.SDPMLineIndex != 1 {
		t.Errorf("sdpMLineIndex = %v, want 1", decoded.SDPMLineIndex)
	}
}

func TestBuildWSMessage_MissingInfo(t *testing.T) {
	if _, err := buildWSMessage(&signalingv1.SignalingResponse{}); err == nil {
		t.Fatal("expected error for missing signaling_response_info, got nil")
	}
}
