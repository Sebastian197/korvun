// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package executor is the Action Kernel's execution seam (Trust Layer
// Etapa 1, spec FR-EXEC-1, lote 3): the ONLY component authorized to
// invoke a tool's Execute/ExecuteScoped. It preserves runTool's execution
// semantics exactly — per-tool timeout, optional scope, latency
// measurement — and nothing else: decisions, observations, audit and
// persistence stay with the caller (the brain adapter). The tripwire test
// beside this file keeps the single-path invariant enforced by machine.
package executor

import (
	"context"
	"errors"
	"time"

	"github.com/Sebastian197/korvun/internal/tool"
)

// ErrUnknownTool reports a name absent from the registry. Callers are
// expected to check Has first (the honest unknown-tool observation comes
// before governance), so this is a defensive sentinel, not a UX surface.
var ErrUnknownTool = errors.New("executor: unknown tool")

// Executor wraps the read-only tool.Registry as the single execution path.
type Executor struct {
	tools   tool.Registry
	perTool time.Duration
	now     func() time.Time
}

// New builds the executor over the brain's registry. perTool of zero means
// no per-tool timeout (today's semantics, verbatim); now is the injected
// clock the latency measurement uses.
func New(tools tool.Registry, perTool time.Duration, now func() time.Time) *Executor {
	return &Executor{tools: tools, perTool: perTool, now: now}
}

// Has reports whether name exists in the registry.
func (e *Executor) Has(name string) bool {
	_, ok := e.tools[name]
	return ok
}

// Run executes the named tool: per-tool timeout when configured, the
// scope-aware path when the tool implements ScopedTool (same rule as the
// pre-kernel runTool), and the measured latency either way. The returned
// error is the tool's own — classification (cages, observations) belongs
// to the caller.
func (e *Executor) Run(ctx context.Context, name string, scope tool.Scope, args string) (string, time.Duration, error) {
	t, ok := e.tools[name]
	if !ok {
		return "", 0, ErrUnknownTool
	}
	toolCtx := ctx
	if e.perTool > 0 {
		var cancel context.CancelFunc
		toolCtx, cancel = context.WithTimeout(ctx, e.perTool)
		defer cancel()
	}
	start := e.now()
	var result string
	var err error
	if st, scoped := t.(tool.ScopedTool); scoped {
		result, err = st.ExecuteScoped(toolCtx, scope, args)
	} else {
		result, err = t.Execute(toolCtx, args)
	}
	return result, e.now().Sub(start), err
}
