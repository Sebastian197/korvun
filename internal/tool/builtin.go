// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package tool

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// The three built-in tools of the Stage 8 seam-validation slice are ALL PURE:
// no I/O, no side effects, no external reach (ADR-0021 §8). That is a deliberate
// SECURITY decision, not a feature gap. A tool that can reach the network or the
// OS (shell, arbitrary HTTP, filesystem) turns a reasoning bug into a remote
// exploit, so the minimal cut proves the loop + the seam with the safest possible
// payload. Dangerous / side-effecting tools are deferred behind a much higher bar
// (their own ADR, with sandboxing / allow-listing / consent designed first).

// Builtin resolves a built-in tool by its protocol name. It is the single source
// of truth the config wiring (internal/app) uses to turn configured tool names
// into Tool values, so the safe-toolset boundary (ADR-0021 §8) lives in exactly
// one place: only time, echo, and calc resolve — a dangerous name like "shell"
// returns ok=false, never a tool.
func Builtin(name string) (Tool, bool) {
	switch name {
	case "time":
		return Time(nil), true
	case "echo":
		return Echo(), true
	case "calc":
		return Calc(), true
	default:
		return nil, false
	}
}

// BuiltinNames returns the names of every built-in tool, sorted, for help and
// config-validation messages.
func BuiltinNames() []string { return []string{"calc", "echo", "time"} }

// echoTool returns its args verbatim — the trivial loop-proving tool.
type echoTool struct{}

// Echo returns the verbatim-echo built-in tool.
func Echo() Tool { return echoTool{} }

func (echoTool) Name() string        { return "echo" }
func (echoTool) Description() string { return "returns its args verbatim. args = the text to echo." }

// Execute returns args unchanged. It cannot fail.
func (echoTool) Execute(_ context.Context, args string) (string, error) { return args, nil }

// timeTool returns the current UTC time. The clock is a seam so the tool is
// deterministically testable; a nil clock defaults to the real one.
type timeTool struct{ now func() time.Time }

// Time returns the UTC-clock built-in tool. A nil now defaults to time.Now, so a
// misconfigured registry can never crash the loop with a nil-func panic.
func Time(now func() time.Time) Tool {
	if now == nil {
		now = time.Now
	}
	return timeTool{now: now}
}

func (timeTool) Name() string        { return "time" }
func (timeTool) Description() string { return "returns the current UTC time in RFC3339. args ignored." }

// Execute returns the current time formatted as RFC3339 in UTC. args is ignored.
func (t timeTool) Execute(_ context.Context, _ string) (string, error) {
	return t.now().UTC().Format(time.RFC3339), nil
}

// calcTool evaluates a BOUNDED arithmetic expression with its OWN recursive-
// descent parser (ADR-0021 §8). It supports only numbers, the four operators
// (+ - * /), unary minus, and parentheses with standard precedence. It is NOT a
// general expression evaluator: no identifiers, no function calls, no
// exponentiation, no eval, and NO external dependency (go.mod stays at three
// direct deps). A "calculator" that evaluates arbitrary expressions is a security
// vector; this one cannot reach beyond arithmetic on the literals it is given.
type calcTool struct{}

// Calc returns the bounded-arithmetic built-in tool.
func Calc() Tool { return calcTool{} }

func (calcTool) Name() string { return "calc" }
func (calcTool) Description() string {
	return "evaluates a basic arithmetic expression (+ - * / and parentheses) over numbers. args = the expression, e.g. 2+2*3."
}

// Execute parses and evaluates the bounded arithmetic in args. A malformed
// expression returns an error, which the loop surfaces to the model as an
// OBSERVATION (ADR-0021 §2), never a panic.
func (calcTool) Execute(_ context.Context, args string) (string, error) {
	v, err := evalArithmetic(args)
	if err != nil {
		return "", err
	}
	// FormatFloat with -1 precision prints whole results without a decimal
	// point (4, 8, 20) and fractional ones minimally (2.5).
	return strconv.FormatFloat(v, 'f', -1, 64), nil
}

