// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package tool

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// fixedClock returns a deterministic time so the Time tool is testable.
func fixedClock() time.Time { return time.Date(2026, 6, 27, 12, 0, 0, 0, time.UTC) }

func TestEcho(t *testing.T) {
	t.Parallel()
	e := Echo()
	if e.Name() != "echo" {
		t.Fatalf("Name = %q, want echo", e.Name())
	}
	if e.Description() == "" {
		t.Fatal("Description must be non-empty (advertised in the system prompt)")
	}
	got, err := e.Execute(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got != "hello world" {
		t.Fatalf("Execute = %q, want verbatim echo", got)
	}
}

func TestTime(t *testing.T) {
	t.Parallel()
	tt := Time(fixedClock)
	if tt.Name() != "time" {
		t.Fatalf("Name = %q, want time", tt.Name())
	}
	got, err := tt.Execute(context.Background(), "args ignored")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	// The fixed clock is already UTC; the tool must format it in RFC3339 UTC.
	if got != "2026-06-27T12:00:00Z" {
		t.Fatalf("Execute = %q, want the fixed UTC clock in RFC3339", got)
	}
}

// TestTime_nilClock proves the Time tool defaults to a real clock rather than
// panicking on a nil func (a misconfigured registry must never crash the loop).
func TestTime_nilClock(t *testing.T) {
	t.Parallel()
	tt := Time(nil)
	got, err := tt.Execute(context.Background(), "")
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if _, perr := time.Parse(time.RFC3339, got); perr != nil {
		t.Fatalf("Execute = %q, want a parseable RFC3339 time: %v", got, perr)
	}
}

func TestCalc(t *testing.T) {
	t.Parallel()
	c := Calc()
	if c.Name() != "calc" {
		t.Fatalf("Name = %q, want calc", c.Name())
	}

	tests := []struct {
		name string
		args string
		want string
	}{
		{"add", "2+2", "4"},
		{"precedence", "2+2*3", "8"},
		{"subtract", "10-3-2", "5"},
		{"divide", "10/4", "2.5"},
		{"parens", "(2+3)*4", "20"},
		{"unary minus", "-5+8", "3"},
		{"whitespace", "  7  *  6 ", "42"},
		{"float", "1.5+1.5", "3"},
		{"nested parens", "((1+2)*(3+4))", "21"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := c.Execute(context.Background(), tc.args)
			if err != nil {
				t.Fatalf("Execute(%q): %v", tc.args, err)
			}
			if got != tc.want {
				t.Fatalf("Execute(%q) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// TestCalc_errors proves calc fails loud (an OBSERVATION error, ADR-0021 §2) on
// anything outside its bounded arithmetic grammar — it is NOT a general
// expression evaluator (ADR-0021 §8 security note). No identifiers, no calls, no
// exponentiation, no eval.
func TestCalc_errors(t *testing.T) {
	t.Parallel()
	c := Calc()

	bad := []struct {
		name string
		args string
	}{
		{"empty", ""},
		{"blank", "   "},
		{"divide by zero", "1/0"},
		{"trailing garbage", "2+2 foo"},
		{"bare identifier", "x"},
		{"function call", "sqrt(4)"},
		{"unbalanced paren", "(2+3"},
		{"double operator", "2++2"},
		{"exponent rejected", "2^8"},
		{"letters", "abc"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := c.Execute(context.Background(), tc.args)
			if err == nil {
				t.Fatalf("Execute(%q) = %q, want an error (calc is bounded, not eval)", tc.args, got)
			}
		})
	}
}

// TestCalc_concurrent exercises the Tool concurrency contract (ADR-0021 §4) on
// the pure built-in: many goroutines share ONE Calc instance. Run with -race.
func TestCalc_concurrent(t *testing.T) {
	t.Parallel()
	c := Calc()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := c.Execute(context.Background(), "2+2*3")
			if err != nil || got != "8" {
				t.Errorf("concurrent Execute = %q, %v; want 8", got, err)
			}
		}()
	}
	wg.Wait()
}

// TestBuiltin resolves the built-in tools by name (the single source of truth
// the config wiring uses) and rejects unknown names.
func TestBuiltin(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"time", "echo", "calc"} {
		tl, ok := Builtin(name)
		if !ok {
			t.Fatalf("Builtin(%q) not found", name)
		}
		if tl.Name() != name {
			t.Fatalf("Builtin(%q).Name() = %q", name, tl.Name())
		}
	}
	if _, ok := Builtin("shell"); ok {
		t.Fatal("Builtin(\"shell\") resolved — dangerous tools must NOT be built in")
	}
	if _, ok := Builtin(""); ok {
		t.Fatal("Builtin(\"\") resolved")
	}
}

// TestBuiltinNames lists every built-in, sorted, for help/validation output.
func TestBuiltinNames(t *testing.T) {
	t.Parallel()
	names := BuiltinNames()
	want := []string{"calc", "echo", "time"}
	if len(names) != len(want) {
		t.Fatalf("BuiltinNames() = %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("BuiltinNames()[%d] = %q, want %q (sorted)", i, names[i], want[i])
		}
	}
}

// TestDescriptionsNonEmpty guards that every built-in advertises itself.
func TestDescriptionsNonEmpty(t *testing.T) {
	t.Parallel()
	for _, tl := range []Tool{Echo(), Time(nil), Calc()} {
		if strings.TrimSpace(tl.Description()) == "" {
			t.Fatalf("%s has an empty Description", tl.Name())
		}
	}
}
