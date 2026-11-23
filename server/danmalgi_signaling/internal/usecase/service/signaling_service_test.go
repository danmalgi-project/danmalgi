package service_test

import (
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/manager"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/signaling"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/usecase/service"
	"github.com/pion/webrtc/v4"
)

// These tests cover the wiring inside SignalingService.SignalAnswer that
// resumes a renegotiation deferred by glare:
//
//	pending, err := wrapper.ApplyAnswer(answer)
//	...
//	if pending { go u.signalingevent.SendOffer(wrapper) }
//
// That branch is the last link of the chain the whole fanout fix hangs on: if
// it disappears, a peer that hit glare never gets another offer and its
// participant list freezes permanently. The signaling package's fanout tests
// cannot protect it — their browser-side harness reimplements the same resume
// step, so deleting the production branch leaves them green.
//
// The tests therefore drive the production entry point (SignalingService,
// built with real collaborators) and never call ApplyAnswer or SendOffer on
// the event/wrapper themselves.
//
// SignalingService.redis is left nil on purpose: the answer path never touches
// it, and standing up a redis client would only couple this test to
// infrastructure it does not exercise.

const (
	testChannelID = int64(100)
	testUserID    = int64(1)

	// offerWait is deliberately generous: the resumed offer is produced by a
	// goroutine that has to run CreateOffer/SetLocalDescription. Waiting on a
	// channel (never polling) means a healthy run finishes in milliseconds and
	// only a genuine regression pays the full timeout.
	offerWait = 10 * time.Second

	// quietWait is how long we watch for an offer that must NOT appear. A slow
	// machine cannot turn this into a false failure — only an offer that
	// should never have been produced can.
	quietWait = 500 * time.Millisecond
)

// ---- harness -----------------------------------------------------------

type answerHarness struct {
	svc              *service.SignalingService
	signalingManager *manager.SignalingManager
	wrapper          *domain.PeerConnectionWrapper
	clientPC         *webrtc.PeerConnection

	// offers receives everything the wrapper enqueues. The Offer channel is
	// unbuffered, so a consumer must already be running before anything calls
	// SendOffer, otherwise the sender blocks forever.
	offers chan domain.Offer
}

func newAnswerHarness(t *testing.T) *answerHarness {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	signalingManager := manager.NewSignalingManager(logger)

	// The user usecase is only consulted from OnTrack, which these tests never
	// register or trigger.
	signalingEvent := signaling.NewSignalingEvent(signalingManager, nil, logger)

	svc, err := service.NewSignalingService(nil, signalingManager, signalingEvent)
	if err != nil {
		t.Fatalf("new signaling service: %v", err)
	}

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

	wrapper := domain.NewPeerConnectionWrapper(serverPC, testUserID, testChannelID, "device-a")
	signalingManager.Save(testChannelID, wrapper)

	h := &answerHarness{
		svc:              svc,
		signalingManager: signalingManager,
		wrapper:          wrapper,
		clientPC:         clientPC,
		offers:           make(chan domain.Offer, 8),
	}

	// Drain the wrapper's Offer channel for the lifetime of the test, standing
	// in for the handler's send goroutine. It never closes Offer — that
	// channel has multiple senders and closing it from the receiving side is
	// exactly the panic the wrapper's lifecycle rules forbid.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case offer := <-wrapper.Offer:
				select {
				case h.offers <- offer:
				case <-stop:
					return
				}
			case <-stop:
				return
			}
		}
	}()

	t.Cleanup(func() {
		// Close first: any fanout goroutine still parked in EnqueueOffer is
		// released by done, so wg.Wait cannot deadlock on a blocked sender.
		wrapper.Close()
		close(stop)
		wg.Wait()
		_ = serverPC.Close()
		_ = clientPC.Close()
	})

	return h
}

// addTrack registers a participant's track with the manager, which is where
// SendOffer recomputes the snapshot from at send time.
func (h *answerHarness) addTrack(t *testing.T, userID int64) {
	t.Helper()

	trackLocal, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		fmt.Sprintf("track-%d", userID),
		strconv.FormatInt(userID, 10),
	)
	if err != nil {
		t.Fatalf("track local for user %d: %v", userID, err)
	}

	h.signalingManager.AddTrack(testChannelID, &domain.UserTrack{
		User:       &domain.User{ID: userID, Name: fmt.Sprintf("user-%d", userID)},
		TrackType:  int(webrtc.RTPCodecTypeAudio),
		TrackLocal: trackLocal,
	})
}

// waitForOffer blocks on the offer channel instead of polling, so a healthy
// run costs no more than the goroutine handoff.
func (h *answerHarness) waitForOffer(t *testing.T, what string) domain.Offer {
	t.Helper()

	select {
	case offer := <-h.offers:
		return offer
	case <-time.After(offerWait):
		t.Fatalf("timed out after %s waiting for %s", offerWait, what)
		return domain.Offer{}
	}
}

func (h *answerHarness) expectNoOffer(t *testing.T, what string) {
	t.Helper()

	select {
	case offer := <-h.offers:
		t.Fatalf("unexpected offer while %s: roster=%v", what, rosterOf(offer))
	case <-time.After(quietWait):
	}
}

