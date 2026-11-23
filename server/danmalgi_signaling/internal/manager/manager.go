package manager

import (
	"log/slog"
	"sync"
	"time"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
	"github.com/pion/webrtc/v4"
)

type ChannelShard struct {
	wrappers []*domain.PeerConnectionWrapper
	tracks   []*domain.UserTrack
	mu       sync.RWMutex

	// callStartedAt은 이 채널 통화방의 세션 시작 시각(0→1 전이 시각)이다.
	// shard 생성 시점이 아니라 Save()에서 첫 참가자가 들어온 시점에만 기록한다 -
	// getOrCreateShard는 Gets/GetTracks/AddTrack 같은 읽기·부수 경로에서도 shard를
	// 만들기 때문에, shard 생성 시점을 기준으로 삼으면 조회만 발생한 빈 채널이
	// "통화 중"으로 오인된다.
	callStartedAt time.Time
}

type SignalingManager struct {
	shards map[int64]*ChannelShard
	mu     sync.RWMutex
	logger *slog.Logger
}

func NewSignalingManager(logger *slog.Logger) *SignalingManager {
	return &SignalingManager{
		shards: make(map[int64]*ChannelShard),
		logger: logger,
	}
}

func (s *SignalingManager) getOrCreateShard(channelId int64) *ChannelShard {
	s.mu.RLock()
	shard, ok := s.shards[channelId]
	s.mu.RUnlock()

	if !ok {
		s.mu.Lock()
		defer s.mu.Unlock()
		// Double check
		shard, ok = s.shards[channelId]
		if !ok {
			shard = &ChannelShard{
				wrappers: []*domain.PeerConnectionWrapper{},
				tracks:   []*domain.UserTrack{},
			}
			s.shards[channelId] = shard
		}
	}

	return shard
}

func (s *SignalingManager) Save(channelId int64, wrapper *domain.PeerConnectionWrapper) {
	shard := s.getOrCreateShard(channelId)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	if len(shard.wrappers) == 0 {
		shard.callStartedAt = time.Now()
	}
	shard.wrappers = append(shard.wrappers, wrapper)
	s.logger.Info("User connected", "channelId", channelId, "userId", wrapper.UserId, "deviceId", wrapper.DeviceId)
}

func (s *SignalingManager) Delete(channelId int64, targetWrapper *domain.PeerConnectionWrapper) {
	shard := s.getOrCreateShard(channelId)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	for i, wrapper := range shard.wrappers {
		if wrapper == targetWrapper {
			shard.wrappers = append(shard.wrappers[:i], shard.wrappers[i+1:]...)
			s.logger.Info("User disconnected", "channelId", channelId, "userId", targetWrapper.UserId, "deviceId", targetWrapper.DeviceId)
			if len(shard.wrappers) == 0 {
				// shard 자체를 map에서 지우므로 보통은 불필요하지만, 다른 goroutine이
				// 이미 들고 있는 stale shard 포인터에서도 "값 없음" 불변식이 유지되도록
				// 명시적으로 초기화한다.
				shard.callStartedAt = time.Time{}
				s.mu.Lock()
				delete(s.shards, channelId)
				s.mu.Unlock()
			}
			return
		}
	}
}

// GetCallStartedAt은 채널 통화방의 세션 시작 시각을 반환한다. 통화 중이 아니면
// (shard가 없거나 참가자가 0명이면) ok == false를 반환한다.
//
// getOrCreateShard를 의도적으로 쓰지 않는다 - 조회 경로가 shard를 만들면 임의의
// channel_id 조회마다 빈 shard가 shards map에 쌓여 메모리 누수/DoS 표면이 된다.
//
// len(shard.wrappers) == 0 가드는 "이중 안전장치"가 아니라 필수 방어다: 마지막
// 참가자가 Delete로 빠지면 shard가 s.shards에서 제거되지만, 그 직후 pion teardown
// 콜백(OnTrack 정리 등)이 Gets/GetTracks/RemoveTrack을 호출하면 이들이
// getOrCreateShard를 타 빈 shard를 map에 부활시킨다. 이 부활한 shard를 되돌려
// 지우는 주체가 없으므로, 이 가드가 없으면 "shard는 있지만 참가자가 0명"인 상태에서
// callStartedAt의 zero value(0001-01-01T00:00:00Z)가 그대로 "통화 중"으로
// 오인되어 클라이언트에 새어나간다.
func (s *SignalingManager) GetCallStartedAt(channelId int64) (time.Time, bool) {
	s.mu.RLock()
	shard, ok := s.shards[channelId]
	s.mu.RUnlock()
	if !ok {
		return time.Time{}, false
	}

	shard.mu.RLock()
	defer shard.mu.RUnlock()
	if len(shard.wrappers) == 0 {
		return time.Time{}, false
	}
	return shard.callStartedAt, true
}

func (s *SignalingManager) Gets(channelId int64) []*domain.PeerConnectionWrapper {
	shard := s.getOrCreateShard(channelId)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	// Return a copy to avoid race conditions on the slice
	c := make([]*domain.PeerConnectionWrapper, len(shard.wrappers))
	copy(c, shard.wrappers)
	return c
}

func (s *SignalingManager) GetPeerConnections(channelId int64) []*webrtc.PeerConnection {
	wrappers := s.Gets(channelId)
	peerConnections := make([]*webrtc.PeerConnection, len(wrappers))
	for i, wrapper := range wrappers {
		peerConnections[i] = wrapper.PeerConnection
	}
	return peerConnections
}

func (s *SignalingManager) AddTrack(channelId int64, track *domain.UserTrack) {
	shard := s.getOrCreateShard(channelId)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	shard.tracks = append(shard.tracks, track)
}

func (s *SignalingManager) GetTracks(channelId int64) []*domain.UserTrack {
	shard := s.getOrCreateShard(channelId)
	shard.mu.RLock()
	defer shard.mu.RUnlock()

	c := make([]*domain.UserTrack, len(shard.tracks))
	copy(c, shard.tracks)
	return c
}

func (s *SignalingManager) RemoveTrack(channelId int64, targetTrack *domain.UserTrack) {
	shard := s.getOrCreateShard(channelId)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	for i, track := range shard.tracks {
		if track == targetTrack {
			shard.tracks = append(shard.tracks[:i], shard.tracks[i+1:]...)
			return
		}
	}
}
