// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// serveMain is the default serve seam: it parses the serve flags (-config) from
// args and hands the lifecycle to the supervisor (ADR-0027), which runs the
// wired channel -> router -> brain -> channel system and performs a cutover on a
// reload request, until SIGINT/SIGTERM. It returns a process exit code (0 clean
// stop, 1 boot/run failure, 2 usage error) instead of calling os.Exit, so the
// CLI dispatch owns the exit.
//
// This is the pre-CLI main body, moved verbatim behind the seam: the CLI routes
// both the canonical `korvun serve --config x.json` and the retrocompat shim
// `korvun -config x.json` here, so today's boot is byte-for-byte unchanged. A
// dedicated serve flag surface (and its styling) is sub-phase 2; sub-phase 1 only
// preserves the existing behaviour. Logging stays the structured slog JSON on
// stderr (ADR-0017 §5) — serve is deliberately not restyled.
func serveMain(args []string) int {
	fs := flag.NewFlagSet("korvun serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	configPath := fs.String("config", "korvun.json", "path to the Korvun JSON config file")
	if err := fs.Parse(args); err != nil {
		return 2 // usage error: bad/unknown serve flag
	}

	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	cfg, err := config.Load(*configPath)
	if err != nil {
		return serveFatal(logger, "config", err) // malformed/missing config: fatal, named
	}

	// sup is late-bound: the build seam closes over it so app.Build can mount the
	// config-mutation endpoint pointing back at the supervisor (ADR-0027 §seam).
	var sup *supervisor.Supervisor

	// The build seam the supervisor uses for the initial boot and every reload.
	build := func(c *config.Config) (supervisor.App, error) {
		return app.Build(c, app.WithLogger(logger), app.WithReloader(sup))
	}

	// The effect-free pre-cutover validation seam (ADR-0027 §5).
	preflight := func(c *config.Config) error {
		return app.Preflight(c, app.WithLogger(logger))
	}

	// The supervisor listens for shutdown on its OWN channel (F6/N2), not through
	// any App's context, so it can tell a cutover-cancel from a real signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// After a successful cutover the supervisor persists the new config atomically
	// back to the -config file (ADR-0027 §F5).
	persist := func(c *config.Config) error {
		return supervisor.WriteConfigAtomic(*configPath, c)
	}

	sup = supervisor.New(cfg,
		supervisor.WithBuild(build),
		supervisor.WithPreflight(preflight),
		supervisor.WithPersist(persist),
		supervisor.WithLogger(logger),
		supervisor.WithSignalChan(sigCh),
	)
	if err := sup.Run(context.Background()); err != nil {
		return serveFatal(logger, "run", err) // bad secret / invalid token / cutover failure
	}
	logger.Info("korvun stopped cleanly")
	return 0
}

// serveFatal logs a clear boot failure naming the stage and returns exit code 1.
// No panic on any normal path (CLAUDE.md, ADR-0017 §5).
func serveFatal(logger *slog.Logger, stage string, err error) int {
	logger.Error("fatal: korvun cannot start", "stage", stage, "error", err.Error())
	return 1
}