// answerFor drives the browser side through an offer and returns the answer it
// produced, in the string form the gRPC handler hands to SignalAnswer.
func (h *answerHarness) answerFor(t *testing.T, offer domain.Offer) (string, string) {
	t.Helper()

	if err := h.clientPC.SetRemoteDescription(offer.SessionDescription); err != nil {
		t.Fatalf("client set remote description: %v", err)
	}

	answer, err := h.clientPC.CreateAnswer(nil)
	if err != nil {
		t.Fatalf("client create answer: %v", err)
	}

	if err := h.clientPC.SetLocalDescription(answer); err != nil {
		t.Fatalf("client set local description: %v", err)
	}

	return answer.Type.String(), answer.SDP
}

func rosterOf(offer domain.Offer) []int64 {
	ids := make([]int64, 0, len(offer.UserTracks))
	for _, track := range offer.UserTracks {
		ids = append(ids, track.User.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func assertRoster(t *testing.T, offer domain.Offer, what string, want ...int64) {
	t.Helper()

	got := rosterOf(offer)
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })

	if len(got) != len(want) {
		t.Fatalf("%s: roster = %v, want %v", what, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: roster = %v, want %v", what, got, want)
		}
	}
}

// ---- tests -------------------------------------------------------------

// TestSignalAnswerResumesDeferredRenegotiation is the regression guard for the
// pending -> resume branch in SignalAnswer. Remove those three lines and this
// test fails: nothing else in the production path would ever emit the second
// offer, so the roster the client sees would stay stuck at the deferred state.
func TestSignalAnswerResumesDeferredRenegotiation(t *testing.T) {
	h := newAnswerHarness(t)

	// Two participants are already publishing, plus a track owned by the
	// receiver itself so the snapshot's self-exclusion is covered too.
	h.addTrack(t, testUserID)
	h.addTrack(t, 2)
	h.addTrack(t, 3)

	// Initial offer through the production entry point. This leaves the peer
	// connection in have-local-offer, awaiting its answer.
	if err := h.svc.SendOffer(h.wrapper); err != nil {
		t.Fatalf("initial SendOffer: %v", err)
	}

	first := h.waitForOffer(t, "the initial offer")
	assertRoster(t, first, "initial offer", 2, 3)

	if state := h.wrapper.PeerConnection.SignalingState(); state != webrtc.SignalingStateHaveLocalOffer {
		t.Fatalf("signaling state = %s, want have-local-offer before the fanout", state)
	}

	// A fourth participant joins while that offer is still in flight. This is
	// what addTrack's `go s.SendOffer(wrapper)` does; here it hits glare, so
	// Negotiate defers it and only sets the pending flag.
	h.addTrack(t, 4)
	if err := h.svc.SendOffer(h.wrapper); err != nil {
		t.Fatalf("fanout SendOffer: %v", err)
	}
	h.expectNoOffer(t, "the previous offer is still awaiting its answer")

	// The answer lands. SignalAnswer must notice the deferred renegotiation
	// and resume it.
	sdpType, sdp := h.answerFor(t, first)
	if err := h.svc.SignalAnswer(sdpType, sdp, h.wrapper); err != nil {
		t.Fatalf("SignalAnswer: %v", err)
	}

	resumed := h.waitForOffer(t, "the renegotiation resumed by SignalAnswer")

	// The resumed offer must carry the COMPLETE current roster, not just the
	// participant that triggered the deferred fanout: the snapshot is
	// recomputed at send time precisely so a dropped offer costs nothing.
	assertRoster(t, resumed, "resumed offer", 2, 3, 4)

	if resumed.SessionDescription.Type != webrtc.SDPTypeOffer {
		t.Errorf("resumed description type = %s, want offer", resumed.SessionDescription.Type)
	}
	if resumed.SessionDescription.SDP == "" {
		t.Error("resumed offer carries an empty SDP")
	}
}

// TestSignalAnswerWithoutDeferredRenegotiationSendsNothing pins the other half
// of the branch: an answer that did not race a fanout must not trigger an
// extra offer, otherwise every answer would kick off an endless
// offer/answer ping-pong.
func TestSignalAnswerWithoutDeferredRenegotiationSendsNothing(t *testing.T) {
	h := newAnswerHarness(t)

	h.addTrack(t, 2)

	if err := h.svc.SendOffer(h.wrapper); err != nil {
		t.Fatalf("initial SendOffer: %v", err)
	}

	first := h.waitForOffer(t, "the initial offer")
	assertRoster(t, first, "initial offer", 2)

	sdpType, sdp := h.answerFor(t, first)
	if err := h.svc.SignalAnswer(sdpType, sdp, h.wrapper); err != nil {
		t.Fatalf("SignalAnswer: %v", err)
	}

	h.expectNoOffer(t, "no renegotiation was deferred")

	if state := h.wrapper.PeerConnection.SignalingState(); state != webrtc.SignalingStateStable {
		t.Fatalf("signaling state = %s, want stable after the answer was applied", state)
	}
}
