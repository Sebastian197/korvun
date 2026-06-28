// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package buildinfo

import (
	"runtime/debug"
	"testing"
)

// biWith builds a *debug.BuildInfo carrying the given VCS settings, mirroring
// what runtime/debug.ReadBuildInfo() returns for a real build.
func biWith(revision string, modified bool) *debug.BuildInfo {
	mod := "false"
	if modified {
		mod = "true"
	}
	return &debug.BuildInfo{Settings: []debug.BuildSetting{
		{Key: "vcs.revision", Value: revision},
		{Key: "vcs.modified", Value: mod},
	}}
}

// TestFormat covers the three identities the binary can report (ADR-0025 §2): a
// local "dev" build with no VCS info, a local build with a VCS revision, and a
// release build whose version was injected by GoReleaser ldflags — plus the
// dirty-tree and empty-version edges.
func TestFormat(t *testing.T) {
	tests := []struct {
		name    string
		version string
		bi      *debug.BuildInfo
		want    string
	}{
		{
			name:    "dev without build info",
			version: "dev",
			bi:      nil,
			want:    "korvun dev",
		},
		{
			name:    "dev with vcs revision (truncated to 12)",
			version: "dev",
			bi:      biWith("abc1234def567890fedcba", false),
			want:    "korvun dev (abc1234def56)",
		},
		{
			name:    "dev with dirty working tree",
			version: "dev",
			bi:      biWith("abc1234def567890fedcba", true),
			want:    "korvun dev (abc1234def56+dirty)",
		},
		{
			name:    "release version from ldflags",
			version: "v0.1.0",
			bi:      nil,
			want:    "korvun v0.1.0",
		},
		{
			name:    "release version with vcs revision",
			version: "v1.2.3",
			bi:      biWith("deadbeefcafe0123", false),
			want:    "korvun v1.2.3 (deadbeefcafe)",
		},
		{
			name:    "empty version falls back to dev",
			version: "",
			bi:      nil,
			want:    "korvun dev",
		},
		{
			name:    "build info present but no vcs revision",
			version: "dev",
			bi:      &debug.BuildInfo{},
			want:    "korvun dev",
		},
		{
			name:    "short revision is not truncated",
			version: "dev",
			bi:      biWith("abc123", false),
			want:    "korvun dev (abc123)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Format(tt.version, tt.bi); got != tt.want {
				t.Errorf("Format(%q, %v) = %q, want %q", tt.version, tt.bi, got, tt.want)
			}
		})
	}
}
