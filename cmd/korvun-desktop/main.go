// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

//go:build desktop

// Command korvun-desktop is the desktop shell for Korvun (ADR-0035): a native
// window that will serve the existing builder and operate an in-process core.
//
// SP4 scope: the SP1 skeleton (static page) + the asset seam — the
// internal/shell Controller's same-origin admin proxy mounted as the
// AssetServer Handler, so /api/*, /builder/*, /ui/*, /healthz and /metrics
// resolve against the current core cycle (503 while stopped). The real
// chrome and its start/stop bindings are SP6.
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
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"github.com/Sebastian197/korvun/internal/shell"
	"github.com/Sebastian197/korvun/internal/shell/keyring"
)

//go:embed all:frontend
var shellAssets embed.FS

func main() {
	ctrl := shell.New(
		shell.WithLogger(slog.Default()),
		shell.WithSecretStore(keyring.New()),
	)
	err := wails.Run(&options.App{
		Title:  "Korvun",
		Width:  1100,
		Height: 760,
		// Assets are consulted FIRST; the Handler serves only misses. A
		// frontend asset named under /api, /builder, /ui, /healthz or
		// /metrics would silently shadow the proxy — never add one.
		AssetServer: &assetserver.Options{
			Assets:  shellAssets,
			Handler: ctrl.ProxyHandler(),
		},
	})
	if err != nil {
		slog.Error("korvun-desktop: window loop failed", "error", err.Error())
		os.Exit(1)
	}
}
