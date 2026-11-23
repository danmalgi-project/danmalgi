package domain

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
)

func TestWrapper_EnqueueAfterClose_NoPanicReturnsFalse(t *testing.T) {
	w := NewPeerConnectionWrapper(nil, 1, 100, "device-a")
	w.Close()

	if ok := w.EnqueueOffer(Offer{}); ok {
		t.Error("EnqueueOffer after Close should return false")
	}
	if ok := w.EnqueueCandidate(&webrtc.ICECandidate{}); ok {
		t.Error("EnqueueCandidate after Close should return false")
	}
	if ok := w.EnqueueErr(errors.New("boom")); ok {
		t.Error("EnqueueErr after Close should return false")
	}
}

func TestWrapper_EnqueueDelivers(t *testing.T) {
	w := NewPeerConnectionWrapper(nil, 1, 100, "device-a")

	received := make(chan Offer, 1)
	go func() {
		received <- <-w.Offer
	}()

	offer := Offer{SessionDescription: webrtc.SessionDescription{SDP: "test-sdp"}}
	if ok := w.EnqueueOffer(offer); !ok {
		t.Fatal("EnqueueOffer should return true when a receiver is active")
	}

	select {
	case got := <-received:
		if got.SessionDescription.SDP != "test-sdp" {
			t.Errorf("delivered SDP = %q, want %q", got.SessionDescription.SDP, "test-sdp")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delivered offer")
	}
}

func TestWrapper_CloseIdempotent(t *testing.T) {
	w := NewPeerConnectionWrapper(nil, 1, 100, "device-a")

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Close panicked on repeated calls: %v", r)
		}
	}()

	w.Close()
	w.Close()
	w.Close()
}

func TestWrapper_ConcurrentEnqueueAndClose(t *testing.T) {
	w := NewPeerConnectionWrapper(nil, 1, 100, "device-a")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.EnqueueErr(errors.New("boom"))
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		w.Close()
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out: goroutines did not complete (possible deadlock or send-on-closed-channel panic)")
	}
}

// ---- renegotiation serialization --------------------------------------

// newNegotiationPair returns a wrapper backed by a real peer connection plus a
// browser-like peer on the other end, so the negotiation tests exercise pion's
// actual signaling state machine instead of a stub.
func newNegotiationPair(t *testing.T) (*PeerConnectionWrapper, *webrtc.PeerConnection) {
	t.Helper()

	serverPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("server pc: %v", err)
	}
	if _, err := serverPC.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		t.Fatalf("recvonly transceiver: %v", err)
	}

	clientPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("client pc: %v", err)
	}

	t.Cleanup(func() {
		_ = serverPC.Close()
		_ = clientPC.Close()
	})

	return NewPeerConnectionWrapper(serverPC, 1, 100, "device-a"), clientPC
}

// createOffer is what production passes to Negotiate: CreateOffer +
// SetLocalDescription, which leaves the peer connection in have-local-offer.
func createOffer(w *PeerConnectionWrapper) func() error {
	return func() error {
		sdp, err := w.PeerConnection.CreateOffer(nil)
		if err != nil {
			return err
		}
		return w.PeerConnection.SetLocalDescription(sdp)
	}
}

// answerFor drives the remote side through the offer currently pending on the
// wrapper and returns the answer it produced.
func answerFor(t *testing.T, w *PeerConnectionWrapper, clientPC *webrtc.PeerConnection) webrtc.SessionDescription {
	t.Helper()

	local := w.PeerConnection.LocalDescription()
	if local == nil {
		t.Fatal("no local description to answer")
	}
	if err := clientPC.SetRemoteDescription(*local); err != nil {
		t.Fatalf("client SetRemoteDescription: %v", err)
	}

	answer, err := clientPC.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("client CreateAnswer: %v", err)
	}
	if err := clientPC.SetLocalDescription(answer); err != nil {
		t.Fatalf("client SetLocalDescription: %v", err)
	}

	return answer
}

