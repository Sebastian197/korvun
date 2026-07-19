// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

//go:build desktop

// Command korvun-desktop is the desktop shell for Korvun (ADR-0035): a native
// window that will serve the existing builder and operate an in-process core.
//
// SP1 scope: the skeleton only — a window showing the shell's own static
// page. No core, no lifecycle logic yet (that is internal/shell, SP2+).
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
)

//go:embed all:frontend
var shellAssets embed.FS

func main() {
	err := wails.Run(&options.App{
		Title:  "Korvun",
		Width:  1100,
		Height: 760,
		AssetServer: &assetserver.Options{
			Assets: shellAssets,
		},
	})
	if err != nil {
		slog.Error("korvun-desktop: window loop failed", "error", err.Error())
		os.Exit(1)
	}
}
