// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"sync"
	"time"

	"github.com/Sebastian197/korvun/internal/model"
)

// warmupTarget is one deduplicated local model to warm at boot: the DECORATED
// model (so the retry decorator's generous per-attempt window and F6 no-retry
// come for free) plus its identity for logging and the warmup request.
type warmupTarget struct {
	model    model.Model
	provider string
	modelID  string
}

// startWarmup launches a best-effort boot warmup for every collected target
// (ADR-0031 sub-phase 6, Decision 1b). It runs from Start — NOT Run — so a
// supervisor-driven boot, which calls Start/Serve separately (ADR-0027), warms
// up too. Each target is warmed in its own goroutine (parallel); the warmup ctx
// hangs off the passed ctx AND off a.warmupCancel so Shutdown can cancel any
// in-flight load; a.warmupDone closes once all goroutines return, so Shutdown can
// await the unwind without dangling a goroutine. An empty target set is a no-op
// (zero behaviour change when no model is marked warmup).
func (a *App) startWarmup(ctx context.Context) {
	if len(a.warmupTargets) == 0 {
		return
	}
	warmupCtx, cancel := context.WithCancel(ctx)
	a.warmupCancel = cancel
	done := make(chan struct{})
	a.warmupDone = done

	var wg sync.WaitGroup
	for _, tgt := range a.warmupTargets {
		wg.Add(1)
		go func(t warmupTarget) {
			defer wg.Done()
			a.warmupOne(warmupCtx, t)
		}(tgt)
	}
	go func() {
		wg.Wait()
		close(done)
	}()
}

// warmupOne warms a single model with a trivial "hi" Generate against the
// DECORATED model, so the decorator's generous per-attempt window applies and a
// deadline-expiry is never retried (F6, for free). Best-effort: a failure logs a
// WARN and returns; no error ever propagates to Start.
func (a *App) warmupOne(ctx context.Context, t warmupTarget) {
	a.logger.Info("warming up model", "provider", t.provider, "model", t.modelID)
	start := time.Now()
	req := &model.Request{
		Model:    t.modelID,
		Messages: []model.Message{{Role: model.RoleUser, Content: "hi"}},
	}
	if _, err := t.model.Generate(ctx, req); err != nil {
		a.logger.Warn("warmup failed", "provider", t.provider, "model", t.modelID, "error", err)
		return
	}
	a.logger.Info("model warm", "provider", t.provider, "model", t.modelID, "took", time.Since(start))
}

// awaitWarmup cancels any in-flight warmup and waits for the goroutines to
// unwind, bounded by ctx, so Shutdown leaves no warmup goroutine dangling
// (ADR-0031 sub-phase 6, AS-6). A no-op when no warmup ran.
func (a *App) awaitWarmup(ctx context.Context) {
	if a.warmupCancel != nil {
		a.warmupCancel()
	}
	if a.warmupDone != nil {
		select {
		case <-a.warmupDone:
		case <-ctx.Done():
		}
	}
}
