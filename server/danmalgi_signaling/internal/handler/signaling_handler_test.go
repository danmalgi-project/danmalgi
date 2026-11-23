package handler_test

import (
	"context"
	"testing"
	"time"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/handler"
	signalingv1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/public/signaling/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// stubSignalingUsecase는 usecase.SignalingUsecase를 만족하는 최소 스텁이다.
// GetChannelCallStartedAt만 값을 돌려주고 나머지 메서드는 이 테스트가 다루는
// 스트리밍 경로에서 쓰이지 않으므로 zero value/nil을 반환한다.
type stubSignalingUsecase struct {
	startedAt time.Time
	ok        bool

	gotChannelId int64
}

func (s *stubSignalingUsecase) PreSignaling(ctx context.Context, userId, channelId int64, deviceId string) (*domain.PeerConnectionWrapper, error) {
	return nil, nil
}
func (s *stubSignalingUsecase) PostSignaling(ctx context.Context, wrapper *domain.PeerConnectionWrapper, channelId int64) {
}
func (s *stubSignalingUsecase) SendOffer(wrapper *domain.PeerConnectionWrapper) error { return nil }
func (s *stubSignalingUsecase) SignalAnswer(sdpType, sdp string, wrapper *domain.PeerConnectionWrapper) error {
	return nil
}
func (s *stubSignalingUsecase) SignalCandidate(candidate, sdpMid string, sdpMLineIndex int32, wrapper *domain.PeerConnectionWrapper) error {
	return nil
}
func (s *stubSignalingUsecase) GetChannelCallStartedAt(channelId int64) (time.Time, bool) {
	s.gotChannelId = channelId
	return s.startedAt, s.ok
}

func TestSignalingHandler_GetChannelCallStartedAt_PassesChannelIdThrough(t *testing.T) {
	startedAt := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	stub := &stubSignalingUsecase{startedAt: startedAt, ok: true}
	h := handler.NewSignalingHandler(stub)

	resp, err := h.GetChannelCallStartedAt(context.Background(), &signalingv1.GetChannelCallStartedAtRequest{ChannelId: 42})

	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if stub.gotChannelId != 42 {
		t.Errorf("usecase received channelId = %d, want 42", stub.gotChannelId)
	}
	if resp.StartedAt == nil || !resp.StartedAt.AsTime().Equal(startedAt) {
		t.Errorf("StartedAt = %v, want %v", resp.StartedAt, startedAt)
	}
}

func TestSignalingHandler_GetChannelCallStartedAt_ZeroChannelIdIsInvalidArgument(t *testing.T) {
	stub := &stubSignalingUsecase{}
	h := handler.NewSignalingHandler(stub)

	_, err := h.GetChannelCallStartedAt(context.Background(), &signalingv1.GetChannelCallStartedAtRequest{ChannelId: 0})

	assertInvalidArgument(t, err)
}

func TestSignalingHandler_GetChannelCallStartedAt_NegativeChannelIdIsInvalidArgument(t *testing.T) {
	stub := &stubSignalingUsecase{}
	h := handler.NewSignalingHandler(stub)

	_, err := h.GetChannelCallStartedAt(context.Background(), &signalingv1.GetChannelCallStartedAtRequest{ChannelId: -1})

	assertInvalidArgument(t, err)
}

func assertInvalidArgument(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("err = nil, want InvalidArgument")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Fatalf("err = %v, want status InvalidArgument", err)
	}
}

// 통화 중이 아닌 경우는 에러가 아니다 - 빈 응답 + OK.
func TestSignalingHandler_GetChannelCallStartedAt_NotInCallReturnsOkWithNilStartedAt(t *testing.T) {
	stub := &stubSignalingUsecase{ok: false}
	h := handler.NewSignalingHandler(stub)

	resp, err := h.GetChannelCallStartedAt(context.Background(), &signalingv1.GetChannelCallStartedAtRequest{ChannelId: 42})

	if err != nil {
		t.Fatalf("err = %v, want nil when not in call", err)
	}
	if resp.StartedAt != nil {
		t.Errorf("StartedAt = %v, want nil when not in call", resp.StartedAt)
	}
}
