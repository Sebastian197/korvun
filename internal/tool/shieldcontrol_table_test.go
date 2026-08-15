// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package tool

import (
	"errors"
	"testing"
)

// Estreno E-8 + E-13: the shield's classification, pinned as a table over
// every address class it claims to handle. Link-local is EXCLUDED from the
// private set (the cloud metadata service 169.254.169.254 / fe80::/10 is a
// credential-theft SSRF target that DNS rebinding can reach through an
// allow-listed hostname); the shield's contract is loopback + RFC1918 ULA
// space only.

func TestShieldControl_classificationTable(t *testing.T) {
	t.Parallel()
	allow := []string{
		"127.0.0.1:80", "[::1]:80",
		"192.168.1.1:80", "10.0.0.7:443", "172.16.3.4:80",
		"[fd00::1]:80",
		"[::ffff:192.168.1.1]:80", // IPv4-mapped private must classify as private
	}
	deny := []string{
		"8.8.8.8:80", "[2001:4860::8888]:80",
		"[::ffff:8.8.8.8]:80", // IPv4-mapped public
		"169.254.169.254:80",  // cloud metadata — the E-8 target
		"169.254.1.1:80",      // any IPv4 link-local
		"[fe80::1]:80",        // IPv6 link-local
		"garbage-address",     // unparseable: fail closed
	}
	for _, addr := range allow {
		if err := shieldControl("tcp", addr, nil); err != nil {
			t.Errorf("shieldControl(%q) = %v, want allow", addr, err)
		}
	}
	for _, addr := range deny {
		err := shieldControl("tcp", addr, nil)
		if err == nil {
			t.Errorf("shieldControl(%q) allowed, want ErrShieldViolation", addr)
			continue
		}
		if !errors.Is(err, ErrShieldViolation) {
			t.Errorf("shieldControl(%q) = %v, want errors.Is(_, ErrShieldViolation)", addr, err)
		}
	}
}
