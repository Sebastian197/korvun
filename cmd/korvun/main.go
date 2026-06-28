// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Command korvun is the Korvun binary: it loads a JSON config, wires the
// channel -> router -> brain -> channel system, and serves until a signal stops
// it (ADR-0017). It is deliberately thin and not unit-tested — every piece that
// can fail lives in internal/config (parse + validate) and internal/app (wiring,
// boot health-check, lifecycle); main only glues them and translates failures
// into a clear message + non-zero exit (the golden rule, ADR-0017 §5).
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
	"time"

	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/buildinfo"
	"github.com/Sebastian197/korvun/internal/config"
)

// shutdownTimeout bounds graceful shutdown after a signal (ADR-0008 order:
// channel.Stop -> pump exits -> router.Shutdown).
const shutdownTimeout = 15 * time.Second

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

	application, err := app.Build(cfg, app.WithLogger(logger))
	if err != nil {
		fatal(logger, "boot", err) // bad secret / invalid token / bad wiring: fatal
	}

	// Cancel the run context on the first SIGINT/SIGTERM.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := application.Run(ctx); err != nil {
		fatal(logger, "run", err)
	}

	// Signal received: shut down cleanly within the bound.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := application.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown completed with errors", "error", err.Error())
		os.Exit(1)
	}
	logger.Info("korvun stopped cleanly")
}

// fatal logs a clear boot failure naming the stage and exits non-zero. No
// panic on any normal path (CLAUDE.md, ADR-0017 §5).
func fatal(logger *slog.Logger, stage string, err error) {
	logger.Error("fatal: korvun cannot start", "stage", stage, "error", err.Error())
	os.Exit(1)
}
