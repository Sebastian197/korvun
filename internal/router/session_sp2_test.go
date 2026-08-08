// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// SP2 red suite (operator-console spec 2026-08-08): session triggers on the
// dispatch path (AS-10), lazy daily/idle expiry with an injected clock
// (AS-11), the per-conversation takeover gate (AS-4), and the public
// outbound entry persisting operator turns through the one outbound funnel
// (AS-3's router half). Everything runs against fake channel/brain and a
// MemStore — zero timers, zero real time: the tests travel with the clock.
package router_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Sebastian197/korvun/internal/bus"
	"github.com/Sebastian197/korvun/internal/conversation"
	"github.com/Sebastian197/korvun/internal/envelope"
	"github.com/Sebastian197/korvun/internal/router"
)

// fakeClock is the injected time source: the tests move it by hand.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock(t time.Time) *fakeClock { return &fakeClock{t: t} }
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}
func (c *fakeClock) Set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

// fakeBus captures published lifecycle events.
type fakeBus struct {
	mu  sync.Mutex
	evs []bus.Event
}

func (b *fakeBus) Publish(_ context.Context, ev bus.Event) {
	b.mu.Lock()
	b.evs = append(b.evs, ev)
	b.mu.Unlock()
}
func (b *fakeBus) Events() []bus.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]bus.Event(nil), b.evs...)
}

// waitUntil polls cond for up to ~2s; the async worker paths make direct
// asserts racy without it.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// settle gives the async workers a beat to do something they must NOT do
// (e.g. invoke a silenced brain) before the negative assert.
func settle() { time.Sleep(80 * time.Millisecond) }

const key = conversation.Key("tg::c")

var defaultTriggers = []string{"/new", "/reset"}

