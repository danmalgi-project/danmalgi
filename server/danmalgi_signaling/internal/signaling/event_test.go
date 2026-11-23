package signaling

import (
	"testing"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
	"github.com/pion/webrtc/v4"
)

func newTestUserTrack(t *testing.T, trackID string) *domain.UserTrack {
	t.Helper()

	local, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, trackID, "stream")
	if err != nil {
		t.Fatalf("NewTrackLocalStaticRTP failed: %v", err)
	}

	return &domain.UserTrack{TrackLocal: local}
}

func TestFilterUnattachedTracks_EmptyAttached_ReturnsAll(t *testing.T) {
	tracks := []*domain.UserTrack{
		newTestUserTrack(t, "track-1"),
		newTestUserTrack(t, "track-2"),
	}

	got := filterUnattachedTracks(tracks, map[string]struct{}{})

	if len(got) != len(tracks) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(tracks))
	}
}

func TestFilterUnattachedTracks_AllAttached_ReturnsEmpty(t *testing.T) {
	t1 := newTestUserTrack(t, "track-1")
	t2 := newTestUserTrack(t, "track-2")
	tracks := []*domain.UserTrack{t1, t2}

	attached := map[string]struct{}{
		t1.TrackLocal.ID(): {},
		t2.TrackLocal.ID(): {},
	}

	got := filterUnattachedTracks(tracks, attached)

	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0", len(got))
	}
}

func TestFilterUnattachedTracks_PartialAttached_ReturnsOnlyUnattached(t *testing.T) {
	t1 := newTestUserTrack(t, "track-1")
	t2 := newTestUserTrack(t, "track-2")
	t3 := newTestUserTrack(t, "track-3")
	tracks := []*domain.UserTrack{t1, t2, t3}

	attached := map[string]struct{}{
		t2.TrackLocal.ID(): {},
	}

	got := filterUnattachedTracks(tracks, attached)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0] != t1 || got[1] != t3 {
		t.Fatalf("got = %v, want [t1, t3]", got)
	}
}

// TestFilterUnattachedTracks_ReturnedSliceIsIndependentOfInput guards against
// the original bug's root cause: a result slice that shares backing storage
// with a slice that keeps growing across loop iterations. filterUnattachedTracks
// must always allocate its own backing array so appends to the input slice
// (which has spare capacity here) never corrupt an already-returned result.
func TestFilterUnattachedTracks_ReturnedSliceIsIndependentOfInput(t *testing.T) {
	t1 := newTestUserTrack(t, "track-1")
	tracks := make([]*domain.UserTrack, 1, 4)
	tracks[0] = t1

	got := filterUnattachedTracks(tracks, map[string]struct{}{})

	tracks = append(tracks, newTestUserTrack(t, "track-2"), newTestUserTrack(t, "track-3"))

	if len(got) != 1 || got[0] != t1 {
		t.Fatalf("got was mutated by appending to the input slice: got = %v", got)
	}
}

func newTestUserTrackFor(t *testing.T, trackID string, userID int64) *domain.UserTrack {
	t.Helper()

	track := newTestUserTrack(t, trackID)
	track.User = &domain.User{ID: userID}

	return track
}

func TestSnapshotForWrapper_ExcludesOwnTracks(t *testing.T) {
	own := newTestUserTrackFor(t, "track-1", 1)
	other := newTestUserTrackFor(t, "track-2", 2)
	third := newTestUserTrackFor(t, "track-3", 3)

	got := snapshotForWrapper([]*domain.UserTrack{own, other, third}, 1)

	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0] != other || got[1] != third {
		t.Fatalf("got = %v, want [other, third]", got)
	}
}

// The snapshot contract requires an empty roster to be distinguishable from a
// missing one: convertOfferResponse turns a non-nil slice into a present (but
// empty) joined_user_tracks, which is how the last participant leaving reaches
// the client.
func TestSnapshotForWrapper_EmptyInputReturnsNonNilSlice(t *testing.T) {
	got := snapshotForWrapper(nil, 1)

	if got == nil {
		t.Fatal("snapshotForWrapper returned nil; an empty roster must still be a non-nil slice")
	}
	if len(got) != 0 {
		t.Fatalf("len(got) = %d, want 0", len(got))
	}

	own := newTestUserTrackFor(t, "track-1", 1)
	if got := snapshotForWrapper([]*domain.UserTrack{own}, 1); got == nil || len(got) != 0 {
		t.Fatalf("got = %v, want a non-nil empty slice when every track belongs to the receiver", got)
	}
}

func TestSnapshotForWrapper_ReturnedSliceIsIndependentOfInput(t *testing.T) {
	other := newTestUserTrackFor(t, "track-1", 2)
	tracks := make([]*domain.UserTrack, 1, 4)
	tracks[0] = other

	got := snapshotForWrapper(tracks, 1)

	tracks = append(tracks, newTestUserTrackFor(t, "track-2", 3))

	if len(got) != 1 || got[0] != other {
		t.Fatalf("got was mutated by appending to the input slice: got = %v", got)
	}
}
