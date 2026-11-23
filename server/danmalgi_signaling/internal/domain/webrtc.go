package domain

import (
	"sync"

	"github.com/pion/webrtc/v4"
)

// PeerConnectionWrapper contains the peer connection and its metadata
type PeerConnectionWrapper struct {
	PeerConnection *webrtc.PeerConnection
	UserId         int64
	ChannelId      int64
	DeviceId       string
	Offer          chan Offer
	Candidate      chan *webrtc.ICECandidate
	Err            chan error

	// done and closeOnce encapsulate the wrapper's channel lifecycle.
	// Offer/Candidate/Err have multiple senders (handler/event goroutines),
	// so per Go convention they must never be closed directly — doing so
	// races with sends and panics with "send on closed channel". Instead,
	// Close closes done exactly once, and every Enqueue* call races the
	// send against done so it returns false instead of blocking or
	// panicking once the wrapper is torn down.
	done      chan struct{}
	closeOnce sync.Once

	// negotiationMu serializes renegotiation for this peer connection and
	// pendingNegotiation records that a renegotiation was requested while a
	// previous offer was still awaiting its answer (glare).
	//
	// Both Negotiate and ApplyAnswer take this SAME mutex on purpose. The
	// state check + CreateOffer + SetLocalDescription must be atomic, and so
	// must SetRemoteDescription + taking the pending flag. If either half ran
	// outside the lock there is a lost-wakeup window: Negotiate could observe
	// a non-stable state and set pendingNegotiation just after ApplyAnswer
	// already consumed the flag, so nobody would ever resume the deferred
	// offer and the participant list would freeze permanently.
	negotiationMu      sync.Mutex
	pendingNegotiation bool
}

type Offer struct {
	SessionDescription webrtc.SessionDescription
	UserTracks         []*UserTrack
}

func NewPeerConnectionWrapper(peerConnection *webrtc.PeerConnection, userId, channelId int64, deviceId string) *PeerConnectionWrapper {
	return &PeerConnectionWrapper{
		PeerConnection: peerConnection,
		UserId:         userId,
		ChannelId:      channelId,
		DeviceId:       deviceId,
		Offer:          make(chan Offer),
		Candidate:      make(chan *webrtc.ICECandidate),
		Err:            make(chan error),
		done:           make(chan struct{}),
	}
}

// Negotiate serializes offer creation for this peer connection. prepare runs
// under the negotiation lock ONLY when the signaling state is stable; it is
// expected to CreateOffer + SetLocalDescription (and to capture whatever
// payload the caller wants to send alongside the SDP).
//
// It returns false with a nil error when the offer was deferred because a
// previous offer is still awaiting its answer: the pending flag is set and
// ApplyAnswer will report it so the caller can resume. A deferred offer is
// not an error — dropping it is safe precisely because the payload is
// recomputed from scratch on the next successful negotiation.
func (w *PeerConnectionWrapper) Negotiate(prepare func() error) (bool, error) {
	w.negotiationMu.Lock()
	defer w.negotiationMu.Unlock()

	if w.PeerConnection.SignalingState() != webrtc.SignalingStateStable {
		w.pendingNegotiation = true
		return false, nil
	}

	if err := prepare(); err != nil {
		return false, err
	}

	return true, nil
}

// ApplyAnswer applies the remote answer under the negotiation lock and reports
// whether a renegotiation was deferred while that offer was in flight. Taking
// the pending flag inside the same critical section as SetRemoteDescription is
// what closes the lost-wakeup window described on negotiationMu. The flag is
// consumed only once: a second ApplyAnswer without an intervening deferral
// reports false. On failure the flag is left untouched so the deferral is not
// lost.
func (w *PeerConnectionWrapper) ApplyAnswer(sd webrtc.SessionDescription) (bool, error) {
	w.negotiationMu.Lock()
	defer w.negotiationMu.Unlock()

	if err := w.PeerConnection.SetRemoteDescription(sd); err != nil {
		return false, err
	}

	pending := w.pendingNegotiation
	w.pendingNegotiation = false

	return pending, nil
}

// EnqueueOffer sends an offer to the wrapper's Offer channel. It returns
// false without blocking or panicking if the wrapper has already been
// closed.
func (w *PeerConnectionWrapper) EnqueueOffer(o Offer) bool {
	select {
	case w.Offer <- o:
		return true
	case <-w.done:
		return false
	}
}

// EnqueueCandidate sends an ICE candidate to the wrapper's Candidate
// channel. It returns false without blocking or panicking if the wrapper
// has already been closed.
func (w *PeerConnectionWrapper) EnqueueCandidate(c *webrtc.ICECandidate) bool {
	select {
	case w.Candidate <- c:
		return true
	case <-w.done:
		return false
	}
}

// EnqueueErr sends an error to the wrapper's Err channel. It returns false
// without blocking or panicking if the wrapper has already been closed.
func (w *PeerConnectionWrapper) EnqueueErr(e error) bool {
	select {
	case w.Err <- e:
		return true
	case <-w.done:
		return false
	}
}

// Close marks the wrapper as done. Safe to call multiple times and from
// multiple goroutines; only the first call takes effect. After Close,
// every pending and future Enqueue* call returns false instead of
// blocking or panicking.
func (w *PeerConnectionWrapper) Close() {
	w.closeOnce.Do(func() {
		close(w.done)
	})
}

// type Track struct {
// 	UserId    int64
// 	TrackId   string
// 	TrackType int
// }
