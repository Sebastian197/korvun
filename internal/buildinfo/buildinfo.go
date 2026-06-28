// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Package buildinfo formats the binary's version identity for the --version flag
// (ADR-0025 §2). It is a leaf with a single pure function so the formatting logic
// is unit-tested while cmd/korvun/main stays the deliberately un-unit-tested glue
// (ADR-0017): main injects the ldflags-set version and the impure
// runtime/debug.ReadBuildInfo() result; this package turns them into a string.
package buildinfo

import "runtime/debug"

// shortRevLen bounds the VCS revision shown in the version line: enough to be
// unambiguous in practice, short enough to read.
const shortRevLen = 12

// Format renders the binary's version identity. version is the build version:
// "dev" for a local go build, or the SemVer tag GoReleaser injects via
// -ldflags "-X main.version=vX.Y.Z" on a release build; an empty version falls
// back to "dev". bi is the result of runtime/debug.ReadBuildInfo() (nil when
// unavailable); when it carries a VCS revision, a short revision is appended (with
// a "+dirty" marker if the working tree was modified at build time), so even a
// local "dev" build reports a useful identity.
func Format(version string, bi *debug.BuildInfo) string {
	if version == "" {
		version = "dev"
	}
	out := "korvun " + version
	if bi != nil {
		if rev, modified := vcs(bi); rev != "" {
			if len(rev) > shortRevLen {
				rev = rev[:shortRevLen]
			}
			if modified {
				rev += "+dirty"
			}
			out += " (" + rev + ")"
		}
	}
	return out
}

// vcs extracts the VCS revision and dirty flag from a BuildInfo's settings, the
// keys the Go toolchain stamps into a VCS-tracked build (vcs.revision,
// vcs.modified). Empty revision means the info was not stamped (e.g. a build
// outside a repo, or with -buildvcs=false).
func vcs(bi *debug.BuildInfo) (revision string, modified bool) {
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	return revision, modified
}
