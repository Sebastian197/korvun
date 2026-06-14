// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-telegram/bot"
)

// Start brings the adapter's transport up. The contract differs
// per Mode but is always all-or-nothing: on success the inbound
// channel is being written to; on error nothing was left
// half-initialised.
//
// ModeWebhook:
//
//  1. Build a mux with webhookHandler mounted at cfg.webhookPath
//     and a basic /healthz reporting OK only when the adapter is
//     running.
//  2. Spawn the *http.Server (ListenAndServeTLS if WithTLS, plain
//     ListenAndServe if WithReverseProxyTermination). The serve
//     error, if any after Start has returned, is logged via
//     WarnContext — see ADR-0008 §4a trade-off block for why a
//     synchronous error channel is not added in this phase.
//  3. Call bot.Bot.SetWebhook to register the URL with Telegram.
//     If SetWebhook fails the just-opened server is shut down
//     before Start returns the error.
//
// ModePolling:
//
//  1. Call bot.Bot.DeleteWebhook as a safety net so a left-over
//     registration does not contend with the polling loop.
//  2. Spawn the polling goroutine running runner.Start(loopCtx).
//     loopCtx is fresh background ctx the adapter cancels in Stop.
//
// Start is idempotent on ErrAlreadyStarted: a second call after a
// successful one returns the sentinel and does not touch any
// transport state.
func (a *Adapter) Start(ctx context.Context) error {
	a.mu.Lock()
	if a.state != stateNew {
		a.mu.Unlock()
		if a.state == stateRunning {
			return ErrAlreadyStarted
		}
		return ErrNotStarted
	}
	a.state = stateRunning
	a.loopCtx, a.loopCancel = context.WithCancel(context.Background())
	a.mu.Unlock()

	switch a.cfg.mode {
	case ModeWebhook:
		if err := a.startWebhook(ctx); err != nil {
			a.rewindToNew()
			return err
		}
	case ModePolling:
		if err := a.startPolling(ctx); err != nil {
			a.rewindToNew()
			return err
		}
	default:
		a.rewindToNew()
		return ErrInvalidMode
	}
	return nil
}

// Stop tears the transport down in the reverse order Start built it
// up, bounded by ctx. Stop is idempotent: repeated calls after the
// first are no-ops. The inbound channel is closed exactly once, so
// the router sees a clean "no more updates" signal it can use to
// drain.
//
// Stop is what main.go calls BEFORE router.Shutdown — see ADR-0008
// §4b for the ordering rule.
func (a *Adapter) Stop(ctx context.Context) error {
	var err error
	a.stopOnce.Do(func() {
		a.mu.Lock()
		switch a.state {
		case stateRunning:
			// fallthrough into the per-mode shutdown below.
		case stateNew:
			a.state = stateStopped
			close(a.inbound)
			a.mu.Unlock()
			return
		case stateStopped:
			a.mu.Unlock()
			return
		}
		a.state = stateStopped
		a.mu.Unlock()

		switch a.cfg.mode {
		case ModeWebhook:
			err = a.stopWebhook(ctx)
		case ModePolling:
			err = a.stopPolling(ctx)
		}
		a.workers.Wait()
		close(a.inbound)
	})
	return err
}

// rewindToNew is the recovery path when Start fails mid-way: undo
// the state and ctx the head of Start set up so a retry sees a
// fresh Adapter rather than a half-running one.
func (a *Adapter) rewindToNew() {
	a.mu.Lock()
	a.state = stateNew
	if a.loopCancel != nil {
		a.loopCancel()
		a.loopCancel = nil
		a.loopCtx = nil
	}
	a.mu.Unlock()
}

func (a *Adapter) startWebhook(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.Handle(a.cfg.webhookPath, a.webhookHandler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		a.mu.Lock()
		running := a.state == stateRunning
		a.mu.Unlock()
		if !running {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	a.httpServer = &http.Server{
		Addr:              a.cfg.listenAddr,
		Handler:           mux,
		ReadHeaderTimeout: a.cfg.readHeaderTimeout,
	}

	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		var serveErr error
		if a.cfg.reverseProxyTLS {
			serveErr = a.httpServer.ListenAndServe()
		} else {
			serveErr = a.httpServer.ListenAndServeTLS(a.cfg.tlsCertFile, a.cfg.tlsKeyFile)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			a.cfg.logger.WarnContext(a.loopCtx,
				"telegram: webhook HTTP server stopped with error",
				"error", serveErr.Error())
		}
	}()

	_, err := a.client.SetWebhook(ctx, &bot.SetWebhookParams{
		URL:                a.cfg.webhookURL,
		SecretToken:        a.cfg.secretToken,
		AllowedUpdates:     append([]string(nil), a.cfg.allowedUpdates...),
		DropPendingUpdates: a.cfg.dropPendingOnStart,
	})
	if err != nil {
		shutdownCtx, cancel := context.WithCancel(ctx)
		_ = a.httpServer.Shutdown(shutdownCtx)
		cancel()
		a.workers.Wait()
		a.httpServer = nil
		return fmt.Errorf("telegram: SetWebhook: %w", err)
	}
	return nil
}

func (a *Adapter) startPolling(ctx context.Context) error {
	_, err := a.client.DeleteWebhook(ctx, &bot.DeleteWebhookParams{
		DropPendingUpdates: a.cfg.dropPendingOnStart,
	})
	if err != nil {
		// The polling loop will surface this conflict on its own
		// when getUpdates returns 409; log and proceed.
		a.cfg.logger.WarnContext(ctx,
			"telegram: DeleteWebhook safety-net call failed; polling loop will surface any conflict",
			"error", err.Error())
	}
	if a.runner == nil {
		return errors.New("telegram: polling mode but no runner is configured")
	}
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		a.runner.Start(a.loopCtx)
	}()
	return nil
}

func (a *Adapter) stopWebhook(ctx context.Context) error {
	_, delErr := a.client.DeleteWebhook(ctx, &bot.DeleteWebhookParams{
		DropPendingUpdates: false,
	})
	if delErr != nil {
		a.cfg.logger.WarnContext(ctx,
			"telegram: DeleteWebhook at shutdown failed",
			"error", delErr.Error())
	}
	if a.httpServer != nil {
		if err := a.httpServer.Shutdown(ctx); err != nil {
			a.cfg.logger.WarnContext(ctx,
				"telegram: HTTP server shutdown returned an error",
				"error", err.Error())
		}
	}
	if a.loopCancel != nil {
		a.loopCancel()
	}
	return nil
}

func (a *Adapter) stopPolling(_ context.Context) error {
	if a.loopCancel != nil {
		a.loopCancel()
	}
	return nil
}
