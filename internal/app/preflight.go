// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"fmt"
	"log/slog"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/metrics"
)

// Preflight runs the effect-free validation of a config WITHOUT opening the store
// and WITHOUT registering anything on a router (no worker goroutines): it resolves
// every secret, runs the privacy selector per brain (ADR-0015), and constructs each
// channel adapter — for Telegram that is telegram.New -> bot.New, a throwaway getMe
// round-trip that validates the token (ADR-0017 §4). Every constructed value is
// discarded; Preflight proves only that a clean Build WOULD succeed.
//
// It is the pre-cutover half of the reload flow (ADR-0027 §mutation-flow, step 5):
// running it BEFORE the cutover means a bad token / missing secret / no-eligible-model
// config fails cheaply while the old app is still serving, so the cutover is entered
// only for a config already known to construct. openStore and the real, second getMe
// happen INSIDE the cutover (step 6), strictly after the old app's Shutdown — which is
// how the single-writer store is never open twice (F1). The extra throwaway getMe is
// an accepted cost: sub-second, on a rare operator reload (ADR-0027 §6, option B).
//
// Preflight has no PERSISTENT side effects — no store opened, no goroutines
// started, no router registration — and is safe to call repeatedly. It does emit
// Info logs (via the injected logger) as it constructs models/brains. It takes the
// same Options as Build so the caller controls the logger (and tests inject a
// channel factory).
func Preflight(cfg *config.Config, opts ...Option) error {
	b := &builder{
		logger:          slog.Default(),
		perModelTimeout: DefaultPerModelTimeout,
		newChannel:      defaultChannelFactory,
		metrics:         metrics.Nop{},
	}
	for _, o := range opts {
		o(b)
	}

	// Brains: catalog construction (secret resolution) + privacy selector + the
	// agent single-model check — all pure, no store injected, no router, no workers.
	// The built brain is discarded; Preflight only proves it CAN be built.
	for _, bc := range cfg.Brains {
		if _, err := b.buildBrain(bc); err != nil {
			return err
		}
	}

	// Channels: construct each adapter and discard it. For Telegram this is
	// telegram.New -> bot.New, the throwaway getMe validation. Never registered on a
	// router and never Started, so Preflight opens no port and starts no goroutine.
	for _, cc := range cfg.Channels {
		if _, err := b.newChannel(b, cc); err != nil {
			return fmt.Errorf("app: preflight channel %q: %w", cc.Type, err)
		}
	}
	return nil
}