func sessionRouter(t *testing.T, store conversation.SessionStore, opts ...router.Option) (*router.Router, *fakeChannel, *fakeBrain) {
	t.Helper()
	ch := newFakeChannel("tg")
	br := newFakeBrain()
	base := []router.Option{router.WithSessionStore(store)}
	r := router.New(append(base, opts...)...)
	if err := r.RegisterChannel(ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	if err := r.RegisterBrain("b", br); err != nil {
		t.Fatalf("RegisterBrain: %v", err)
	}
	if err := r.Route("tg", "b"); err != nil {
		t.Fatalf("Route: %v", err)
	}
	t.Cleanup(func() { shutdown(t, r) })
	return r, ch, br
}

func seedTurn(t *testing.T, store conversation.SessionStore, content string, ts time.Time) {
	t.Helper()
	if _, err := store.Append(context.Background(), key,
		conversation.Turn{Role: conversation.RoleUser, Content: content, Timestamp: ts}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func sessionCount(t *testing.T, store conversation.SessionStore) int {
	t.Helper()
	ss, err := store.ListSessions(context.Background(), key)
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	return len(ss)
}

// --- AS-10: triggers --------------------------------------------------------

func TestTrigger_NewWithRemainderReachesBrainInFreshSession(t *testing.T) {
	store := conversation.NewMemStore()
	seedTurn(t, store, "vieja", time.Unix(100, 0))
	r, _, br := sessionRouter(t, store,
		router.WithSessionPolicy(router.SessionPolicy{Triggers: defaultTriggers}))

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/new hola")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "brain handled the remainder", func() bool { return len(br.Handled()) == 1 })

	got := br.Handled()[0]
	if len(got.Parts) != 1 || got.Parts[0].Content != "hola" {
		t.Fatalf("brain received %+v, want exactly the remainder \"hola\"", got.Parts)
	}
	if got.Meta[router.MetaConversationID] != "c" {
		t.Fatalf("remainder envelope lost its conversation id: %+v", got.Meta)
	}
	if n := sessionCount(t, store); n != 2 {
		t.Fatalf("sessions = %d, want 2 (trigger opens a session BEFORE dispatch)", n)
	}
	// The trigger text is nowhere: not in the old session, not in the new.
	for _, sess := range []int{1, 2} {
		turns, err := store.LoadSession(context.Background(), key, sess)
		if err != nil {
			t.Fatalf("LoadSession(%d): %v", sess, err)
		}
		for _, tr := range turns {
			if strings.Contains(tr.Content, "/new") {
				t.Fatalf("trigger text persisted in session %d: %+v", sess, tr)
			}
		}
	}
}

func TestTrigger_BareSendsFixedAckWithoutBrainOrPersistence(t *testing.T) {
	store := conversation.NewMemStore()
	seedTurn(t, store, "vieja", time.Unix(100, 0))
	r, ch, br := sessionRouter(t, store,
		router.WithSessionPolicy(router.SessionPolicy{Triggers: defaultTriggers}))

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/reset")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "the acknowledgement to leave", func() bool { return len(ch.Sent()) == 1 })

	ack := ch.Sent()[0]
	if ack.Channel != "tg" || ack.Direction != envelope.Outbound {
		t.Fatalf("ack envelope misaddressed: %+v", ack)
	}
	if len(ack.Parts) != 1 || ack.Parts[0].Content != router.SessionResetAck {
		t.Fatalf("ack copy = %q, want the fixed %q", ack.Parts[0].Content, router.SessionResetAck)
	}
	if ack.Meta[router.MetaConversationID] != "c" {
		t.Fatalf("ack lost the conversation addressing meta: %+v", ack.Meta)
	}
	if n := len(br.Handled()); n != 0 {
		t.Fatalf("brain invoked %d times for a bare trigger, want 0", n)
	}
	if n := sessionCount(t, store); n != 2 {
		t.Fatalf("sessions = %d, want 2", n)
	}
	// Neither the trigger nor the ack is persisted: the new session is empty.
	fresh, err := store.LoadSession(context.Background(), key, 2)
	if err != nil || len(fresh) != 0 {
		t.Fatalf("new session turns = (%v, %v), want empty", fresh, err)
	}
}

func TestTrigger_ExactFirstTokenCaseSensitiveOnly(t *testing.T) {
	for _, text := range []string{"/newx hola", "x /new", "/New hola", "algo /reset algo"} {
		t.Run(text, func(t *testing.T) {
			store := conversation.NewMemStore()
			seedTurn(t, store, "vieja", time.Unix(100, 0))
			r, _, br := sessionRouter(t, store,
				router.WithSessionPolicy(router.SessionPolicy{Triggers: defaultTriggers}))

			if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", text)); err != nil {
				t.Fatalf("DispatchInbound: %v", err)
			}
			waitUntil(t, "brain handled", func() bool { return len(br.Handled()) == 1 })
			if got := br.Handled()[0].Parts[0].Content; got != text {
				t.Fatalf("non-trigger text rewritten: %q -> %q", text, got)
			}
			if n := sessionCount(t, store); n != 1 {
				t.Fatalf("sessions = %d, want 1 (no reset for a non-trigger)", n)
			}
		})
	}
}

func TestTrigger_SetIsConfigurable(t *testing.T) {
	store := conversation.NewMemStore()
	seedTurn(t, store, "vieja", time.Unix(100, 0))
	r, _, br := sessionRouter(t, store,
		router.WithSessionPolicy(router.SessionPolicy{Triggers: []string{"/otra"}}))

	// The configured trigger fires…
	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/otra hola")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "brain handled remainder", func() bool { return len(br.Handled()) == 1 })
	if n := sessionCount(t, store); n != 2 {
		t.Fatalf("sessions = %d, want 2 (configured trigger must fire)", n)
	}
	// …and the default set does not exist unless configured.
	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/new hola")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "brain handled literal /new", func() bool { return len(br.Handled()) == 2 })
	if got := br.Handled()[1].Parts[0].Content; got != "/new hola" {
		t.Fatalf("unconfigured trigger rewrote the text: %q", got)
	}
}

// --- AS-11: lazy expiry with an injected clock ------------------------------

func TestExpiry_IdleFiresOnlyAtNextInbound(t *testing.T) {
	store := conversation.NewMemStore()
	t0 := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	seedTurn(t, store, "última actividad", t0)
	clock := newFakeClock(t0.Add(29 * time.Minute))
	r, _, br := sessionRouter(t, store,
		router.WithSessionPolicy(router.SessionPolicy{IdleMin: 30}),
		router.WithClock(clock.Now))

	// 29 minutes idle: same session.
	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "sigo")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "brain handled", func() bool { return len(br.Handled()) == 1 })
	if n := sessionCount(t, store); n != 1 {
		t.Fatalf("sessions after 29m = %d, want 1", n)
	}

	// 31 minutes after the LAST ACTIVITY (the store still says t0 — the fake
	// brain persists nothing): the next inbound opens a new session.
	clock.Set(t0.Add(31 * time.Minute))
	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "he vuelto")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "brain handled again", func() bool { return len(br.Handled()) == 2 })
	if n := sessionCount(t, store); n != 2 {
		t.Fatalf("sessions after 31m idle = %d, want 2", n)
	}
}

