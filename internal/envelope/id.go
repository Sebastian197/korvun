// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package envelope

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"
)

// counter provides sub-millisecond ordering when multiple IDs are generated
// within the same timestamp.
var counter uint64

// NewID generates a unique, time-sortable identifier with no external
// dependencies. Format: <unix-ms-hex>-<counter-hex>-<random-hex>.
func NewID() string {
	ms := time.Now().UnixMilli()
	seq := atomic.AddUint64(&counter, 1)

	var buf [4]byte
	_, _ = rand.Read(buf[:])
	rnd := hex.EncodeToString(buf[:])

	return fmt.Sprintf("%013x-%04x-%s", ms, seq, rnd)
}
