package signaling

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/manager"
	"github.com/pion/webrtc/v4"
	"github.com/pkg/errors"
)

type UserUsecase interface {
	GetUserByID(ctx context.Context, userID int64) (*domain.User, error)
}

type SignalingEvent struct {
	signalingManager *manager.SignalingManager
	userUsecase      UserUsecase
	logger           *slog.Logger
}

func NewSignalingEvent(
	signalingManager *manager.SignalingManager,
	userUsecase UserUsecase,
	logger *slog.Logger,
) *SignalingEvent {
	return &SignalingEvent{
		signalingManager: signalingManager,
		userUsecase:      userUsecase,
		logger:           logger,
	}
}

// log는 전역으로 빼는게 맞는거 같음
func (e *SignalingEvent) OnTrack(channelId int64, wrapper *domain.PeerConnectionWrapper) func(t *webrtc.TrackRemote, r *webrtc.RTPReceiver) {
	return func(t *webrtc.TrackRemote, r *webrtc.RTPReceiver) {
		trackLocal, err := webrtc.NewTrackLocalStaticRTP(t.Codec().RTPCodecCapability, t.ID(), strconv.Itoa(int(wrapper.UserId)))
		if err != nil {
			// log
			wrapper.EnqueueErr(errors.WithStack(err))
			return
		}

		user, err := e.userUsecase.GetUserByID(context.Background(), wrapper.UserId)
		if err != nil {
			// log
			wrapper.EnqueueErr(errors.WithStack(err))
			return
		}

		track := &domain.UserTrack{
			User:       user,
			TrackType:  int(t.Kind()),
			TrackLocal: trackLocal,
		}

		// 새로운 트랙 추가 함수
		go e.addTrack(channelId, track)
		// write stream
		go e.writeStream(channelId, track, t)
	}
}

func (s *SignalingEvent) writeStream(channelId int64, track *domain.UserTrack, t *webrtc.TrackRemote) {
	defer func() {
		// track 제거 로직
		s.removeTrack(channelId, track)
	}()

	rtpBuf := make([]byte, 1500)
	for {
		n, _, err := t.Read(rtpBuf)
		if err != nil {
			// log
			s.logger.Error(err.Error())
			return
		}

		if _, err := track.TrackLocal.Write(rtpBuf[:n]); err != nil {
			// log
			s.logger.Error(err.Error())
			return
		}
	}
}

// 같은 채널에서는 lock을 걸어야함
func (s *SignalingEvent) addTrack(channelId int64, track *domain.UserTrack) {
	s.signalingManager.AddTrack(channelId, track)

	tracks := s.signalingManager.GetTracks(channelId)
	wrappers := s.signalingManager.Gets(channelId)
	for _, wrapper := range wrappers {
		receivers := wrapper.PeerConnection.GetReceivers()
		senders := wrapper.PeerConnection.GetSenders()
		attached := attachedTrackIDs(receivers, senders)
		newTracks := filterUnattachedTracks(tracks, attached)

		// peer-local: 각 wrapper마다 새로 만들어야 다른 peer로 보내는 offer와
		// 트랙 목록이 섞이지 않고, go SendOffer로 넘긴 뒤에도 참조 캡처가 안전하다.
		added := make([]*domain.UserTrack, 0, len(newTracks))
		for _, newTrack := range newTracks {
			// echo 방지: 자기 트랙은 자기에게 되돌려주지 않는다. 지금까지는
			// attachedTrackIDs의 receiver 매칭에 암묵 의존했는데, receiver에
			// 트랙이 붙기 전 타이밍에는 자기 트랙이 새 트랙으로 보여 되돌아갔다.
			// snapshotForWrapper의 필터와 같은 기준(track.User.ID == wrapper.UserId)이다.
			if newTrack.User.ID == wrapper.UserId {
				continue
			}

			if _, err := wrapper.PeerConnection.AddTransceiverFromTrack(newTrack.TrackLocal, webrtc.RTPTransceiverInit{
				Direction: webrtc.RTPTransceiverDirectionSendonly,
			}); err != nil {
				s.logger.Error(err.Error())
				continue
			}

			added = append(added, newTrack)
		}

		// m-line 변화가 있을 때만 재협상한다. 명단 자체는 SendOffer가 전송
		// 직전에 매니저에서 다시 뜨므로, 여기서 스킵된 wrapper도 다음 offer에서
		// 완전한 명단을 받는다.
		if len(added) > 0 {
			go s.SendOffer(wrapper)
		}
	}
}

func (e *SignalingEvent) removeTrack(channelId int64, track *domain.UserTrack) {
	// 순서가 중요하다: ① 매니저에서 트랙 제거 → ② wrapper별 RemoveTrack →
	// ③ SendOffer. SendOffer가 전송 직전에 매니저에서 명단을 다시 뜨기 때문에,
	// 제거가 defer로 밀리면 퇴장자가 스냅샷에 그대로 남아버린다.
	e.signalingManager.RemoveTrack(channelId, track)

	wrappers := e.signalingManager.Gets(channelId)
	for _, wrapper := range wrappers {
		if track.User.ID == wrapper.UserId {
			continue
		}

		senders := wrapper.PeerConnection.GetSenders()

		isRemoved := false
		for _, sender := range senders {
			if sender.Track() == nil {
				continue
			}

			if track.TrackLocal.ID() != sender.Track().ID() {
				continue
			}

			if err := wrapper.PeerConnection.RemoveTrack(sender); err != nil {
				e.logger.Error(err.Error())
				continue
			}

			isRemoved = true
		}

		if isRemoved {
			go e.SendOffer(wrapper)
		}
	}
}

func (*SignalingEvent) OnConnectionStateChange() func(webrtc.PeerConnectionState) {
	return func(p webrtc.PeerConnectionState) {
		switch p {
		case webrtc.PeerConnectionStateConnected:
		case webrtc.PeerConnectionStateDisconnected:
			fallthrough
		case webrtc.PeerConnectionStateClosed:
			fallthrough
		case webrtc.PeerConnectionStateFailed:
		case webrtc.PeerConnectionStateConnecting:
		case webrtc.PeerConnectionStateUnknown:
		case webrtc.PeerConnectionStateNew:
		default:
		}
	}
}

func (*SignalingEvent) OnICEConnectionStateChange() func(webrtc.ICEConnectionState) {
	return func(is webrtc.ICEConnectionState) {
	}
}

func (*SignalingEvent) OnIceCandidate(wrapper *domain.PeerConnectionWrapper) func(*webrtc.ICECandidate) {
	return func(i *webrtc.ICECandidate) {
		if i == nil {
			return
		}

		wrapper.EnqueueCandidate(i)
	}
}