// errCalc is the bounded-calculator's sentinel; callers may match it with
// errors.Is, and every parse failure wraps it.
var errCalc = errors.New("calc: invalid expression")

// calcParser is a single-pass recursive-descent parser over an ASCII arithmetic
// string. Grammar (standard precedence):
//
//	expr   := term  (('+' | '-') term)*
//	term   := factor (('*' | '/') factor)*
//	factor := '-' factor | '(' expr ')' | number
//	number := digit+ ('.' digit+)?
type calcParser struct {
	s   string
	pos int
}

// evalArithmetic parses and evaluates a complete arithmetic expression, rejecting
// any trailing input (so "2+2 foo" fails loud rather than silently yielding 4).
func evalArithmetic(s string) (float64, error) {
	p := &calcParser{s: s}
	p.skipSpaces()
	if p.pos >= len(p.s) {
		return 0, fmt.Errorf("%w: empty", errCalc)
	}
	v, err := p.parseExpr()
	if err != nil {
		return 0, err
	}
	p.skipSpaces()
	if p.pos != len(p.s) {
		return 0, fmt.Errorf("%w: trailing %q", errCalc, p.s[p.pos:])
	}
	return v, nil
}

func (p *calcParser) skipSpaces() {
	for p.pos < len(p.s) && (p.s[p.pos] == ' ' || p.s[p.pos] == '\t') {
		p.pos++
	}
}

func (p *calcParser) parseExpr() (float64, error) {
	v, err := p.parseTerm()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpaces()
		if p.pos >= len(p.s) {
			break
		}
		op := p.s[p.pos]
		if op != '+' && op != '-' {
			break
		}
		p.pos++
		rhs, err := p.parseTerm()
		if err != nil {
			return 0, err
		}
		if op == '+' {
			v += rhs
		} else {
			v -= rhs
		}
	}
	return v, nil
}

func (p *calcParser) parseTerm() (float64, error) {
	v, err := p.parseFactor()
	if err != nil {
		return 0, err
	}
	for {
		p.skipSpaces()
		if p.pos >= len(p.s) {
			break
		}
		op := p.s[p.pos]
		if op != '*' && op != '/' {
			break
		}
		p.pos++
		rhs, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		if op == '/' {
			if rhs == 0 {
				return 0, fmt.Errorf("%w: divide by zero", errCalc)
			}
			v /= rhs
		} else {
			v *= rhs
		}
	}
	return v, nil
}

func (p *calcParser) parseFactor() (float64, error) {
	p.skipSpaces()
	if p.pos >= len(p.s) {
		return 0, fmt.Errorf("%w: unexpected end of expression", errCalc)
	}
	switch p.s[p.pos] {
	case '-': // unary minus (unary plus is intentionally NOT allowed, so "2++2" fails)
		p.pos++
		v, err := p.parseFactor()
		if err != nil {
			return 0, err
		}
		return -v, nil
	case '(':
		p.pos++
		v, err := p.parseExpr()
		if err != nil {
			return 0, err
		}
		p.skipSpaces()
		if p.pos >= len(p.s) || p.s[p.pos] != ')' {
			return 0, fmt.Errorf("%w: missing closing paren", errCalc)
		}
		p.pos++
		return v, nil
	default:
		return p.parseNumber()
	}
}

func (p *calcParser) parseNumber() (float64, error) {
	start := p.pos
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		if (c >= '0' && c <= '9') || c == '.' {
			p.pos++
			continue
		}
		break
	}
	if p.pos == start {
		return 0, fmt.Errorf("%w: expected a number at %q", errCalc, p.s[p.pos:])
	}
	v, err := strconv.ParseFloat(p.s[start:p.pos], 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", errCalc, err)
	}
	return v, nil
}
