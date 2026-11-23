package signaling

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/domain"
	"github.com/danmalgi-project/danmalgi-webrtc-server/internal/manager"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
)

// These are integration-flavoured regression tests for issue #5: with three or
// more participants, joined_user_tracks used to arrive incomplete. They stand
// up real pion peer connections on both sides so the assertions cover the
// actual renegotiation state machine, not a stub of it.
//
// The contract under test: joined_user_tracks is a REPLACE snapshot of the
// whole channel minus the receiver, recomputed at send time. Every assertion
// therefore looks at a peer's *latest* offer on its own — asserting on the
// union of all offers it ever received would pass even under the old delta
// contract and hide the bug.

// ---- stubs -------------------------------------------------------------

type stubUserUsecase struct{}

func (stubUserUsecase) GetUserByID(_ context.Context, userID int64) (*domain.User, error) {
	return &domain.User{ID: userID, Name: fmt.Sprintf("user-%d", userID)}, nil
}

// ---- candidate buffering ----------------------------------------------

type candidateBuffer struct {
	mu      sync.Mutex
	pc      *webrtc.PeerConnection
	ready   bool
	pending []webrtc.ICECandidateInit
}

func (b *candidateBuffer) add(c webrtc.ICECandidateInit) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.ready {
		b.pending = append(b.pending, c)
		return
	}
	_ = b.pc.AddICECandidate(c)
}

func (b *candidateBuffer) markReady() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.ready = true
	for _, c := range b.pending {
		_ = b.pc.AddICECandidate(c)
	}
	b.pending = nil
}

// ---- test peer ---------------------------------------------------------

type testPeer struct {
	userID   int64
	wrapper  *domain.PeerConnectionWrapper
	serverPC *webrtc.PeerConnection
	clientPC *webrtc.PeerConnection

	mu                sync.Mutex
	offers            [][]int64           // joined_user_tracks (userIDs) per received offer
	remoteTracks      map[string]struct{} // track IDs the browser side actually got
	offersBeforeLeave int                 // snapshot of len(offers) taken before a peer leaves
}

// recordOffer stores the roster of every offer, including empty ones: under
// the snapshot contract "no participants left" is a meaningful message, so it
// must not be filtered out of the history the assertions look at.
func (p *testPeer) recordOffer(o domain.Offer) {
	p.mu.Lock()
	defer p.mu.Unlock()
	ids := make([]int64, 0, len(o.UserTracks))
	for _, ut := range o.UserTracks {
		ids = append(ids, ut.User.ID)
	}
	p.offers = append(p.offers, ids)
}

func (p *testPeer) offerCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.offers)
}

// latestSnapshot returns the roster carried by the most recent offer. This is
// the value the client's replace-semantics leaves on screen.
func (p *testPeer) latestSnapshot() []int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.offers) == 0 {
		return nil
	}
	latest := p.offers[len(p.offers)-1]
	out := make([]int64, len(latest))
	copy(out, latest)
	return out
}

func (p *testPeer) snapshotHistory() [][]int64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([][]int64, len(p.offers))
	copy(out, p.offers)
	return out
}

func (p *testPeer) remoteTrackCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.remoteTracks)
}

// ---- assertions --------------------------------------------------------

func logPeers(t *testing.T, peers []*testPeer) {
	t.Helper()
	for _, p := range peers {
		t.Logf("user %d: offers=%d snapshots(순서대로)=%v latest=%v remoteTracks=%d",
			p.userID, p.offerCount(), p.snapshotHistory(), p.latestSnapshot(), p.remoteTrackCount())
	}
}

// assertRosterConverged is the acceptance criterion of issue #5: once the room
// has settled, every participant's LATEST snapshot lists exactly the other
// participants — no self (echo), no duplicates, nobody missing.
func assertRosterConverged(t *testing.T, peers []*testPeer, userIDs []int64) {
	t.Helper()

	for _, p := range peers {
		latest := p.latestSnapshot()

		counts := map[int64]int{}
		for _, id := range latest {
			counts[id]++
		}

		for _, other := range userIDs {
			switch {
			case other == p.userID:
				if counts[other] > 0 {
					t.Errorf("user %d was told about ITSELF (echo): latest snapshot = %v", p.userID, latest)
				}
			case counts[other] == 0:
				t.Errorf("user %d's latest snapshot is missing user %d: %v", p.userID, other, latest)
			case counts[other] > 1:
				t.Errorf("user %d's latest snapshot lists user %d %d times: %v", p.userID, other, counts[other], latest)
			}
		}

		if want := len(userIDs) - 1; len(latest) != want {
			t.Errorf("user %d's latest snapshot has %d entries, want %d: %v", p.userID, len(latest), want, latest)
		}

		// echo must never happen, not just in the final snapshot
		for i, snapshot := range p.snapshotHistory() {
			for _, id := range snapshot {
				if id == p.userID {
					t.Errorf("user %d: offer #%d echoed its own track: %v", p.userID, i, snapshot)
				}
			}
		}
	}
}

