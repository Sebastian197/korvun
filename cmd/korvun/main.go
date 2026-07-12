// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// Command korvun is the Korvun binary. main is deliberately thin (ADR-0017): it
// forwards argv and the standard streams into internal/cli, which parses the
// subcommands, runs the retrocompat shim, and hands `serve` to the supervisor
// (ADR-0027). All logic — and all tests — live in internal/cli.
package main

import (
	"os"

	"github.com/Sebastian197/korvun/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
