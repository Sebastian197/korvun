// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

//go:build desktop

// Command korvun-desktop is the desktop shell for Korvun (ADR-0035): a native
// window serving the chrome (SP6) over the in-process core.
//
// SP6a scope: the built chrome (frontend/dist, //go:embed — the ADR-0029 §4
// stub pattern keeps a clean clone compiling; `make desktop-frontend` builds
// the real bundle) + the SP4 same-origin admin proxy as the AssetServer
// Handler + the internal/shell Desktop bindings (THE LAW: every call
// bounded). The window chrome itself grows cut by cut (6b/6c).
//
// Build (never part of the default suite; the desktop build tag gates this
// package so the headless ×6 CGO_ENABLED=0 pipeline and the 3-OS quality
// gate stay untouched):
//
//	go build -tags desktop,production ./cmd/korvun-desktop
//
// On macOS 13 (and older Xcode CLT) the link needs the UniformTypeIdentifiers
// framework made explicit (found empirically in SP1 — UTType is otherwise an
// undefined symbol):
//
//	CGO_LDFLAGS="-framework UniformTypeIdentifiers" go build -tags desktop,production ./cmd/korvun-desktop
package main

import (
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/Sebastian197/korvun/internal/shell"
	"github.com/Sebastian197/korvun/internal/shell/keyring"
	"github.com/Sebastian197/korvun/internal/shell/logsink"
)

//go:embed all:frontend/dist
var chromeAssets embed.FS

// version is stamped by the release build via
// -ldflags "-X main.version=v…" (ADR-0025 pattern); "dev" otherwise.
var version = "dev"

func main() {
	// Non-GUI identity path (SP7 §1g): a packaged bundle answers `--version`
	// without opening a window, so CI and the packaging recipe can prove the
	// build was version-stamped (goodbye "dev") on a headless runner.
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v" || os.Args[1] == "version") {
		fmt.Println("korvun-desktop", version)
		return
	}

	// File log sink (v0.9.1, app-audit B3): Finder-launched apps have their
	// stderr discarded by macOS, so slog tees to korvun-desktop.log in the
	// profile directory (logsink docs; one-deep rotation). A sink failure
	// keeps stderr-only logging — logging must never block the launch.
	if cfgPath, err := shell.DefaultConfigPath(); err == nil {
		dir := filepath.Dir(cfgPath)
		if err := os.MkdirAll(dir, 0o700); err == nil {
			if f, err := logsink.Open(dir); err == nil {
				slog.SetDefault(slog.New(slog.NewTextHandler(
					io.MultiWriter(os.Stderr, f), nil)))
				slog.Info("korvun-desktop: log sink open",
					"path", filepath.Join(dir, logsink.FileName), "version", version)
			} else {
				slog.Warn("korvun-desktop: log sink unavailable, stderr only", "error", err.Error())
			}
		}
	}

	assets, err := fs.Sub(chromeAssets, "frontend/dist")
	if err != nil {
		slog.Error("korvun-desktop: chrome assets missing", "error", err.Error())
		os.Exit(1)
	}

	ctrl := shell.New(
		shell.WithLogger(slog.Default()),
		shell.WithSecretStore(keyring.New()),
	)
	desk := shell.NewDesktop(ctrl,
		shell.WithDesktopLogger(slog.Default()),
		shell.WithDesktopVersion(version),
	)

	err = wails.Run(&options.App{
		Title:  "Korvun",
		Width:  1100,
		Height: 760,
		// Assets are consulted FIRST; the Handler serves only misses. A
		// frontend asset named under /api, /builder, /ui, /healthz or
		// /metrics would silently shadow the proxy — never add one.
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: ctrl.ProxyHandler(),
		},
		Bind: []interface{}{desk},
	})
	if err != nil {
		slog.Error("korvun-desktop: window loop failed", "error", err.Error())
		os.Exit(1)
	}
}
