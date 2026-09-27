// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// run's isolation seen from outside: the harness BINARY, started as a
// separate OS process with its core running (-start=true, so the ledger the
// core opens is watched too), resolves its config inside its own temp dir and
// never writes to the user's config dir. Every source os.UserConfigDir reads on
// macOS, Linux and Windows points at this test's scratch dirs, so the user's
// "real" profile is a marker file only this test owns. The Go suite runs it on
// every OS it runs on — Windows included, in CI, where the chrome e2e never
// starts the harness.
//
// PROBING MUTATIONS: the isolate call removed from run → the scripted config
// lands on the marker profile → reddens; isolate moved after
// shell.DefaultConfigPath → the same.
//
// Evidence level: binary in a separate OS process, real files.
func TestHarnessBinary_neverTouchesTheRealUserConfigDir(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the harness binary")
	}
	root := t.TempDir()
	bin := filepath.Join(root, "harness")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".") //nolint:gosec // G204: the toolchain building this package into TempDir
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the harness: %v\n%s", err, out)
	}

	realDir, tmpDir, distDir := filepath.Join(root, "real"), filepath.Join(root, "tmp"), filepath.Join(root, "dist")
	for _, d := range []string{realDir, tmpDir, distDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	userEnv := map[string]string{
		"HOME":            realDir,
		"XDG_CONFIG_HOME": "",
		"AppData":         filepath.Join(realDir, "AppData", "Roaming"),
		"LocalAppData":    filepath.Join(realDir, "AppData", "Local"),
		"TMPDIR":          tmpDir,
		"TMP":             tmpDir,
		"TEMP":            tmpDir,
	}
	// Where the child's user config dir is when nothing isolates it: this
	// same environment, resolved here (after the build, which needs the real
	// HOME for its caches).
	for k, v := range userEnv {
		t.Setenv(k, v)
	}
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	marker := []byte(`{"note": "the user's real profile: the harness must not touch it"}`)
	realProfile := filepath.Join(cfgDir, "korvun", "korvun.json")
	if err := os.MkdirAll(filepath.Dir(realProfile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(realProfile, marker, 0o600); err != nil {
		t.Fatal(err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	cmd := exec.Command(bin, "-addr", addr, "-dist", distDir, "-start=true") //nolint:gosec // G204: the binary this test just built, arguments literal
	env := os.Environ()
	for k, v := range userEnv {
		env = append(env, k+"="+v) // the last value wins, case-insensitively on Windows (os/exec)
	}
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		t.Fatalf("start the harness: %v", err)
	}
	stopped := false
	stop := func() {
		if stopped {
			return
		}
		stopped = true
		if runtime.GOOS == "windows" {
			_ = cmd.Process.Kill()
		} else {
			_ = cmd.Process.Signal(os.Interrupt)
		}
		done := make(chan struct{})
		go func() { _ = cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(15 * time.Second):
			_ = cmd.Process.Kill()
			<-done
		}
	}
	t.Cleanup(stop)

	var resolved string
	for deadline := time.Now().Add(90 * time.Second); ; {
		resp, err := http.Post("http://"+addr+"/__test/bindings/DefaultConfigPath", "application/json", strings.NewReader("[]")) //nolint:gosec // G107: the loopback address this test chose
		if err == nil {
			var body struct {
				Result string `json:"result"`
				Error  string `json:"error"`
			}
			derr := json.NewDecoder(resp.Body).Decode(&body)
			_ = resp.Body.Close()
			if derr != nil || body.Error != "" {
				stop()
				t.Fatalf("DefaultConfigPath through the harness: %v %q\n%s", derr, body.Error, out.String())
			}
			resolved = body.Result
			break
		}
		if time.Now().After(deadline) {
			stop()
			t.Fatalf("the harness never answered on %s: %v\n%s", addr, err, out.String())
		}
		time.Sleep(200 * time.Millisecond)
	}
	stop()

	if !strings.HasPrefix(resolved, tmpDir+string(os.PathSeparator)) {
		t.Errorf("the harness resolves its config at %q, outside its temp dir under %q", resolved, tmpDir)
	}
	after, err := os.ReadFile(realProfile) //nolint:gosec // G304: this test's own marker file
	if err != nil || !bytes.Equal(after, marker) {
		t.Errorf("the real profile was touched: %q (%v), want %q", after, err, marker)
	}
	entries, err := os.ReadDir(filepath.Dir(realProfile))
	if err != nil {
		t.Fatalf("read the real config dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	if len(names) != 1 || names[0] != "korvun.json" {
		t.Errorf("the real config dir holds %v after the harness ran, want only korvun.json", names)
	}
}
