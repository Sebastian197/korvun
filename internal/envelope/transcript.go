// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package envelope

import "strings"

// TranscriptText renders parts for the conversation transcript
// (operator-console spec FR-ATTACH): an honest marker per non-text part —
// "[image]", "[audio]", "[video]", "[file]" — followed by the latest
// non-empty text, so a media message never persists as a mute void and the
// operator always sees WHAT arrived. Marker order follows part order; the
// text is the same latest-non-empty rule the request builders use. Media
// rendering itself is explicitly post-beta.
func TranscriptText(parts []Part) string {
	var pieces []string
	text := ""
	for _, p := range parts {
		switch p.Type {
		case Text:
			if strings.TrimSpace(p.Content) != "" {
				text = p.Content
			}
		case Image:
			pieces = append(pieces, "[image]")
		case Audio:
			pieces = append(pieces, "[audio]")
		case Video:
			pieces = append(pieces, "[video]")
		case File:
			pieces = append(pieces, "[file]")
		}
	}
	if text != "" {
		pieces = append(pieces, text)
	}
	return strings.Join(pieces, " ")
}