// assertRemoteTracks checks that the roster was backed by real media. It counts
// every track the browser side has EVER been offered, so it is only meaningful
// while nobody has left yet.
func assertRemoteTracks(t *testing.T, peers []*testPeer, want int) {
	t.Helper()

	for _, p := range peers {
		if got := p.remoteTrackCount(); got != want {
			t.Errorf("user %d received %d remote tracks, want %d", p.userID, got, want)
		}
	}
}

// ---- peer setup --------------------------------------------------------

// runBrowserSide mirrors what a real client plus the handler's recv loop do:
// answer every offer, feed candidates back, and — crucially — apply the answer
// through wrapper.ApplyAnswer so a renegotiation deferred by glare gets
// resumed, exactly like SignalingService.SignalAnswer does in production.
func runBrowserSide(t *testing.T, ev *SignalingEvent, p *testPeer, serverCands, clientCands *candidateBuffer, stop <-chan struct{}) {
	wrapper := p.wrapper
	userID := p.userID

	go func() {
		for {
			select {
			case <-stop:
				return
			case offer := <-wrapper.Offer:
				p.recordOffer(offer)
				if err := p.clientPC.SetRemoteDescription(offer.SessionDescription); err != nil {
					t.Logf("user %d: client SetRemoteDescription: %v", userID, err)
					continue
				}
				clientCands.markReady()

				answer, err := p.clientPC.CreateAnswer(nil)
				if err != nil {
					t.Logf("user %d: client CreateAnswer: %v", userID, err)
					continue
				}
				if err := p.clientPC.SetLocalDescription(answer); err != nil {
					t.Logf("user %d: client SetLocalDescription: %v", userID, err)
					continue
				}

				pending, err := wrapper.ApplyAnswer(answer)
				if err != nil {
					t.Logf("user %d: server ApplyAnswer: %v", userID, err)
					continue
				}
				serverCands.markReady()

				// resume out of band: SendOffer blocks on this very goroutine
				// reading wrapper.Offer, so calling it inline would deadlock.
				if pending {
					go func() { _ = ev.SendOffer(wrapper) }()
				}
			case c := <-wrapper.Candidate:
				clientCands.add(c.ToJSON())
			case err := <-wrapper.Err:
				t.Logf("user %d: wrapper err: %v", userID, err)
				return
			}
		}
	}()
}

// newTestPeer builds the server-side half of a participant the same way
// PreSignaling does: PC with a recvonly audio transceiver, callbacks wired,
// wrapper saved in the manager.
func newTestPeer(t *testing.T, ev *SignalingEvent, mgr *manager.SignalingManager, channelID, userID int64) (*testPeer, *candidateBuffer, *candidateBuffer) {
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

	p := &testPeer{
		userID:       userID,
		serverPC:     serverPC,
		clientPC:     clientPC,
		remoteTracks: map[string]struct{}{},
	}
	p.wrapper = domain.NewPeerConnectionWrapper(serverPC, userID, channelID, fmt.Sprintf("web-%d", userID))

	serverCands := &candidateBuffer{pc: serverPC}
	clientCands := &candidateBuffer{pc: clientPC}

	serverPC.OnICECandidate(ev.OnIceCandidate(p.wrapper))
	serverPC.OnTrack(ev.OnTrack(channelID, p.wrapper))
	mgr.Save(channelID, p.wrapper)

	clientPC.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		serverCands.add(c.ToJSON())
	})
	clientPC.OnTrack(func(tr *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		p.mu.Lock()
		p.remoteTracks[tr.ID()] = struct{}{}
		p.mu.Unlock()
	})

	return p, serverCands, clientCands
}