func TestExpiry_DailyBoundaryLocalHour(t *testing.T) {
	store := conversation.NewMemStore()
	lastNight := time.Date(2026, 8, 7, 23, 0, 0, 0, time.UTC)
	seedTurn(t, store, "anoche", lastNight)
	clock := newFakeClock(time.Date(2026, 8, 8, 3, 59, 0, 0, time.UTC))
	r, _, br := sessionRouter(t, store,
		router.WithSessionPolicy(router.SessionPolicy{Daily: true, DailyHour: 4}),
		router.WithClock(clock.Now))

	// Before the boundary: same session.
	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "madrugada")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "brain handled", func() bool { return len(br.Handled()) == 1 })
	if n := sessionCount(t, store); n != 1 {
		t.Fatalf("sessions before boundary = %d, want 1", n)
	}

	// Past 04:00 with the last activity before it: new session.
	clock.Set(time.Date(2026, 8, 8, 4, 1, 0, 0, time.UTC))
	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "buenos días")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "brain handled again", func() bool { return len(br.Handled()) == 2 })
	if n := sessionCount(t, store); n != 2 {
		t.Fatalf("sessions past boundary = %d, want 2", n)
	}
}

func TestExpiry_CombinedRulesOpenExactlyOneSession(t *testing.T) {
	// Both daily and idle have fired; a trigger rides the same inbound too.
	// First-to-fire wins and NewSession's empty-active idempotence guarantees
	// ONE new session — never two or three.
	store := conversation.NewMemStore()
	t0 := time.Date(2026, 8, 7, 23, 0, 0, 0, time.UTC)
	seedTurn(t, store, "anoche", t0)
	clock := newFakeClock(time.Date(2026, 8, 8, 9, 0, 0, 0, time.UTC))
	r, _, br := sessionRouter(t, store,
		router.WithSessionPolicy(router.SessionPolicy{
			Triggers: defaultTriggers, Daily: true, DailyHour: 4, IdleMin: 30,
		}),
		router.WithClock(clock.Now))

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/new hola")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "brain handled remainder", func() bool { return len(br.Handled()) == 1 })
	if got := br.Handled()[0].Parts[0].Content; got != "hola" {
		t.Fatalf("remainder = %q, want \"hola\"", got)
	}
	if n := sessionCount(t, store); n != 2 {
		t.Fatalf("sessions = %d, want exactly 2 (one combined reset)", n)
	}
}

func TestExpiry_NonePolicyNeverResets(t *testing.T) {
	store := conversation.NewMemStore()
	seedTurn(t, store, "hace un año", time.Date(2025, 8, 8, 12, 0, 0, 0, time.UTC))
	clock := newFakeClock(time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC))
	r, _, br := sessionRouter(t, store, router.WithClock(clock.Now))

	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "sigo aquí")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "brain handled", func() bool { return len(br.Handled()) == 1 })
	if n := sessionCount(t, store); n != 1 {
		t.Fatalf("sessions = %d, want 1 (policy none is the default)", n)
	}
}

// --- AS-4: the takeover gate ------------------------------------------------

