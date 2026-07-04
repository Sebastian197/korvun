// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Command korvun is the Korvun binary: it loads a JSON config, then hands the
// lifecycle to the supervisor (ADR-0027), which runs the wired channel -> router ->
// brain -> channel system and, on a reload request, performs the cutover. main is
// deliberately thin: config parsing lives in internal/config, wiring/boot in
// internal/app, and the lifecycle/cutover in internal/supervisor; main only loads
// the config, wires the real seams, and translates a fatal into a clear message +
// non-zero exit (ADR-0017 §5).
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/buildinfo"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// version is the build version, overridden at release time by GoReleaser via
// -ldflags "-X main.version=vX.Y.Z" (ADR-0025 §2). A local `go build` keeps "dev".
var version = "dev"

func main() {
	configPath := flag.String("config", "korvun.json", "path to the Korvun JSON config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		bi, _ := debug.ReadBuildInfo()
		fmt.Println(buildinfo.Format(version, bi))
		os.Exit(0)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal(logger, "config", err) // malformed/missing config: fatal, named
	}

	// The build seam the supervisor uses for the initial boot and every reload:
	// wrap app.Build (which opens the store and wires channels/brains). *app.App
	// satisfies supervisor.App.
	build := func(c *config.Config) (supervisor.App, error) {
		return app.Build(c, app.WithLogger(logger))
	}

	// The supervisor listens for shutdown on its OWN channel (F6/N2), not through
	// any App's context, so it can tell a cutover-cancel from a real signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// After a successful cutover the supervisor persists the new config atomically
	// back to the -config file (ADR-0027 §F5), so a plain restart reloads exactly what
	// the builder produced.
	persist := func(c *config.Config) error {
		return supervisor.WriteConfigAtomic(*configPath, c)
	}

	sup := supervisor.New(cfg,
		supervisor.WithBuild(build),
		supervisor.WithPersist(persist),
		supervisor.WithSignalChan(sigCh),
	)
	if err := sup.Run(context.Background()); err != nil {
		fatal(logger, "run", err) // bad secret / invalid token / cutover failure
	}
	logger.Info("korvun stopped cleanly")
}

// fatal logs a clear boot failure naming the stage and exits non-zero. No
// panic on any normal path (CLAUDE.md, ADR-0017 §5).
func fatal(logger *slog.Logger, stage string, err error) {
	logger.Error("fatal: korvun cannot start", "stage", stage, "error", err.Error())
	os.Exit(1)
}
