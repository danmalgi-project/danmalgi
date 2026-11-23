package manager

// 저장소 관례는 외부 `_test` 패키지지만, 이 파일은 예외로 내부 `package manager`를
// 쓴다. "조회가 shard를 만들지 않는다"는 불변식은 unexported `shards` map을 직접
// 관측해야만 검증할 수 있기 때문이다 (계획서 T6 참조).

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
)

func newTestManager() *SignalingManager {
	return NewSignalingManager(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// Save/Delete는 PeerConnection을 역참조하지 않으므로 wrapper의 PeerConnection에는
// nil을 넘겨도 안전하다.
func newTestWrapper(userId, channelId int64, deviceId string) *domain.PeerConnectionWrapper {
	return domain.NewPeerConnectionWrapper(nil, userId, channelId, deviceId)
}

func TestSaveRecordsCallStartedAtOnFirstJoin(t *testing.T) {
	m := newTestManager()
	channelId := int64(1)

	before := time.Now()
	m.Save(channelId, newTestWrapper(1, channelId, "device-1"))
	after := time.Now()

	startedAt, ok := m.GetCallStartedAt(channelId)
	if !ok {
		t.Fatal("GetCallStartedAt ok = false, want true after first join")
	}
	if startedAt.Before(before) || startedAt.After(after) {
		t.Fatalf("startedAt = %v, want between %v and %v", startedAt, before, after)
	}
}

func TestSecondJoinDoesNotResetCallStartedAt(t *testing.T) {
	m := newTestManager()
	channelId := int64(1)

	m.Save(channelId, newTestWrapper(1, channelId, "device-1"))
	first, ok := m.GetCallStartedAt(channelId)
	if !ok {
		t.Fatal("GetCallStartedAt ok = false after first join")
	}

	time.Sleep(2 * time.Millisecond)
	m.Save(channelId, newTestWrapper(2, channelId, "device-2"))

	second, ok := m.GetCallStartedAt(channelId)
	if !ok {
		t.Fatal("GetCallStartedAt ok = false after second join")
	}
	if !second.Equal(first) {
		t.Fatalf("startedAt changed on second join: first=%v second=%v, want unchanged", first, second)
	}
}

func TestCallStartedAtClearedWhenChannelEmpties(t *testing.T) {
	m := newTestManager()
	channelId := int64(1)

	w1 := newTestWrapper(1, channelId, "device-1")
	w2 := newTestWrapper(2, channelId, "device-2")
	m.Save(channelId, w1)
	m.Save(channelId, w2)

	m.Delete(channelId, w1)
	m.Delete(channelId, w2)

	if _, ok := m.GetCallStartedAt(channelId); ok {
		t.Fatal("GetCallStartedAt ok = true after channel emptied, want false")
	}
}

func TestRejoinAfterEmptyStartsNewSession(t *testing.T) {
	m := newTestManager()
	channelId := int64(1)

	w1 := newTestWrapper(1, channelId, "device-1")
	m.Save(channelId, w1)
	firstSession, ok := m.GetCallStartedAt(channelId)
	if !ok {
		t.Fatal("GetCallStartedAt ok = false after first join")
	}

	m.Delete(channelId, w1)
	if _, ok := m.GetCallStartedAt(channelId); ok {
		t.Fatal("GetCallStartedAt ok = true after channel emptied, want false")
	}

	time.Sleep(2 * time.Millisecond)
	m.Save(channelId, newTestWrapper(3, channelId, "device-3"))

	secondSession, ok := m.GetCallStartedAt(channelId)
	if !ok {
		t.Fatal("GetCallStartedAt ok = false after rejoin")
	}
	if !secondSession.After(firstSession) {
		t.Fatalf("secondSession = %v, want after firstSession = %v", secondSession, firstSession)
	}
}

// Gets/GetTracks는 getOrCreateShard를 타므로 존재하지 않는 channelId를 조회하면
// shard가 새로 생긴다 (기존 동작). GetCallStartedAt은 그와 대비되게 조회 전용이어야
// 하며, shard를 만들지 않아야 임의 channel_id 조회로 shards map이 무한 성장하는
// 것을 막을 수 있다.
func TestGetCallStartedAtDoesNotCreateShard(t *testing.T) {
	m := newTestManager()

	if _, ok := m.GetCallStartedAt(999); ok {
		t.Fatal("GetCallStartedAt ok = true for nonexistent channel, want false")
	}

	m.mu.RLock()
	shardCount := len(m.shards)
	m.mu.RUnlock()

	if shardCount != 0 {
		t.Fatalf("len(m.shards) = %d after querying nonexistent channel, want 0", shardCount)
	}
}

// 마지막 참가자가 Delete로 빠지면 shard가 s.shards에서 제거되지만, 실제 teardown
// 경로(pion 콜백 → Gets/GetTracks/RemoveTrack)는 이 직후에도 getOrCreateShard를
// 타는 메서드를 호출할 수 있어 빈 shard를 map에 되살릴 수 있다. 이 테스트는 그
// "shard는 있지만 참가자는 0명" 상태를 Gets로 직접 재현해, GetCallStartedAt이
// 여전히 ok == false를 반환함을 확인한다 (리뷰 M-1: 이 가드를 지워도 기존 5개
// 테스트만으로는 전 스위트가 녹색이었다 - map에 shard가 없는 케이스만 다뤘기 때문).
func TestGetCallStartedAtIgnoresResurrectedEmptyShard(t *testing.T) {
	m := newTestManager()
	channelId := int64(1)

	w := newTestWrapper(1, channelId, "device-1")
	m.Save(channelId, w)
	m.Delete(channelId, w)

	// Gets는 getOrCreateShard를 타므로, 방금 지워진 channelId를 조회하면 빈 shard가
	// map에 부활한다 - teardown 중 RemoveTrack/Gets가 호출될 때와 동일한 상황이다.
	m.Gets(channelId)

	m.mu.RLock()
	shardCount := len(m.shards)
	m.mu.RUnlock()
	if shardCount != 1 {
		t.Fatalf("len(m.shards) = %d after Gets on emptied channel, want 1 (shard resurrected)", shardCount)
	}

	if _, ok := m.GetCallStartedAt(channelId); ok {
		t.Fatal("GetCallStartedAt ok = true for resurrected empty shard, want false")
	}
}