// joinPeer mirrors the production path end to end: PreSignaling → the
// handler's initial SendOffer, with a browser-like client PC on the other end
// that publishes one audio track.
func joinPeer(t *testing.T, ev *SignalingEvent, mgr *manager.SignalingManager, channelID, userID int64, stop <-chan struct{}) *testPeer {
	t.Helper()

	p, serverCands, clientCands := newTestPeer(t, ev, mgr, channelID, userID)

	audio, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		fmt.Sprintf("mic-%d", userID),
		fmt.Sprintf("stream-%d", userID),
	)
	if err != nil {
		t.Fatalf("client track: %v", err)
	}
	if _, err := p.clientPC.AddTrack(audio); err != nil {
		t.Fatalf("client AddTrack: %v", err)
	}

	runBrowserSide(t, ev, p, serverCands, clientCands, stop)

	if err := ev.SendOffer(p.wrapper); err != nil {
		t.Errorf("user %d: initial SendOffer: %v", userID, err)
	}

	// keep RTP flowing so the SFU's OnTrack fires and stays alive
	go func() {
		ticker := time.NewTicker(20 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				_ = audio.WriteSample(media.Sample{Data: []byte{0xF8, 0xFF, 0xFE}, Duration: 20 * time.Millisecond})
			}
		}
	}()

	return p
}

// joinPeerSilent is joinPeer for a participant who never turns the mic on: the
// client PC publishes no track, so the server's OnTrack never fires for them.
func joinPeerSilent(t *testing.T, ev *SignalingEvent, mgr *manager.SignalingManager, channelID, userID int64, stop <-chan struct{}) *testPeer {
	t.Helper()

	p, serverCands, clientCands := newTestPeer(t, ev, mgr, channelID, userID)
	runBrowserSide(t, ev, p, serverCands, clientCands, stop)

	if err := ev.SendOffer(p.wrapper); err != nil {
		t.Errorf("user %d: initial SendOffer: %v", userID, err)
	}

	return p
}

func newTestEvent(t *testing.T) (*SignalingEvent, *manager.SignalingManager) {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := manager.NewSignalingManager(logger)

	return NewSignalingEvent(mgr, stubUserUsecase{}, logger), mgr
}

// ---- tests -------------------------------------------------------------

// TestFanoutOnSequentialJoins reproduces "user joins a call room": three users
// join one after another and each one's latest snapshot must list the other two.
func TestFanoutOnSequentialJoins(t *testing.T) {
	ev, mgr := newTestEvent(t)

	stop := make(chan struct{})
	defer close(stop)

	const channelID = int64(42)
	userIDs := []int64{1, 2, 3}
	peers := make([]*testPeer, 0, len(userIDs))

	for _, uid := range userIDs {
		peers = append(peers, joinPeer(t, ev, mgr, channelID, uid, stop))
		time.Sleep(3 * time.Second) // let negotiation + fanout settle
	}

	time.Sleep(2 * time.Second)

	logPeers(t, peers)
	assertRosterConverged(t, peers, userIDs)
	assertRemoteTracks(t, peers, len(userIDs)-1)
}

// TestFanoutOnConcurrentJoins is the same room, but everyone joins at once —
// which is what happens when a call starts and each client hits "join". This is
// the case issue #5 failed 100% of the time: the glare between the initial
// offer and the fanout offer dropped the roster on the floor.
func TestFanoutOnConcurrentJoins(t *testing.T) {
	ev, mgr := newTestEvent(t)

	stop := make(chan struct{})
	defer close(stop)

	const channelID = int64(77)
	userIDs := []int64{1, 2, 3}
	peers := make([]*testPeer, len(userIDs))

	var wg sync.WaitGroup
	for i, uid := range userIDs {
		wg.Add(1)
		go func(i int, uid int64) {
			defer wg.Done()
			peers[i] = joinPeer(t, ev, mgr, channelID, uid, stop)
		}(i, uid)
	}
	wg.Wait()

	time.Sleep(8 * time.Second)

	logPeers(t, peers)
	assertRosterConverged(t, peers, userIDs)
	assertRemoteTracks(t, peers, len(userIDs)-1)
}

