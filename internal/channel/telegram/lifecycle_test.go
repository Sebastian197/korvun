// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/go-telegram/bot"
)

func TestStart_pollingHappyPath(t *testing.T) {
	runner := newRunnableBotClient()
	a, err := New(
		WithToken("test-token"),
		WithMode(ModePolling),
		withInjectedBotForTests(runner),
	)
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}

	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start() err = %v", err)
	}

	select {
	case <-runner.started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("runner.Start was not invoked after Start()")
	}
	if runner.lastDeleteWebhook == nil {
		t.Error("expected DeleteWebhook safety-net call before polling loop")
	}

	if err := a.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() err = %v", err)
	}

	// inbound chan must be closed after Stop.
	if _, ok := <-a.inbound; ok {
		t.Error("inbound chan was not closed after Stop()")
	}
}

func TestStart_doubleStartReturnsErrAlreadyStarted(t *testing.T) {
	runner := newRunnableBotClient()
	a, err := New(
		WithToken("test-token"),
		WithMode(ModePolling),
		withInjectedBotForTests(runner),
	)
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("first Start err = %v", err)
	}
	t.Cleanup(func() { _ = a.Stop(context.Background()) })

	if err := a.Start(context.Background()); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("second Start() err = %v, want ErrAlreadyStarted", err)
	}
}

func TestStop_idempotent(t *testing.T) {
	runner := newRunnableBotClient()
	a, err := New(
		WithToken("test-token"),
		WithMode(ModePolling),
		withInjectedBotForTests(runner),
	)
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start() err = %v", err)
	}
	if err := a.Stop(context.Background()); err != nil {
		t.Fatalf("first Stop() err = %v", err)
	}
	// Second Stop must not panic or close the channel a second time.
	if err := a.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop() err = %v", err)
	}
}

func TestStop_beforeStartIsNoOp(t *testing.T) {
	a := newTestAdapter(t)
	if err := a.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() before Start err = %v", err)
	}
	if _, ok := <-a.inbound; ok {
		t.Error("inbound should be closed after Stop on a never-started adapter")
	}
}

// TestStop_doesNotPanicWithInFlightDispatch reproduces the race
// the original Phase 2E.8 sub-E commit had: dispatchUpdate goroutines
// blocked in the saturated-buffer select while Stop closed
// a.inbound. Before the fix this test panics under -race with
// "send on closed channel"; after the fix every in-flight
// dispatchUpdate observes <-a.done and returns cleanly, Stop joins
// dispatchWG before closing, and no send hits a closed channel.
//
// The test runs under -race because the race detector is what
// makes the bug consistently visible — wall-clock alone would
// catch it only sometimes.
func TestStop_doesNotPanicWithInFlightDispatch(t *testing.T) {
	runner := newRunnableBotClient()
	a, err := New(
		WithToken("test-token"),
		WithMode(ModePolling),
		WithInboundCapacity(1),
		// Long enough that without the <-a.done case the dispatch
		// goroutines would still be parked in the select when Stop
		// reaches close(a.inbound).
		WithEnqueueTimeout(5*time.Second),
		withInjectedBotForTests(runner),
	)
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start() err = %v", err)
	}

	// Saturate the buffer (no reader is attached).
	a.dispatchUpdate(context.Background(), newTextUpdate(1, 1, "first"))
	if got := len(a.inbound); got != 1 {
		t.Fatalf("inbound not saturated: len = %d, want 1", got)
	}

	// Launch N dispatchers that will block on the saturated send.
	const N = 8
	var ready sync.WaitGroup
	var done sync.WaitGroup
	for i := 0; i < N; i++ {
		ready.Add(1)
		done.Add(1)
		go func(i int) {
			defer done.Done()
			ready.Done()
			a.dispatchUpdate(context.Background(),
				newTextUpdate(1, 100+i, "blocked"))
		}(i)
	}
	ready.Wait()
	// Let the goroutines enter the select. 50ms is generous but
	// keeps the test fast under -race overhead.
	time.Sleep(50 * time.Millisecond)

	// Stop with a short ctx — this is the original failure mode:
	// httpServer.Shutdown / loopCancel returns quickly, workers.Wait
	// returns quickly, and close(a.inbound) used to race the
	// in-flight sends.
	stopCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if err := a.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() err = %v", err)
	}

	// Every blocked dispatchUpdate must have returned cleanly via
	// the <-a.done case (no send, no counter increment).
	waitDone := make(chan struct{})
	go func() {
		done.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(2 * time.Second):
		t.Fatal("dispatch goroutines did not return within 2s of Stop()")
	}

	// Drop counter must not have been incremented by the shutdown
	// path — drops are saturation events, not shutdown events.
	if got := a.DroppedCount(); got != 0 {
		t.Errorf("DroppedCount = %d after shutdown, want 0", got)
	}

	// Verify a.inbound is closed (drains the one seeded envelope
	// first, then returns the closed-channel ok=false).
	for range a.inbound {
	}
	if _, ok := <-a.inbound; ok {
		t.Error("inbound channel was not closed after Stop()")
	}
}