func TestTakeover_SilencesBrainAndPersistsUserTurns(t *testing.T) {
	store := conversation.NewMemStore()
	seedTurn(t, store, "antes", time.Unix(100, 0))
	clock := newFakeClock(time.Unix(200, 0).UTC())
	events := &fakeBus{}
	r, _, br := sessionRouter(t, store,
		router.WithClock(clock.Now), router.WithEventPublisher(events))

	r.TakeOver(key)
	if !r.TakenOver(key) {
		t.Fatal("TakenOver = false right after TakeOver")
	}
	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "ayuda")); err != nil {
		t.Fatalf("DispatchInbound under takeover: %v", err)
	}
	waitUntil(t, "user turn persisted under takeover", func() bool {
		turns, _ := store.LoadRecent(context.Background(), key, 10)
		return len(turns) == 2
	})
	settle()
	if n := len(br.Handled()); n != 0 {
		t.Fatalf("brain invoked %d times under takeover, want 0", n)
	}
	turns, _ := store.LoadRecent(context.Background(), key, 10)
	last := turns[len(turns)-1]
	if last.Role != conversation.RoleUser || last.Content != "ayuda" || !last.Timestamp.Equal(clock.Now()) {
		t.Fatalf("persisted takeover turn = %+v, want user/ayuda @clock", last)
	}
	// The console still hears about the message: a MessageReceived event.
	var received int
	for _, ev := range events.Events() {
		if ev.Type == bus.MessageReceived {
			received++
		}
	}
	if received != 1 {
		t.Fatalf("MessageReceived events = %d, want 1 (the console needs its change signal)", received)
	}

	// Release: the brain speaks again.
	r.Release(key)
	if r.TakenOver(key) {
		t.Fatal("TakenOver = true after Release")
	}
	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "ya está")); err != nil {
		t.Fatalf("DispatchInbound after release: %v", err)
	}
	waitUntil(t, "brain handled after release", func() bool { return len(br.Handled()) == 1 })
}

func TestTakeover_SurvivesSessionReset(t *testing.T) {
	store := conversation.NewMemStore()
	seedTurn(t, store, "antes", time.Unix(100, 0))
	r, ch, br := sessionRouter(t, store,
		router.WithSessionPolicy(router.SessionPolicy{Triggers: defaultTriggers}))

	r.TakeOver(key)
	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/new")); err != nil {
		t.Fatalf("DispatchInbound trigger under takeover: %v", err)
	}
	waitUntil(t, "ack sent", func() bool { return len(ch.Sent()) == 1 })
	if !r.TakenOver(key) {
		t.Fatal("a session reset released the takeover — it must NOT (FR-SESS-6)")
	}
	if err := r.DispatchInbound(context.Background(), mkInbound("tg", "c", "sigo esperando")); err != nil {
		t.Fatalf("DispatchInbound: %v", err)
	}
	waitUntil(t, "user turn persisted in the NEW session", func() bool {
		turns, _ := store.LoadSession(context.Background(), key, 2)
		return len(turns) == 1 && turns[0].Content == "sigo esperando"
	})
	settle()
	if n := len(br.Handled()); n != 0 {
		t.Fatalf("brain invoked %d times across reset under takeover, want 0", n)
	}
}

func TestTakeover_FreshRouterFailsOpen(t *testing.T) {
	r, _, _ := sessionRouter(t, conversation.NewMemStore())
	if r.TakenOver(key) {
		t.Fatal("fresh router reports a takeover — the gate must fail open to the brain")
	}
}

// --- AS-3 (router half): the public outbound entry --------------------------

