package signaling

import (
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
	"github.com/pion/webrtc/v4"
	"github.com/pkg/errors"
)

// SendOffer renegotiates with a single peer and hands it the full participant
// snapshot for its channel.
//
// The snapshot is recomputed from the manager at send time rather than passed
// in by the caller: that is what makes a failed or deferred offer harmless —
// whichever offer succeeds next carries the complete current roster instead of
// the delta that happened to be in flight when it failed.
func (s *SignalingEvent) SendOffer(wrapper *domain.PeerConnectionWrapper) error {
	var offer domain.Offer

	sent, err := wrapper.Negotiate(func() error {
		sdp, err := wrapper.PeerConnection.CreateOffer(nil)
		if err != nil {
			return errors.WithStack(err)
		}

		if err := wrapper.PeerConnection.SetLocalDescription(sdp); err != nil {
			return errors.WithStack(err)
		}

		tracks := s.signalingManager.GetTracks(wrapper.ChannelId)
		offer = domain.Offer{
			SessionDescription: sdp,
			UserTracks:         snapshotForWrapper(tracks, wrapper.UserId),
		}

		return nil
	})
	if err != nil {
		s.logger.Error("send offer failed", "userId", wrapper.UserId, "channelId", wrapper.ChannelId, "err", err)
		return err
	}

	// Deferred: a previous offer is still awaiting its answer. The pending
	// flag is set, so SignalAnswer resumes this renegotiation once that
	// answer lands.
	if !sent {
		return nil
	}

	// EnqueueOffer is deliberately outside the negotiation lock: the Offer
	// channel is unbuffered, so sending under the lock would pin the fanout
	// goroutine to the client's read speed. Ordering still holds because the
	// signaling state machine forbids a second offer before the first one is
	// answered.
	wrapper.EnqueueOffer(offer)

	return nil
}

// snapshotForWrapper returns every track in the channel except the receiver's
// own — the full participant list the offer must carry. The result is always
// non-nil (an empty channel roster must reach the client as an empty list, not
// as an absent field) and never shares storage with tracks.
func snapshotForWrapper(tracks []*domain.UserTrack, userId int64) []*domain.UserTrack {
	snapshot := make([]*domain.UserTrack, 0, len(tracks))

	for _, track := range tracks {
		if track.User.ID == userId {
			continue
		}

		snapshot = append(snapshot, track)
	}

	return snapshot
}

// attachedTrackIDs returns the set of track IDs already attached to a peer
// connection, whether as a receiver (incoming) or sender (outgoing) track.
func attachedTrackIDs(receivers []*webrtc.RTPReceiver, senders []*webrtc.RTPSender) map[string]struct{} {
	attached := make(map[string]struct{}, len(receivers)+len(senders))

	for _, receiver := range receivers {
		trackLocal := receiver.Track()
		if trackLocal == nil {
			continue
		}

		attached[trackLocal.ID()] = struct{}{}
	}

	for _, sender := range senders {
		trackLocal := sender.Track()
		if trackLocal == nil {
			continue
		}

		attached[trackLocal.ID()] = struct{}{}
	}

	return attached
}

// filterUnattachedTracks returns the subset of tracks whose ID is not in
// attached. It always allocates a fresh backing array, so the result never
// shares storage with tracks and is safe to hand off to a goroutine while
// the caller keeps mutating its own slices.
func filterUnattachedTracks(tracks []*domain.UserTrack, attached map[string]struct{}) []*domain.UserTrack {
	unattached := make([]*domain.UserTrack, 0, len(tracks))

	for _, track := range tracks {
		if _, ok := attached[track.TrackLocal.ID()]; ok {
			continue
		}

		unattached = append(unattached, track)
	}

	return unattached
}
