// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package shell

import "github.com/Sebastian197/korvun/internal/action"

// testProfileIdentity is the profile the package's tests open their ledgers
// for: a handle is born with its profile (the durable mark's redesign, R3),
// and a test that needs no particular profile uses this one.
var testProfileIdentity = action.HashCanonical(`{"profile":"/korvun/tests/shell/korvun.json"}`)
