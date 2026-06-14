// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package telegram

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-telegram/bot/models"
)

// secretTokenHeader is the HTTP header Telegram populates on every
// webhook request with the configured secret. The library's own
// WebhookHandler validates this header with == and responds
// silently on mismatch; the adapter uses crypto/subtle and a 401
// for both security and operational reasons (ADR-0008 §3).
//
// This is the header NAME, not a credential — gosec G101 is
// silenced accordingly.
const secretTokenHeader = "X-Telegram-Bot-Api-Secret-Token" // #nosec G101 -- HTTP header name, not a credential

// maxWebhookBodyBytes caps the request body the webhook handler
// will read. Telegram's documented update payloads sit well below
// 1 MiB; a hard cap defuses memory-exhaustion attempts from a
// misbehaving or malicious peer (ADR-0008 §3).
const maxWebhookBodyBytes = 1 << 20

// webhookHandler returns the http.HandlerFunc the adapter mounts
// at WithWebhookPath when running in ModeWebhook. The handler is
// hand-rolled rather than delegated to bot.Bot.WebhookHandler() so
// the secret-token comparison stays constant-time and rejection
// responses are explicit HTTP statuses, per ADR-0008 §3.
//
// Response codes:
//
//   - 200 OK on success, on InboundFromUpdate's silent-skip cases
//     (ErrNoMessage / ErrUnsupportedContent), and on saturation
//     drop. Acknowledging on saturation is deliberate: a non-2xx
//     would push Telegram into exponential backoff and risk the
//     webhook being removed entirely. ADR-0008 §4c discusses the
//     trade-off in depth.
//   - 401 Unauthorized when the X-Telegram-Bot-Api-Secret-Token
//     header is absent or does not match the configured secret.
//   - 405 Method Not Allowed on anything other than POST.
//   - 400 Bad Request on body decode failure or oversized body.
func (a *Adapter) webhookHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		got := r.Header.Get(secretTokenHeader)
		if subtle.ConstantTimeCompare([]byte(got), []byte(a.cfg.secretToken)) != 1 {
			a.cfg.logger.WarnContext(r.Context(),
				"telegram: rejected webhook with invalid secret token",
				"remote_addr", r.RemoteAddr,
				"path", r.URL.Path)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxWebhookBodyBytes))
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(body) == maxWebhookBodyBytes {
			// Indistinguishable from a truncated payload at this seam;
			// reject so we never feed a partial Update into the
			// converter.
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var u models.Update
		if err := json.Unmarshal(body, &u); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		a.dispatchUpdate(r.Context(), &u)
		w.WriteHeader(http.StatusOK)
	}
}
