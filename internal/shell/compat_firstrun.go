// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// This file is the N1 engine half (spec 2026-08-29-n1-onboarding-gateway.md):
// the compat-branch model probe and the first-run template rewrite. No
// secret value ever appears in an outcome — the Bearer is read from the
// environment, used, and never echoed.

// CompatModelCheck is CheckCompatModel's outcome — results the onboarding
// paints, never Go errors, and never a secret value.
type CompatModelCheck struct {
	Reachable  bool   `json:"reachable"`
	ModelFound bool   `json:"modelFound"`
	NeedsKey   bool   `json:"needsKey"`
	Detail     string `json:"detail"`
}

// CheckCompatModel probes {baseURL}/models (the OpenAI-compatible list
// shape) and reports whether the server answers AND the chosen model id
// exists in its list (FR-N1-1). When apiKeyEnv names a variable present in
// the environment its value rides as the Bearer; a 401/403 answer flips
// needsKey — the onboarding's bridge to the Secrets panel (B10).
func (d *Desktop) CheckCompatModel(baseURL, modelID, apiKeyEnv string) CompatModelCheck {
	ctx, cancel := context.WithTimeout(context.Background(), d.deadlines.check)
	defer cancel()
	url := strings.TrimSuffix(baseURL, "/") + "/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return CompatModelCheck{Detail: "invalid base URL"}
	}
	if apiKeyEnv != "" {
		if v := os.Getenv(apiKeyEnv); v != "" {
			req.Header.Set("Authorization", "Bearer "+v)
		}
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return CompatModelCheck{Detail: fmt.Sprintf("not reachable: %v", err)}
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
		return CompatModelCheck{Reachable: true, NeedsKey: true,
			Detail: fmt.Sprintf("auth required (status %d)", resp.StatusCode)}
	case resp.StatusCode != http.StatusOK:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
		return CompatModelCheck{Reachable: true, Detail: fmt.Sprintf("unexpected status %d", resp.StatusCode)}
	}
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&list); err != nil {
		return CompatModelCheck{Reachable: true, Detail: "unparseable /models answer"}
	}
	for _, m := range list.Data {
		if m.ID == modelID {
			return CompatModelCheck{Reachable: true, ModelFound: true, Detail: "model present"}
		}
	}
	return CompatModelCheck{Reachable: true,
		Detail: fmt.Sprintf("model %q not in /models (%d listed)", modelID, len(list.Data))}
}

// ApplyCompatFirstRun rewrites the first-run template's brain to a single
// openai-compatible entry (FR-N1-2): locality declared, api_key_env only
// when non-empty. The whole result must pass config.Validate BEFORE the
// atomic write — a refused shape leaves the on-disk template untouched.
// The onboarding is its only caller; it governs the first-run template only.
func (d *Desktop) ApplyCompatFirstRun(baseURL, modelID, apiKeyEnv, locality string) error {
	_, err := bounded(d, "apply compat first-run", d.deadlines.read, func() (struct{}, error) {
		path, err := d.secretsConfigPath()
		if err != nil {
			return struct{}{}, err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return struct{}{}, fmt.Errorf("shell: read first-run config %q: %w", path, err)
		}
		var cfg config.Config
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return struct{}{}, fmt.Errorf("shell: parse first-run config %q: %w", path, err)
		}
		if len(cfg.Brains) == 0 {
			return struct{}{}, errors.New("shell: first-run config has no brain to rewrite")
		}
		m := config.ModelConfig{
			Provider: "openai-compatible",
			ModelID:  modelID,
			Locality: locality,
			BaseURL:  baseURL,
		}
		if apiKeyEnv != "" {
			m.APIKeyEnv = apiKeyEnv
		}
		cfg.Brains[0].Models = []config.ModelConfig{m}
		if err := cfg.Validate(); err != nil {
			return struct{}{}, fmt.Errorf("shell: compat first-run shape refused: %w", err)
		}
		if err := supervisor.WriteConfigAtomic(path, &cfg); err != nil {
			return struct{}{}, fmt.Errorf("shell: write first-run config %q: %w", path, err)
		}
		return struct{}{}, nil
	})
	return err
}
