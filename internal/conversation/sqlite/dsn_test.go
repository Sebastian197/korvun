// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package sqlite

import (
	"net/url"
	"testing"
)

// TestBuildFileDSN_ConstructsValidFileURI is the regression guard for the
// Windows path bug CI caught (run 27908104027, windows-latest): an absolute
// Windows path like "C:\Users\x\korvun.db" — which filepath.ToSlash turns into
// "C:/Users/x/korvun.db" — was rendered as "file://C:/..." so the SQLite driver
// read "C:" as a URI authority and rejected it ("invalid uri authority").
//
// buildFileDSN takes a FORWARD-SLASHED absolute path (the result of
// filepath.ToSlash, identical to what runs on Windows) so the Windows case is
// reproducible from a Unix/macOS test run, where filepath.ToSlash is a no-op
// and could never exercise a drive-letter path end-to-end via Open.
//
// The contract for every input: a valid file: URL with (1) an EMPTY authority —
// the drive letter must live in the path, never the host — and (2) the _pragma
// query preserved verbatim so WAL/busy_timeout/foreign_keys are actually applied.
func TestBuildFileDSN_ConstructsValidFileURI(t *testing.T) {
	tests := []struct {
		name     string
		in       string // forward-slashed absolute path (post filepath.ToSlash)
		wantPath string // decoded path the URL must carry
	}{
		{
			name:     "unix absolute path",
			in:       "/home/user/.config/korvun/korvun.db",
			wantPath: "/home/user/.config/korvun/korvun.db",
		},
		{
			name:     "windows drive-letter path",
			in:       "C:/Users/x/AppData/Roaming/korvun/korvun.db",
			wantPath: "/C:/Users/x/AppData/Roaming/korvun/korvun.db",
		},
		{
			name:     "windows drive-letter path with spaces",
			in:       "C:/Users/John Doe/korvun.db",
			wantPath: "/C:/Users/John Doe/korvun.db",
		},
		{
			name:     "unix path with a question mark",
			in:       "/tmp/weird?name.db",
			wantPath: "/tmp/weird?name.db",
		},
		{
			name:     "windows drive-letter path with a question mark",
			in:       "D:/data/weird?name.db",
			wantPath: "/D:/data/weird?name.db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn := buildFileDSN(tt.in)

			u, err := url.Parse(dsn)
			if err != nil {
				t.Fatalf("buildFileDSN(%q) = %q, not a parseable URL: %v", tt.in, dsn, err)
			}
			if u.Scheme != "file" {
				t.Errorf("scheme = %q, want \"file\" (DSN %q)", u.Scheme, dsn)
			}
			// The crux of the Windows bug: a drive letter rendered as the
			// authority. The authority MUST be empty for every platform.
			if u.Host != "" {
				t.Errorf("host = %q, want empty — a non-empty authority is the \"invalid uri authority\" bug (DSN %q)", u.Host, dsn)
			}
			if u.Path != tt.wantPath {
				t.Errorf("path = %q, want %q (DSN %q)", u.Path, tt.wantPath, dsn)
			}
			// The _pragma query must survive intact, separated from the path,
			// or WAL/busy_timeout/foreign_keys are silently dropped.
			if u.RawQuery != dsnQuery {
				t.Errorf("query = %q, want %q (DSN %q)", u.RawQuery, dsnQuery, dsn)
			}
		})
	}
}
