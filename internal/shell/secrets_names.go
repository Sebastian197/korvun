// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/Sebastian197/korvun/internal/config"
)

// This file is the B10 Secrets-panel engine half (spec
// 2026-08-29-b10-secrets-panel.md): NAME discovery from the config file and
// the [Abrir carpeta] opener. Values never transit here by construction —
// the return types cannot carry one.

// secretsConfigPath resolves the config file the panel reads names from:
// the controller's loaded path, or the fallback (DefaultConfigPath) when
// nothing is loaded — the star case is a boot that FAILED on a broken
// config, which leaves the controller path-less exactly when the panel is
// needed most.
func (d *Desktop) secretsConfigPath() (string, error) {
	if p := d.ctrl.Status().ConfigPath; p != "" {
		return p, nil
	}
	return d.configPathFallback()
}

// ListSecretNames returns the deduplicated, appearance-ordered list of
// secret env-var NAMES the config references (FR-B10-1): channel token_env,
// webhook outbound_token_env, model api_key_env, admin token_env.
// PARSE-ONLY on purpose — a config that parses but fails validation still
// yields useful rows; an unreadable or unparseable file is the error the
// panel's sealed notice paints.
func (d *Desktop) ListSecretNames() ([]string, error) {
	return bounded(d, "list secret names", d.deadlines.read, func() ([]string, error) {
		path, err := d.secretsConfigPath()
		if err != nil {
			return nil, err
		}
		// #nosec G304 -- path is the desktop's own config location (the
		// controller's loaded path or DefaultConfigPath), never user input.
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("shell: read config %q: %w", path, err)
		}
		var cfg config.Config
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, fmt.Errorf("shell: parse config %q: %w", path, err)
		}
		return collectSecretNames(&cfg), nil
	})
}

// collectSecretNames walks the parsed config in appearance order, skipping
// empties and duplicates. Pure.
func collectSecretNames(cfg *config.Config) []string {
	seen := make(map[string]bool)
	names := []string{}
	add := func(n string) {
		if n != "" && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	for _, ch := range cfg.Channels {
		add(ch.TokenEnv)
		if ch.Webhook != nil {
			add(ch.Webhook.OutboundTokenEnv)
		}
	}
	for _, b := range cfg.Brains {
		for _, m := range b.Models {
			add(m.APIKeyEnv)
		}
	}
	if cfg.Admin != nil {
		add(cfg.Admin.TokenEnv)
	}
	return names
}

// OpenConfigFolder opens the config file's directory in the OS file manager
// (FR-B10-2) — the sealed [Abrir carpeta] fix of the unreadable-config
// notice. The opener seam is injectable for tests.
func (d *Desktop) OpenConfigFolder() error {
	path, err := d.secretsConfigPath()
	if err != nil {
		return err
	}
	return d.folderOpener(filepath.Dir(path))
}

// defaultFolderOpener launches the platform file manager on dir.
func defaultFolderOpener(dir string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("shell: open config folder %q: %w", dir, err)
	}
	// Detach: the file manager owns its own lifetime.
	go func() { _ = cmd.Wait() }()
	return nil
}
