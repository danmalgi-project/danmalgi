package handler

import (
	"testing"
	"time"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
	signalingv1 "github.com/danmalgi-project/danmalgi-webrtc-server/pkg/gen/public/signaling/v1"
	"github.com/pion/webrtc/v4"
)

func newUserTrack(t *testing.T, userID int64, trackID string) *domain.UserTrack {
	t.Helper()

	local, err := webrtc.NewTrackLocalStaticRTP(webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus}, trackID, "stream")
	if err != nil {
		t.Fatalf("NewTrackLocalStaticRTP failed: %v", err)
	}

	return &domain.UserTrack{
		User:       &domain.User{ID: userID, Name: "user"},
		TrackType:  1,
		TrackLocal: local,
	}
}

func offerOf(t *testing.T, response *signalingv1.SignalingResponse) *signalingv1.Offer {
	t.Helper()

	message, ok := response.SignalingResponseInfo.Message.(*signalingv1.SignalingResponseInfo_Offer)
	if !ok {
		t.Fatalf("message = %T, want offer", response.SignalingResponseInfo.Message)
	}

	return message.Offer
}

// joined_user_tracks is a replace snapshot, so an empty roster must still be
// sent as a present-but-empty field. If it degraded to an absent field the
// client could not tell "no participants" from "no news" and would keep the
// last participant on screen forever.
func TestConvertOfferResponse_EmptySnapshotStillSetsJoinedUserTracks(t *testing.T) {
	for name, tracks := range map[string][]*domain.UserTrack{
		"empty slice": {},
		"nil slice":   nil,
	} {
		t.Run(name, func(t *testing.T) {
			offer := offerOf(t, convertOfferResponse(domain.Offer{
				SessionDescription: webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: "sdp"},
				UserTracks:         tracks,
			}))

			if offer.JoinedUserTracks == nil {
				t.Fatal("JoinedUserTracks is nil; an empty snapshot must still be delivered")
			}
			if got := len(offer.JoinedUserTracks.UserTracks); got != 0 {
				t.Fatalf("len(UserTracks) = %d, want 0", got)
			}
		})
	}
}

func TestConvertOfferResponse_MapsEverySnapshotEntry(t *testing.T) {
	response := convertOfferResponse(domain.Offer{
		SessionDescription: webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: "sdp"},
		UserTracks: []*domain.UserTrack{
			newUserTrack(t, 1, "track-1"),
			newUserTrack(t, 2, "track-2"),
		},
	})

	offer := offerOf(t, response)

	if offer.Sdp.SdpType != webrtc.SDPTypeOffer.String() || offer.Sdp.SdpData != "sdp" {
		t.Errorf("sdp = %+v, want type=offer data=sdp", offer.Sdp)
	}

	got := offer.JoinedUserTracks.GetUserTracks()
	if len(got) != 2 {
		t.Fatalf("len(UserTracks) = %d, want 2", len(got))
	}
	if got[0].User.Id != 1 || got[0].TrackId != "track-1" {
		t.Errorf("first entry = (user %d, track %q), want (1, track-1)", got[0].User.Id, got[0].TrackId)
	}
	if got[1].User.Id != 2 || got[1].TrackId != "track-2" {
		t.Errorf("second entry = (user %d, track %q), want (2, track-2)", got[1].User.Id, got[1].TrackId)
	}
}

// 통화 중이 아니면(ok == false) StartedAt 필드는 값 부재(nil)여야 한다.
// 항상 timestamppb.New로 채우면 "통화 중이 아님"과 "통화 시작 시각 0값"을
// 클라이언트가 구분할 수 없게 된다.
func TestConvertChannelCallStartedAtResponse_NotInCall(t *testing.T) {
	response := convertChannelCallStartedAtResponse(time.Time{}, false)

	if response.StartedAt != nil {
		t.Fatalf("StartedAt = %v, want nil when not in call", response.StartedAt)
	}
}

func TestConvertChannelCallStartedAtResponse_InCall(t *testing.T) {
	startedAt := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	response := convertChannelCallStartedAtResponse(startedAt, true)

	if response.StartedAt == nil {
		t.Fatal("StartedAt is nil, want set when in call")
	}
	if !response.StartedAt.AsTime().Equal(startedAt) {
		t.Errorf("StartedAt.AsTime() = %v, want %v", response.StartedAt.AsTime(), startedAt)
	}
}
