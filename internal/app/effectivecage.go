// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The typed effective cage (R4 Phase 5, FR-R4F5): ONE resolver
// normalizes a brain's whole cage ONCE — effective attrs (house
// defaults + operator overrides), cage bounds with their declared
// defaults, allow-lists sorted, parsed sensitivity, ceiling, tools and
// governance — and everything downstream consumes THIS object: the law
// digest serializes its canonical view, and the boot and the deferred
// executor construct their tools from it. "One resolver, one verdict"
// as structural truth: agentTool cannot re-read raw BrainConfig.

package app

import (
	"fmt"
	"sort"

	"time"

	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/policy"
	"github.com/Sebastian197/korvun/internal/tool"
)

// EffectiveReadFileCage is the read_file cage, bounds resolved.
type EffectiveReadFileCage struct {
	Root     string
	MaxBytes int64
}

// EffectiveHTTPFetchCage is the http_fetch cage, bounds resolved and
// the allow-list sorted (set semantics).
type EffectiveHTTPFetchCage struct {
	AllowHosts   []string
	MaxBytes     int64
	MaxRedirects int
}

// EffectiveWebhookCage is the webhook_call cage, bounds resolved and
// the allow-list sorted.
type EffectiveWebhookCage struct {
	AllowHosts     []string
	MaxBytes       int64
	TimeoutSeconds int
}

// EffectiveCage is one brain's whole resolved cage.
type EffectiveCage struct {
	BrainName string
	// RawSensitivity keeps the operator's spelling for the canonical
	// digest view; Sensitivity is the parsed value the shield consumes.
	RawSensitivity string
	Sensitivity    policy.Sensitivity
	// HasAgent distinguishes a brain with no agent block at all.
	HasAgent      bool
	EffectCeiling string
	Tools         []string
	Governance    []config.ToolGrantConfig
	Attrs         map[string]policy.ToolAttrs
	ReadFile      *EffectiveReadFileCage
	HTTPFetch     *EffectiveHTTPFetchCage
	WebhookCall   *EffectiveWebhookCage
	Memory        *config.MemorySettings
}

// ResolveEffectiveCage normalizes one brain's cage ONCE. A config the
// boot refuses (unparseable sensitivity, an override naming an
// unlisted tool) is refused HERE — and therefore by every consumer.
func ResolveEffectiveCage(bc config.BrainConfig) (*EffectiveCage, error) {
	sens, err := parseSensitivity(bc.Sensitivity)
	if err != nil {
		return nil, fmt.Errorf("app: brain %q: %w", bc.Name, err)
	}
	cage := &EffectiveCage{
		BrainName:      bc.Name,
		RawSensitivity: bc.Sensitivity,
		Sensitivity:    sens,
	}
	a := bc.Agent
	if a == nil {
		return cage, nil
	}
	attrs, err := effectiveToolAttrs(a)
	if err != nil {
		return nil, fmt.Errorf("app: brain %q: %w", bc.Name, err)
	}
	cage.HasAgent = true
	cage.EffectCeiling = a.EffectCeiling
	cage.Tools = a.Tools
	cage.Governance = a.Governance
	cage.Attrs = attrs
	if a.ReadFile != nil {
		cage.ReadFile = &EffectiveReadFileCage{
			Root:     a.ReadFile.Root,
			MaxBytes: defaultedInt64(a.ReadFile.MaxBytes, tool.DefaultReadFileMaxBytes),
		}
	}
	if a.HTTPFetch != nil {
		cage.HTTPFetch = &EffectiveHTTPFetchCage{
			AllowHosts:   sortedCopy(a.HTTPFetch.AllowHosts),
			MaxBytes:     defaultedInt64(a.HTTPFetch.MaxBytes, tool.DefaultFetchMaxBytes),
			MaxRedirects: defaultedInt(a.HTTPFetch.MaxRedirects, tool.DefaultFetchMaxRedirects),
		}
	}
	if a.WebhookCall != nil {
		cage.WebhookCall = &EffectiveWebhookCage{
			AllowHosts:     sortedCopy(a.WebhookCall.AllowHosts),
			MaxBytes:       defaultedInt64(a.WebhookCall.MaxBytes, tool.DefaultWebhookMaxBytes),
			TimeoutSeconds: defaultedInt(a.WebhookCall.TimeoutSeconds, int(tool.DefaultWebhookTimeout/time.Second)),
		}
	}
	if a.Memory != nil {
		settings := a.Memory.Settings()
		cage.Memory = &settings
	}
	return cage, nil
}

func defaultedInt64(v, def int64) int64 {
	if v == 0 {
		return def
	}
	return v
}

func defaultedInt(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// canonicalView rebuilds the exact digest shape the pre-typed resolver
// serialized — the golden pin holds byte-for-byte across the change.
func (c *EffectiveCage) canonicalView() map[string]any {
	view := map[string]any{"sensitivity": c.RawSensitivity}
	if !c.HasAgent {
		return view
	}
	view["tools"] = c.Tools
	view["governance"] = c.Governance
	view["attrs"] = c.Attrs
	view["effect_ceiling"] = c.EffectCeiling
	if c.ReadFile != nil {
		view["read_file"] = map[string]any{
			"root":      c.ReadFile.Root,
			"max_bytes": c.ReadFile.MaxBytes,
		}
	}
	if c.HTTPFetch != nil {
		view["http_fetch"] = map[string]any{
			"allow_hosts":   c.HTTPFetch.AllowHosts,
			"max_bytes":     c.HTTPFetch.MaxBytes,
			"max_redirects": c.HTTPFetch.MaxRedirects,
		}
	}
	if c.WebhookCall != nil {
		view["webhook_call"] = map[string]any{
			"allow_hosts":     c.WebhookCall.AllowHosts,
			"max_bytes":       c.WebhookCall.MaxBytes,
			"timeout_seconds": c.WebhookCall.TimeoutSeconds,
		}
	}
	if c.Memory != nil {
		view["memory"] = *c.Memory
	}
	return view
}