func TestStart_webhookHappyPath(t *testing.T) {
	fake := &capturingBotClient{}
	a, err := New(
		WithToken("test-token"),
		WithMode(ModeWebhook),
		WithWebhookURL("https://example.com/wh"),
		WithListenAddr("127.0.0.1:0"),
		WithSecretToken("topsecret"),
		WithReverseProxyTermination(),
		WithAllowedUpdates([]string{"message", "callback_query"}),
		WithDropPendingOnStart(true),
		withInjectedBotForTests(fake),
	)
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start() err = %v", err)
	}
	if fake.lastSetWebhook == nil {
		t.Fatal("SetWebhook was not invoked")
	}
	if fake.lastSetWebhook.URL != "https://example.com/wh" {
		t.Errorf("SetWebhook URL = %q", fake.lastSetWebhook.URL)
	}
	if fake.lastSetWebhook.SecretToken != "topsecret" {
		t.Errorf("SetWebhook SecretToken propagation failed")
	}
	if !fake.lastSetWebhook.DropPendingUpdates {
		t.Error("SetWebhook DropPendingUpdates was not propagated")
	}
	if len(fake.lastSetWebhook.AllowedUpdates) != 2 {
		t.Errorf("SetWebhook AllowedUpdates = %v", fake.lastSetWebhook.AllowedUpdates)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := a.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() err = %v", err)
	}
	if fake.lastDeleteWebhook == nil {
		t.Error("DeleteWebhook was not invoked at shutdown")
	}
	if _, ok := <-a.inbound; ok {
		t.Error("inbound chan was not closed after Stop()")
	}
}

func TestStart_webhookSetWebhookErrorRollsBack(t *testing.T) {
	setErr := errors.New("bad URL")
	fake := &webhookFailingBotClient{setWebhookErr: setErr}
	a, err := New(
		WithToken("test-token"),
		WithMode(ModeWebhook),
		WithWebhookURL("https://example.com/wh"),
		WithListenAddr("127.0.0.1:0"),
		WithSecretToken("s"),
		WithReverseProxyTermination(),
		withInjectedBotForTests(fake),
	)
	if err != nil {
		t.Fatalf("New() err = %v", err)
	}
	err = a.Start(context.Background())
	if err == nil {
		t.Fatal("expected Start() to fail when SetWebhook errors")
	}
	if !errors.Is(err, setErr) {
		t.Fatalf("Start err = %v, does not wrap %v", err, setErr)
	}
	// After rollback, a fresh Stop must still cleanly close the channel.
	if err := a.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() after failed Start err = %v", err)
	}
}

// webhookFailingBotClient overrides only SetWebhook to surface a
// configurable error, so the webhook-failure-rollback test can
// assert without touching the rest of the capturing client.
type webhookFailingBotClient struct {
	capturingBotClient
	setWebhookErr error
}

func (w *webhookFailingBotClient) SetWebhook(_ context.Context, p *bot.SetWebhookParams) (bool, error) {
	if w.setWebhookErr != nil {
		return false, w.setWebhookErr
	}
	w.lastSetWebhook = p
	return true, nil
}
