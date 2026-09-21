// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bufio"
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestIdentity_SecretsNeverEnterEvidenceOrFeeds(t *testing.T) {
	const (
		acceptedBearer = "CANARY-ACCEPTED-BEARER-c338"
		rejectedHeader = "CANARY-REJECTED-HEADER-5b21"
		promptCanary   = "CANARY-PROMPT-e679"
		payloadCanary  = "CANARY-PAYLOAD-f10a"
	)
	t.Setenv("KORVUN_TEST_TOKEN", acceptedBearer)
	var logs bytes.Buffer
	channel := newFakeChannel("telegram")
	built, err := Build(cfgWith(ollamaBrain()),
		WithLogger(slog.New(slog.NewJSONHandler(&logs, nil))),
		withChannelFactory(okFactory(channel)))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- built.Run(ctx) }()
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			cancel()
			<-done
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer shutdownCancel()
			_ = built.Shutdown(shutdownCtx)
		})
	}
	t.Cleanup(stop)
	base := func() string { return "http://" + built.adminServer.Addr() }
	if !waitFor(t, func() bool {
		if built.adminServer.Addr() == "" {
			return false
		}
		code, _ := tryGet(base() + "/healthz")
		return code == http.StatusOK
	}) {
		t.Fatal("admin server never became healthy")
	}
	req, err := http.NewRequest(http.MethodGet, base()+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	frames := make(chan string, 8)
	reader := bufio.NewReader(resp.Body)
	go func() {
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				return
			}
			if strings.HasPrefix(line, "data: ") {
				frames <- line
			}
		}
	}()
	message := inboundText("telegram", "identity-canary", promptCanary+" "+payloadCanary)
	deadline := time.After(10 * time.Second)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	var frame string
	for frame == "" {
		select {
		case candidate := <-frames:
			if strings.Contains(candidate, `"type":"message_received"`) {
				frame = candidate
			}
		case <-ticker.C:
			select {
			case channel.inbound <- message:
			default:
			}
		case <-deadline:
			t.Fatal("no message_received SSE frame")
		}
	}
	_, metricsText := getEventually(t, base()+"/metrics")
	stop()
	surfaces := map[string]string{
		"structured logs": logs.String(),
		"metrics":         metricsText,
		"SSE":             frame,
	}
	for surface, content := range surfaces {
		for _, canary := range []string{acceptedBearer, rejectedHeader, promptCanary, payloadCanary} {
			if strings.Contains(content, canary) {
				t.Fatalf("%s contains %q", surface, canary)
			}
		}
	}
}
