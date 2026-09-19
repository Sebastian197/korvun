// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"fmt"
	"strings"
	"unicode"
)

// escapeUntrusted prints stored bytes no human typed into this terminal in the
// SAME alphabet the approvals screen uses (Approvals.tsx, escapeUntrusted): a
// code point a reader cannot see for what it is becomes a visible `<U+XXXX>`,
// and '<' itself is printed as `<U+003C>` so untrusted text cannot spell an
// escape. Without it «pagar100 EUR» and «pagar<U+2060>100 EUR», which seal
// different digests, print the same line (v0.15.1 block B, P2-1 sister).
//
// The class, as on the screen: controls (Cc), format characters (Cf),
// surrogates (Cs), every separator except the ASCII space (Zs, Zl, Zp) and the
// default-ignorable code points. What it does NOT cover, as on the screen:
// characters with a glyph, even a blank one, and look-alikes.
func escapeUntrusted(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '<':
			b.WriteString("<U+003C>")
		case r != ' ' && unseen(r):
			fmt.Fprintf(&b, "<U+%04X>", r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// unseen reports membership in the screen's class. Go's unicode package has no
// Default_Ignorable_Code_Point table; Unicode derives it as
// Other_Default_Ignorable_Code_Point + Cf + Variation_Selector minus code
// points that are all Cf or Zs, so the union unseen tests is the screen's
// class as far as the Unicode version of this toolchain's tables defines it
// (U+034F and the Hangul fillers are Other_Default_Ignorable_Code_Point). A
// code point assigned by a later Unicode version than Go's tables may be
// classed differently here and on the screen.
func unseen(r rune) bool {
	return unicode.In(r, unicode.Cc, unicode.Cf, unicode.Cs, unicode.Zs, unicode.Zl, unicode.Zp,
		unicode.Other_Default_Ignorable_Code_Point, unicode.Variation_Selector)
}
