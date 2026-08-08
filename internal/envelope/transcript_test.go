// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package envelope

import "testing"

func TestTranscriptText(t *testing.T) {
	cases := []struct {
		name  string
		parts []Part
		want  string
	}{
		{"text only", []Part{{Type: Text, Content: "hola"}}, "hola"},
		{"image with caption", []Part{{Type: Image, Source: "x"}, {Type: Text, Content: "mira esto"}}, "[image] mira esto"},
		{"image only", []Part{{Type: Image, Source: "x"}}, "[image]"},
		{"audio only", []Part{{Type: Audio, Source: "x"}}, "[audio]"},
		{"video only", []Part{{Type: Video, Source: "x"}}, "[video]"},
		{"file only", []Part{{Type: File, Source: "x"}}, "[file]"},
		{"two images", []Part{{Type: Image}, {Type: Image}}, "[image] [image]"},
		{"whitespace text with file", []Part{{Type: File}, {Type: Text, Content: "   "}}, "[file]"},
		{"empty", nil, ""},
		{"latest non-empty text wins", []Part{{Type: Text, Content: "vieja"}, {Type: Text, Content: "última"}}, "última"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TranscriptText(tc.parts); got != tc.want {
				t.Fatalf("TranscriptText = %q, want %q", got, tc.want)
			}
		})
	}
}
