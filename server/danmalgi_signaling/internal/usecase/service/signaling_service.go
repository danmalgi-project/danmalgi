package service

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/manager"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/signaling"
	"github.com/pion/webrtc/v4"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
)

type SignalingService struct {
	redis            *redis.Client
	signalingManager *manager.SignalingManager
	signalingevent   *signaling.SignalingEvent
}

func NewSignalingService(
	redis *redis.Client,
	signalingManager *manager.SignalingManager,
	signalingevent *signaling.SignalingEvent,
) (*SignalingService, error) {
	return &SignalingService{
		redis:            redis,
		signalingManager: signalingManager,
		signalingevent:   signalingevent,
	}, nil
}

func (u *SignalingService) PreSignaling(ctx context.Context, userId, channelId int64, deviceId string) (*domain.PeerConnectionWrapper, error) {
	peerConnection, err := u.createPeerConnection()
	if err != nil {
		return nil, err
	}

	// add session
	wrapper := domain.NewPeerConnectionWrapper(peerConnection, userId, channelId, deviceId)
	u.registerEvent(wrapper, channelId)
	u.signalingManager.Save(channelId, wrapper)

	// redis 추가 로직
	u.saveJoinedUser(ctx, userId, channelId, deviceId)

	return wrapper, nil
}

func (u *SignalingService) PostSignaling(ctx context.Context, wrapper *domain.PeerConnectionWrapper, channelId int64) {
	key := fmt.Sprintf("channel:%d:voice", channelId)
	u.redis.HDel(ctx, key, wrapper.DeviceId)

	u.signalingManager.Delete(channelId, wrapper)
	wrapper.PeerConnection.Close()
	wrapper.Close()
}

// SendOffer renegotiates with a single peer, handing it the current
// participant snapshot. The handler uses it for the initial offer so it never
// has to reach into the signaling package directly.
func (u *SignalingService) SendOffer(wrapper *domain.PeerConnectionWrapper) error {
	return u.signalingevent.SendOffer(wrapper)
}

func (u *SignalingService) SignalAnswer(sdpType, sdp string, wrapper *domain.PeerConnectionWrapper) error {
	answer := webrtc.SessionDescription{
		Type: webrtc.NewSDPType(sdpType),
		SDP:  sdp,
	}

	pending, err := wrapper.ApplyAnswer(answer)
	if err != nil {
		return errors.WithStack(err)
	}

	// A renegotiation was deferred while this offer was in flight; the peer is
	// stable again, so resume it. It runs in its own goroutine because the
	// caller is the handler's recv loop and SendOffer's EnqueueOffer blocks on
	// the send goroutine — doing it inline would stall inbound signaling.
	if pending {
		go u.signalingevent.SendOffer(wrapper)
	}

	return nil
}

func (u *SignalingService) GetChannelCallStartedAt(channelId int64) (time.Time, bool) {
	return u.signalingManager.GetCallStartedAt(channelId)
}

func (*SignalingService) SignalCandidate(candidate, sdpMid string, sdpMLineIndex int32, wrapper *domain.PeerConnectionWrapper) error {
	uintSdpMLineIndex := uint16(sdpMLineIndex)
	return wrapper.PeerConnection.AddICECandidate(webrtc.ICECandidateInit{
		Candidate:     candidate,
		SDPMid:        &sdpMid,
		SDPMLineIndex: &uintSdpMLineIndex,
	})
}

func (u *SignalingService) saveJoinedUser(ctx context.Context, userId, channelId int64, deviceId string) error {
	key := fmt.Sprintf("channel:%d:voice", channelId)

	_, err := u.redis.Pipelined(ctx, func(p redis.Pipeliner) error {
		p.HSet(ctx, key, deviceId, userId)
		p.Expire(ctx, key, 2*time.Minute)
		return nil
	})

	return errors.WithStack(err)
}

func (s *SignalingService) createPeerConnection() (*webrtc.PeerConnection, error) {
	settingEngine := webrtc.SettingEngine{}
	settingEngine.SetEphemeralUDPPortRange(49000, 49010) // UDP 포트 범위

	settingEngine.SetNAT1To1IPs([]string{os.Getenv("NAT_IP")}, webrtc.ICECandidateTypeHost)

	api := webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))
	peerConnection, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	})

	if err != nil {
		return nil, errors.WithStack(err)
	}

	// add audio transceiver
	if _, err := peerConnection.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		return nil, errors.WithStack(err)
	}

	return peerConnection, nil
}

func (u *SignalingService) registerEvent(wrapper *domain.PeerConnectionWrapper, channelId int64) {
	wrapper.PeerConnection.OnICECandidate(u.signalingevent.OnIceCandidate(wrapper))
	wrapper.PeerConnection.OnTrack(u.signalingevent.OnTrack(channelId, wrapper))
	wrapper.PeerConnection.OnConnectionStateChange(u.signalingevent.OnConnectionStateChange())
	wrapper.PeerConnection.OnICEConnectionStateChange(u.signalingevent.OnICEConnectionStateChange())
}
