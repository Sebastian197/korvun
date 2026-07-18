// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/config"
	"github.com/Sebastian197/korvun/internal/supervisor"
)

// serveCmd is serve's own flag surface (sub-phase 2, FR-CLI-1/FR-CLI-4): a pure,
// unit-testable parse/validate step that owns argv and the pre-serve banner, then
// hands the real boot to the injectable c.boot seam. It is reached by BOTH the
// canonical `korvun serve --config x.json` (args after "serve") and the
// retrocompat shim `korvun -config x.json` (full args) — stdlib flag accepts
// `-config` and `--config` identically, so the shim stays byte-compatible.
//
// The flag surface writes usage errors to the INJECTED c.stderr (never os.Stderr
// directly), so a bad serve flag is a usage error (exit 2) a test can assert
// without a real terminal. The banner is decoration on stderr, gated exactly like
// all styling (FR-STY-8): TTY-only, off under --plain/--no-color/NO_COLOR. serve
// itself is NOT restyled — its slog JSON (bootServe) is untouched.
func (c *cli) serveCmd(args []string) int {
	fs := flag.NewFlagSet("korvun serve", flag.ContinueOnError)
	// Buffer the flag package's output so it can be routed by kind: -h/--help is a
	// query (usage -> stdout, exit 0, matching top-level help), while a bad flag is
	// a usage error (message -> stderr, exit 2, keeping stdout machine-clean —
	// R3/FR-STY-5). Deciding the stream after Parse is why the buffer is needed:
	// flag writes during Parse, before we know which case occurred.
	var usage bytes.Buffer
	fs.SetOutput(&usage)
	configPath := fs.String("config", "korvun.json", "path to the Korvun JSON config file")
	var plain, noColor bool
	fs.BoolVar(&plain, "plain", false, "disable the decorative pre-serve banner")
	fs.BoolVar(&noColor, "no-color", false, "disable ANSI color and the banner")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = io.Copy(c.stdout, &usage) // help is a query, not an error
			return 0
		}
		_, _ = io.Copy(c.stderr, &usage) // usage error: bad/unknown serve flag
		return 2
	}

	if c.styleEnabled(c.stderr, plain, noColor) {
		_, _ = fmt.Fprint(c.stderr, logo) // pre-serve banner: decoration, stderr only
	}

	return c.boot(*configPath)
}

// bootServe is the default boot seam: it hands the lifecycle to the supervisor
// (ADR-0027), which runs the wired channel -> router -> brain -> channel system
// and performs a cutover on a reload request, until SIGINT/SIGTERM. It returns a
// process exit code (0 clean stop, 1 boot/run failure) instead of calling
// os.Exit, so the CLI dispatch owns the exit.
//
// This is the pre-CLI main boot body: flag parsing/validation lived here in
// sub-phase 1 and moved to serveCmd in sub-phase 2, leaving bootServe as pure
// boot glue (still un-unit-tested beyond the config.Load fatal, covered by
// internal/app's lifecycle e2e — ADR-0017). Logging stays the structured slog
// JSON on stderr (ADR-0017 §5) — serve is deliberately not restyled.
func bootServe(configPath string) int {
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))

	cfg, err := config.Load(configPath)
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
		return supervisor.WriteConfigAtomic(configPath, c)
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
