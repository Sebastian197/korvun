// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"net/url"
	"path/filepath"
	"testing"
)

// The hook finds its file through the DSN (dsnPath), and every seam of the
// judge is selected by that file's absolute path: a DSN that parses back to
// another spelling than the one the opener built it from arms no seam at all.
// On Windows it did: the builders write C:\… as file:///C:/…, the path came
// back as \C:\…, and in the first CI run of PR #69 no hook seam ever armed
// there (748 subtests of TE19–TE26, TE28–TE30 and TE02).
//
// PROBING MUTATIONS: the drive's leading slash kept → the Windows drive rows
// redden; the slash dropped on every GOOS → the Linux row reddens; any letter
// taken for a drive, a drive-relative "C:x" taken for a drive path, or a
// first byte dropped that is not a slash → the rows that pin each of them
// redden (the adversary's pass over the cures of PR #69).
//
// Evidence level: unit, in process. Every platform's row runs on any host,
// through the real DSN builders and url.Parse; the GOOS is an argument. The
// rows past the builders call dsnPathFor directly, with URL paths the
// builders never make from an absolute path.
func TestDsnPathFor_givesBackTheFileTheDSNWasBuiltFrom(t *testing.T) {
	for _, c := range []struct {
		name, goos, slashed, want string
	}{
		{"windows drive", "windows", "C:/Users/x/korvun.db", `C:\Users\x\korvun.db`},
		{"windows lower-case drive", "windows", "c:/Users/x/korvun.db", `c:\Users\x\korvun.db`},
		{"windows drive root", "windows", "C:/", `C:\`},
		{"windows UNC share", "windows", "//server/share/korvun.db", `\\server\share\korvun.db`},
		{"windows path without a drive", "windows", "/data/korvun.db", `\data\korvun.db`},
		{"linux directory named C:", "linux", "/C:/x/korvun.db", "/C:/x/korvun.db"},
		{"linux", "linux", "/tmp/x/korvun.db", "/tmp/x/korvun.db"},
		{"darwin", "darwin", "/var/folders/x/korvun.db", "/var/folders/x/korvun.db"},
	} {
		for _, dsn := range []string{
			buildFileDSN(c.slashed),
			guardedDSN(buildWriterDSN(c.slashed), "n0nce"),
		} {
			u, err := url.Parse(dsn)
			if err != nil {
				t.Fatalf("%s: parse %q: %v", c.name, dsn, err)
			}
			if got := dsnPathFor(c.goos, u.Path); got != c.want {
				t.Errorf("%s: dsnPathFor(%q, %q), from the DSN %q, = %q, want %q", c.name, c.goos, u.Path, dsn, got, c.want)
			}
		}
	}
	for _, c := range []struct {
		name, goos, urlPath, want string
	}{
		{"windows, the empty path", "windows", "", ""},
		{"linux, the empty path", "linux", "", ""},
		{"darwin, the empty path", "darwin", "", ""},
		{"windows, only a leading slash is dropped", "windows", "xC:/y", `xC:\y`},
		{"windows, not a drive letter: the slash stays", "windows", "/1:/x/korvun.db", `\1:\x\korvun.db`},
		{"windows, a drive-relative spelling: the slash stays", "windows", "/C:x/korvun.db", `\C:x\korvun.db`},
	} {
		if got := dsnPathFor(c.goos, c.urlPath); got != c.want {
			t.Errorf("%s: dsnPathFor(%q, %q) = %q, want %q", c.name, c.goos, c.urlPath, got, c.want)
		}
	}
}

// On THIS host, dsnPath gives back the absolute path a writer's guarded DSN
// was built from — the glue TestDsnPathFor_givesBackTheFileTheDSNWasBuiltFrom
// does not reach (url.Parse, then
// dsnPathFor with runtime.GOOS). On macOS and Linux it held before the cure
// too; its Windows run is in CI.
//
// PROBING MUTATION: dsnPath reads the URL's opaque part instead of its path →
// reddens on every host.
//
// Evidence level: unit, in process, host OS only.
func TestDsnPath_onThisHostIsThePathTheDSNWasBuiltFrom(t *testing.T) {
	abs := filepath.Join(t.TempDir(), "korvun.db")
	dsn := guardedDSN(buildWriterDSN(filepath.ToSlash(abs)), "n0nce")
	if got := dsnPath(dsn); got != abs {
		t.Fatalf("dsnPath(%q) = %q, want %q, the path the DSN was built from", dsn, got, abs)
	}
}