func TestDispatchOutbound_SameFunnelPersistsOperatorTurn(t *testing.T) {
	store := conversation.NewMemStore()
	seedTurn(t, store, "hola", time.Unix(100, 0))
	clock := newFakeClock(time.Unix(300, 0).UTC())
	events := &fakeBus{}
	r, ch, _ := sessionRouter(t, store,
		router.WithClock(clock.Now), router.WithEventPublisher(events))

	if err := r.DispatchOutbound(context.Background(), mkOutbound("tg", "c", "aquí Chano")); err != nil {
		t.Fatalf("DispatchOutbound: %v", err)
	}
	waitUntil(t, "operator reply delivered", func() bool { return len(ch.Sent()) == 1 })
	if got := ch.Sent()[0].Parts[0].Content; got != "aquí Chano" {
		t.Fatalf("delivered %q, want the operator text", got)
	}
	turns, _ := store.LoadRecent(context.Background(), key, 10)
	last := turns[len(turns)-1]
	if last.Role != conversation.RoleOperator || last.Content != "aquí Chano" {
		t.Fatalf("persisted operator turn = %+v, want operator/aquí Chano", last)
	}
	waitUntil(t, "ReplySent event", func() bool {
		for _, ev := range events.Events() {
			if ev.Type == bus.ReplySent {
				return true
			}
		}
		return false
	})
}

func TestDispatchOutbound_Validation(t *testing.T) {
	r, _, _ := sessionRouter(t, conversation.NewMemStore())
	if err := r.DispatchOutbound(context.Background(), nil); err == nil {
		t.Fatal("nil envelope: want error")
	}
	if err := r.DispatchOutbound(context.Background(), mkInbound("tg", "c", "x")); err == nil {
		t.Fatal("inbound direction: want error")
	}
	if err := r.DispatchOutbound(context.Background(), mkOutbound("nope", "c", "x")); err == nil {
		t.Fatal("unknown channel: want error")
	}
}

func TestDispatchOutbound_PersistsBeforeSendFailure(t *testing.T) {
	// Persist-then-send (spec folded decision): a failed delivery is
	// recorded in history AND surfaced through the error hook.
	store := conversation.NewMemStore()
	seedTurn(t, store, "hola", time.Unix(100, 0))
	var (
		mu   sync.Mutex
		errs []router.RouterError
	)
	ch := newFakeChannel("tg")
	ch.sendErr = context.DeadlineExceeded
	br := newFakeBrain()
	r := router.New(
		router.WithSessionStore(store),
		router.WithErrorHandler(func(re router.RouterError) {
			mu.Lock()
			errs = append(errs, re)
			mu.Unlock()
		}),
	)
	if err := r.RegisterChannel(ch); err != nil {
		t.Fatalf("RegisterChannel: %v", err)
	}
	if err := r.RegisterBrain("b", br); err != nil {
		t.Fatalf("RegisterBrain: %v", err)
	}
	if err := r.Route("tg", "b"); err != nil {
		t.Fatalf("Route: %v", err)
	}
	t.Cleanup(func() { shutdown(t, r) })

	if err := r.DispatchOutbound(context.Background(), mkOutbound("tg", "c", "se pierde")); err != nil {
		t.Fatalf("DispatchOutbound: %v", err)
	}
	waitUntil(t, "send failure surfaced", func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, re := range errs {
			if re.Kind == router.ErrKindSend {
				return true
			}
		}
		return false
	})
	turns, _ := store.LoadRecent(context.Background(), key, 10)
	if last := turns[len(turns)-1]; last.Role != conversation.RoleOperator {
		t.Fatalf("failed send lost the operator turn: %+v (persist-then-send)", turns)
	}
}

// --- concurrency: the -race meat --------------------------------------------

func TestSessionPaths_RaceClean(t *testing.T) {
	store := conversation.NewMemStore()
	clock := newFakeClock(time.Unix(500, 0).UTC())
	r, _, _ := sessionRouter(t, store,
		router.WithSessionPolicy(router.SessionPolicy{Triggers: defaultTriggers, IdleMin: 30}),
		router.WithClock(clock.Now))

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				switch i % 4 {
				case 0:
					_ = r.DispatchInbound(context.Background(), mkInbound("tg", "c", "hola"))
				case 1:
					_ = r.DispatchOutbound(context.Background(), mkOutbound("tg", "c", "op"))
				case 2:
					r.TakeOver(key)
					r.Release(key)
				case 3:
					_ = r.DispatchInbound(context.Background(), mkInbound("tg", "c", "/new otra"))
				}
			}
		}(i)
	}
	wg.Wait()
}
