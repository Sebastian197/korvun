// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package brain

import (
	"context"

	"github.com/Sebastian197/korvun/internal/model"
)

// named binds a provider-specific model id to a model.Model. It exists
// because fanout.Run hands the SAME *model.Request to every goroutine
// (ADR-0011), while each provider in a heterogeneous fan-out needs its
// own Request.Model (Ollama "llama3.2" vs Groq "llama-3.3-70b").
//
// Generate gives the provider its id by SHALLOW-COPYING the request and
// overriding Model on the copy. It MUST NOT write req.Model: that shared
// pointer is read by every other goroutine concurrently, so a mutation
// would be a data race and would hand providers nondeterministic ids —
// the same "intersection of two features" bug class the project already
// hit (the Phase 2E.8 close-after-Wait race, the fan-out P2 clock race).
// A shallow copy is enough: adapters only READ Messages, never mutate the
// slice. (ADR-0014 §2.)
type named struct {
	inner model.Model
	id    string
}

// Generate copies req, sets Model to the bound id on the copy, and
// delegates. The shared req is never mutated.
func (n named) Generate(ctx context.Context, req *model.Request) (*model.Response, error) {
	cp := *req // shallow copy: new Model field, shared (read-only) Messages
	cp.Model = n.id
	return n.inner.Generate(ctx, &cp)
}

// Name returns the wrapped provider's name, so fan-out attribution stays
// the provider identity (e.g. "ollama"), not the model id.
func (n named) Name() string { return n.inner.Name() }

// WithModelID binds a model id to a model.Model for use in a fan-out set.
// Use it to assemble a heterogeneous set where each provider receives its
// own Request.Model without mutating the shared request.
func WithModelID(m model.Model, id string) model.Model {
	return named{inner: m, id: id}
}