func TestWrapper_NegotiateDefersWhileOfferIsInFlight(t *testing.T) {
	w, _ := newNegotiationPair(t)

	sent, err := w.Negotiate(createOffer(w))
	if err != nil {
		t.Fatalf("first Negotiate: %v", err)
	}
	if !sent {
		t.Fatal("first Negotiate should have sent: the peer connection starts stable")
	}

	prepared := false
	sent, err = w.Negotiate(func() error {
		prepared = true
		return nil
	})
	if err != nil {
		t.Fatalf("deferred Negotiate must not be an error: %v", err)
	}
	if sent {
		t.Error("Negotiate should defer while the previous offer awaits its answer")
	}
	if prepared {
		t.Error("prepare must not run when the signaling state is not stable")
	}
}

func TestWrapper_ApplyAnswerReportsPendingExactlyOnce(t *testing.T) {
	w, clientPC := newNegotiationPair(t)

	if sent, err := w.Negotiate(createOffer(w)); err != nil || !sent {
		t.Fatalf("first Negotiate: sent=%v err=%v", sent, err)
	}

	// a renegotiation arrives while the first offer is still in flight
	if sent, err := w.Negotiate(func() error { return nil }); err != nil || sent {
		t.Fatalf("second Negotiate: sent=%v err=%v, want deferred", sent, err)
	}

	pending, err := w.ApplyAnswer(answerFor(t, w, clientPC))
	if err != nil {
		t.Fatalf("ApplyAnswer: %v", err)
	}
	if !pending {
		t.Fatal("ApplyAnswer must report the deferred renegotiation, otherwise it is never resumed")
	}

	// second round, this time with nothing deferred: the flag must not linger
	if sent, err := w.Negotiate(createOffer(w)); err != nil || !sent {
		t.Fatalf("third Negotiate: sent=%v err=%v", sent, err)
	}

	pending, err = w.ApplyAnswer(answerFor(t, w, clientPC))
	if err != nil {
		t.Fatalf("second ApplyAnswer: %v", err)
	}
	if pending {
		t.Error("pending must be consumed once; a stale flag causes a pointless extra offer")
	}
}

func TestWrapper_ApplyAnswerKeepsPendingOnFailure(t *testing.T) {
	w, _ := newNegotiationPair(t)

	if sent, err := w.Negotiate(createOffer(w)); err != nil || !sent {
		t.Fatalf("first Negotiate: sent=%v err=%v", sent, err)
	}
	if sent, err := w.Negotiate(func() error { return nil }); err != nil || sent {
		t.Fatalf("second Negotiate: sent=%v err=%v, want deferred", sent, err)
	}

	// a malformed answer must not swallow the deferral
	if _, err := w.ApplyAnswer(webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: "not-sdp"}); err == nil {
		t.Fatal("ApplyAnswer should fail on a malformed answer")
	}

	if !w.pendingNegotiation {
		t.Error("a failed ApplyAnswer must keep the pending flag, otherwise the deferred offer is lost")
	}
}

func TestWrapper_NegotiateSerializesPrepare(t *testing.T) {
	w, _ := newNegotiationPair(t)

	var inFlight atomic.Int32
	var overlapped atomic.Bool

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// prepare deliberately leaves the state stable so every call gets
			// through and we actually observe the mutual exclusion.
			_, _ = w.Negotiate(func() error {
				if inFlight.Add(1) > 1 {
					overlapped.Store(true)
				}
				time.Sleep(time.Millisecond)
				inFlight.Add(-1)
				return nil
			})
		}()
	}
	wg.Wait()

	if overlapped.Load() {
		t.Error("prepare ran concurrently: offer creation is not serialized")
	}
}

func TestWrapper_NegotiatePropagatesPrepareError(t *testing.T) {
	w, _ := newNegotiationPair(t)

	boom := errors.New("boom")
	sent, err := w.Negotiate(func() error { return boom })

	if sent {
		t.Error("Negotiate must not report a send when prepare failed")
	}
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want %v", err, boom)
	}
}
