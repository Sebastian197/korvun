// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Native fuzz targets (spec FR-DOM-5, the director's 2026-08-30 addendum):
// the kernel's parsers are born fuzzed. The seed corpus below is the
// spec's own edge list; a short smoke of these targets gates make quality,
// and long campaigns run as the documented manual/nightly task.

package action

import (
	"bytes"
	"testing"
)

// canonicalSeeds is the spec's edge list, versioned in the tree as the
// seed corpus for both targets.
var canonicalSeeds = []string{
	``,
	`hello world`,
	`{"a":1}`,
	`{"b":1, "a":{"y":2,"x":3}}`,
	`{"n":90071992547409919999}`,
	`{"a":1,"a":2}`,
	`{"a":1} trailing`,
	`{broken`,
	`  5 `,
	`"str"`,
	`true`,
	`null`,
	`[2,1,[3,{"z":true}]]`,
	`{"s":"é"}`,
	`{"s":"é"}`,
	`{"deep":{"deep":{"deep":{"deep":{"deep":1}}}}}`,
	"\x00\xff\xfe invalid utf8 \x80",
}

// FuzzCanonicalize pins the canonicalizer's safety net: arbitrary input
// never panics, canonicalization is deterministic (same input, same
// bytes) and idempotent (canonical output re-canonicalizes to itself).
func FuzzCanonicalize(f *testing.F) {
	for _, seed := range canonicalSeeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		once := CanonicalParams(raw)
		again := CanonicalParams(raw)
		if !bytes.Equal(once, again) {
			t.Fatalf("canonicalization must be deterministic for %q: %q vs %q", raw, once, again)
		}
		twice := CanonicalParams(string(once))
		if !bytes.Equal(once, twice) {
			t.Fatalf("canonicalization must be idempotent for %q: %q vs %q", raw, once, twice)
		}
	})
}

// FuzzDigest pins the digest under arbitrary operation tuples: stable for
// the same tuple, and immune to boundary shifting — moving bytes between
// the length-prefixed fields must never produce the same digest.
func FuzzDigest(f *testing.F) {
	for _, seed := range canonicalSeeds {
		f.Add("tool", "webhook_call", seed)
	}
	f.Add("a", "bc", "d")
	f.Add("ab", "c", "d")
	f.Fuzz(func(t *testing.T, namespace, name, args string) {
		op := Operation{Namespace: namespace, Name: name, Version: 1}
		one := Digest(op, args)
		if two := Digest(op, args); one != two {
			t.Fatalf("digest must be stable for the same tuple, got %q vs %q", one, two)
		}
		if len(namespace) > 0 {
			// Shift the namespace/name boundary by one byte: a different
			// tuple, so the length-prefixed concatenation MUST differ.
			shifted := Digest(Operation{
				Namespace: namespace[:len(namespace)-1],
				Name:      namespace[len(namespace)-1:] + name,
				Version:   1,
			}, args)
			if shifted == one {
				t.Fatalf("boundary shift must never collide: ns=%q name=%q", namespace, name)
			}
		}
	})
}
