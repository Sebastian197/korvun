// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package action tests — Trust Layer Etapa 1, Lote 1 (spec
// 2026-08-30-action-kernel.md, FR-DOM-3): the deterministic
// canonicalization and digest contract. These tests are the approved RED:
// once reviewed they are a contract and are not edited to make an
// implementation pass.

package action

import (
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"
)

// op returns the reference operation used across digest tests.
func op() Operation {
	return Operation{Namespace: "tool", Name: "webhook_call", Version: 1}
}

// serializeInOrder hand-builds a JSON object with keys in the GIVEN order —
// the helper that lets tests produce two different serializations of the
// same logical value (encoding/json always sorts, so the permutations must
// be built manually).
func serializeInOrder(keys []string, values map[string]string) string {
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%q:%s", k, values[k]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func TestCanonicalParams_keyOrderAndWhitespaceDoNotMatter(t *testing.T) {
	t.Parallel()
	a := `{"b":1, "a":{"y":2,"x":3}}`
	b := "{\n  \"a\": {\"x\":3, \"y\":2},\n  \"b\": 1\n}"
	if got, want := string(CanonicalParams(a)), string(CanonicalParams(b)); got != want {
		t.Fatalf("canonical form must ignore key order and whitespace:\n a=%q\n b=%q", got, want)
	}
	if Digest(op(), a) != Digest(op(), b) {
		t.Fatal("the same logical action must produce the same digest")
	}
}

func TestCanonicalParams_numericLiteralsSurviveVerbatim(t *testing.T) {
	t.Parallel()
	big := `{"n":90071992547409919999}`
	if !strings.Contains(string(CanonicalParams(big)), "90071992547409919999") {
		t.Fatalf("big numeric literal must survive canonicalization verbatim, got %q", CanonicalParams(big))
	}
	other := `{"n":90071992547409920000}`
	if Digest(op(), big) == Digest(op(), other) {
		t.Fatal("different numeric literals must produce different digests")
	}
}

func TestCanonicalParams_duplicateKeysLastWins(t *testing.T) {
	t.Parallel()
	if got, want := string(CanonicalParams(`{"a":1,"a":2}`)), string(CanonicalParams(`{"a":2}`)); got != want {
		t.Fatalf("duplicate keys resolve last-wins (documented): got %q want %q", got, want)
	}
}

func TestCanonicalParams_nonJSONIsRawBytes(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"", "hello world", `{"a":1} trailing`, "{broken"} {
		if got := string(CanonicalParams(raw)); got != raw {
			t.Fatalf("non-JSON args canonicalize as raw bytes verbatim: %q -> %q", raw, got)
		}
	}
}

func TestCanonicalParams_scalarsAndUnicode(t *testing.T) {
	t.Parallel()
	if got := string(CanonicalParams("  5 ")); got != "5" {
		t.Fatalf("a lone JSON scalar canonicalizes to its minimal form, got %q", got)
	}
	if a, b := CanonicalParams(`{"s":"é"}`), CanonicalParams(`{"s":"é"}`); string(a) != string(b) {
		t.Fatalf("escaped and literal unicode must canonicalize identically: %q vs %q", a, b)
	}
}

func TestCanonicalParams_arrayOrderIsMeaningful(t *testing.T) {
	t.Parallel()
	if Digest(op(), `[1,2]`) == Digest(op(), `[2,1]`) {
		t.Fatal("array order is data: permuted arrays must digest differently")
	}
}

func TestCanonicalParams_idempotent(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`{"b":1,"a":[1,{"z":true,"y":null}]}`, "plain", `"str"`} {
		once := CanonicalParams(raw)
		twice := CanonicalParams(string(once))
		if string(once) != string(twice) {
			t.Fatalf("canonicalization must be idempotent for %q: %q vs %q", raw, once, twice)
		}
	}
}

func TestDigest_shapeAndAlgorithmPin(t *testing.T) {
	t.Parallel()
	d := Digest(op(), `{"a":1}`)
	if !strings.HasPrefix(d, "sha256:") {
		t.Fatalf("digest must pin its algorithm in the string, got %q", d)
	}
	if hexLen := len(strings.TrimPrefix(d, "sha256:")); hexLen != 64 {
		t.Fatalf("sha256 hex must be 64 chars, got %d", hexLen)
	}
}

func TestDigest_everyDimensionOfTheOperationMatters(t *testing.T) {
	t.Parallel()
	base := Digest(Operation{Namespace: "tool", Name: "echo", Version: 1}, `{"a":1}`)
	for name, other := range map[string]string{
		"namespace": Digest(Operation{Namespace: "mcp", Name: "echo", Version: 1}, `{"a":1}`),
		"name":      Digest(Operation{Namespace: "tool", Name: "calc", Version: 1}, `{"a":1}`),
		"version":   Digest(Operation{Namespace: "tool", Name: "echo", Version: 2}, `{"a":1}`),
		"args":      Digest(Operation{Namespace: "tool", Name: "echo", Version: 1}, `{"a":2}`),
	} {
		if other == base {
			t.Fatalf("changing %s must change the digest", name)
		}
	}
}

func TestDigest_lengthPrefixingPreventsBoundaryCollisions(t *testing.T) {
	t.Parallel()
	a := Digest(Operation{Namespace: "a", Name: "bc", Version: 1}, "d")
	b := Digest(Operation{Namespace: "ab", Name: "c", Version: 1}, "d")
	c := Digest(Operation{Namespace: "a", Name: "b", Version: 1}, "cd")
	if a == b || a == c || b == c {
		t.Fatalf("field boundaries must be unambiguous: %q %q %q", a, b, c)
	}
}

// TestDigest_propertyPermutationStableMutationSensitive is the spec's
// property test (AS-3): random flat objects, serialized in two random key
// orders, digest identically; mutating any single value changes the digest.
// Seeded rand keeps the run deterministic.
func TestDigest_propertyPermutationStableMutationSensitive(t *testing.T) {
	t.Parallel()
	// #nosec G404 -- seeded math/rand ON PURPOSE: the property test must be
	// deterministic and reproducible; this randomness is not security material.
	rng := rand.New(rand.NewSource(20260830))
	for round := 0; round < 200; round++ {
		n := 2 + rng.Intn(6)
		keys := make([]string, 0, n)
		values := map[string]string{}
		for i := 0; i < n; i++ {
			k := fmt.Sprintf("k%02d", i)
			keys = append(keys, k)
			values[k] = fmt.Sprintf("%d", rng.Intn(1000))
		}
		sorted := append([]string(nil), keys...)
		sort.Strings(sorted)
		shuffled := append([]string(nil), keys...)
		rng.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

		one := Digest(op(), serializeInOrder(sorted, values))
		two := Digest(op(), serializeInOrder(shuffled, values))
		if one != two {
			t.Fatalf("round %d: permuted serializations of the same object must digest identically", round)
		}

		mutated := map[string]string{}
		for k, v := range values {
			mutated[k] = v
		}
		victim := keys[rng.Intn(len(keys))]
		mutated[victim] = mutated[victim] + "1"
		if Digest(op(), serializeInOrder(sorted, mutated)) == one {
			t.Fatalf("round %d: mutating %q must change the digest", round, victim)
		}
	}
}
