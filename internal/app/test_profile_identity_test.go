// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"os"
	"path/filepath"
)

// testProfilePath is the profile file the package's tests boot for when the
// test is not about a particular profile: a handle is born with its profile
// (the durable mark's redesign, R3), so every Build with storage names one.
var testProfilePath = filepath.Join(os.TempDir(), "korvun-test-profile", "korvun.json")

// testProfileIdentity is testProfilePath's identity, the one the tests open
// their ledgers for directly.
var testProfileIdentity = ProfileIdentity(testProfilePath)

// withTestProfile is the option the tests add to a Build that names no
// profile of its own.
func withTestProfile() Option { return WithProfilePath(testProfilePath) }
