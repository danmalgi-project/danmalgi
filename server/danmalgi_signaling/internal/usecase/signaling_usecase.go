package usecase

import (
	"context"
	"time"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
)

type SignalingUsecase interface {
	PreSignaling(ctx context.Context, userId, channelId int64, deviceId string) (*domain.PeerConnectionWrapper, error)
	PostSignaling(ctx context.Context, wrapper *domain.PeerConnectionWrapper, channelId int64)
	SendOffer(wrapper *domain.PeerConnectionWrapper) error
	SignalAnswer(sdpType, sdp string, wrapper *domain.PeerConnectionWrapper) error
	SignalCandidate(candidate, sdpMid string, sdpMLineIndex int32, wrapper *domain.PeerConnectionWrapper) error
	// GetChannelCallStartedAt은 채널 통화방의 세션 시작 시각을 반환한다. in-memory
	// 조회로 I/O·취소 처리가 없어 ctx를 받지 않는다 (SendOffer/SignalAnswer와 같은 관례).
	GetChannelCallStartedAt(channelId int64) (time.Time, bool)
}