// TestLeaveNotification: 3 users, then user 2 drops. Departure is delivered by
// user 2 simply disappearing from the next snapshot — that is the whole leave
// contract for now (an explicit leave event is a follow-up).
func TestLeaveNotification(t *testing.T) {
	ev, mgr := newTestEvent(t)

	stop := make(chan struct{})
	defer close(stop)

	const channelID = int64(123)

	p1 := joinPeer(t, ev, mgr, channelID, 1, stop)
	time.Sleep(3 * time.Second)
	p2 := joinPeer(t, ev, mgr, channelID, 2, stop)
	time.Sleep(3 * time.Second)
	p3 := joinPeer(t, ev, mgr, channelID, 3, stop)
	time.Sleep(3 * time.Second)

	assertRosterConverged(t, []*testPeer{p1, p2, p3}, []int64{1, 2, 3})
	assertRemoteTracks(t, []*testPeer{p1, p2, p3}, 2)

	for _, p := range []*testPeer{p1, p3} {
		p.mu.Lock()
		p.offersBeforeLeave = len(p.offers)
		p.mu.Unlock()
	}

	// user 2 leaves — same teardown order as PostSignaling
	mgr.Delete(channelID, p2.wrapper)
	_ = p2.serverPC.Close()
	p2.wrapper.Close()
	_ = p2.clientPC.Close()

	time.Sleep(4 * time.Second)

	remaining := []*testPeer{p1, p3}
	logPeers(t, remaining)

	for _, p := range remaining {
		if p.offerCount() == p.offersBeforeLeave {
			t.Errorf("user %d received no offer after user 2 left", p.userID)
			continue
		}

		latest := p.latestSnapshot()
		for _, id := range latest {
			if id == 2 {
				t.Errorf("user %d still lists the departed user 2: %v", p.userID, latest)
			}
		}
	}

	// the two survivors still see each other. Only the roster is asserted here:
	// remoteTrackCount is cumulative, so user 2's track ID stays in the set even
	// though its sender was removed.
	assertRosterConverged(t, remaining, []int64{1, 3})
}

// TestSilentJoinerIsInvisible pins down a KNOWN GAP that is deliberately out of
// scope for this cycle (issue #5 axis C, split into a follow-up issue): fanout
// is only ever triggered by OnTrack, so a participant who never publishes audio
// (muted mic / no device) contributes no UserTrack and therefore never shows up
// in anyone's snapshot.
//
// The assertions below document today's behaviour so the follow-up has a
// baseline to flip: the silent joiner is absent from everyone else's roster,
// and while it does receive the publishers' names in its own snapshot (the
// snapshot is recomputed from the manager at send time), no media is ever
// attached to it. Closing the gap means making both of those consistent.
func TestSilentJoinerIsInvisible(t *testing.T) {
	ev, mgr := newTestEvent(t)

	stop := make(chan struct{})
	defer close(stop)

	const channelID = int64(99)

	p1 := joinPeer(t, ev, mgr, channelID, 1, stop)
	time.Sleep(3 * time.Second)
	p2 := joinPeer(t, ev, mgr, channelID, 2, stop)
	time.Sleep(3 * time.Second)
	p3 := joinPeerSilent(t, ev, mgr, channelID, 3, stop) // 마이크 안 켬
	time.Sleep(4 * time.Second)

	logPeers(t, []*testPeer{p1, p2, p3})

	// the publishers see each other correctly...
	assertRosterConverged(t, []*testPeer{p1, p2}, []int64{1, 2})
	assertRemoteTracks(t, []*testPeer{p1, p2}, 1)

	// ...but never learn about the silent joiner (the known gap)
	for _, p := range []*testPeer{p1, p2} {
		for _, id := range p.latestSnapshot() {
			if id == 3 {
				t.Errorf("user %d now sees the silent joiner — the follow-up issue landed, update this test: %v",
					p.userID, p.latestSnapshot())
			}
		}
	}

	// the silent joiner is offered no media at all
	if p3.remoteTrackCount() != 0 {
		t.Errorf("silent joiner received %d remote tracks, want 0 (no fanout is triggered for it)", p3.remoteTrackCount())
	}
}

// TestFanoutSequentialGapSweep is a manual investigation knob, not a regression
// gate: set FANOUT_GAP_MS to squeeze the join interval and watch how much
// settling time the fanout needs. Skipped by default so `go test ./...` stays
// quick.
func TestFanoutSequentialGapSweep(t *testing.T) {
	v := os.Getenv("FANOUT_GAP_MS")
	if v == "" {
		t.Skip("set FANOUT_GAP_MS to run the join-interval sweep")
	}

	gapMs, err := strconv.Atoi(v)
	if err != nil {
		t.Fatalf("FANOUT_GAP_MS: %v", err)
	}
	gap := time.Duration(gapMs) * time.Millisecond

	ev, mgr := newTestEvent(t)

	stop := make(chan struct{})
	defer close(stop)

	const channelID = int64(4242)
	userIDs := []int64{1, 2, 3}
	peers := make([]*testPeer, 0, len(userIDs))

	for _, uid := range userIDs {
		peers = append(peers, joinPeer(t, ev, mgr, channelID, uid, stop))
		time.Sleep(gap)
	}

	time.Sleep(6 * time.Second)

	t.Logf("gap=%dms", gapMs)
	logPeers(t, peers)
	assertRosterConverged(t, peers, userIDs)
	assertRemoteTracks(t, peers, len(userIDs)-1)
}
