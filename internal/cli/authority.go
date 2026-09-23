// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

func (c *cli) authorityCmd(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprintln(c.stderr, "korvun authority: expected activate, issue, delegate, revoke or import-v1")
		return 2
	}
	switch args[0] {
	case "-h", "--help":
		_, _ = fmt.Fprintln(c.stdout, "Usage: korvun authority <activate|issue|admin-issue|delegate|admin-delegate|revoke|admin-revoke|import-v1> [flags]")
		return 0
	case "activate":
		return c.authorityActivate(args[1:])
	case "issue", "admin-issue":
		return c.authorityGrantFile(args[1:], "issue", args[0] == "admin-issue")
	case "delegate", "admin-delegate":
		return c.authorityGrantFile(args[1:], "delegate", args[0] == "admin-delegate")
	case "revoke", "admin-revoke":
		return c.authorityRevoke(args[1:], args[0] == "admin-revoke")
	case "import-v1":
		return c.authorityImportV1(args[1:])
	default:
		_, _ = fmt.Fprintf(c.stderr, "korvun authority: unknown subcommand %q\n", args[0])
		return 2
	}
}

func (c *cli) recordAuthorityAct(ctx context.Context, store *actionsqlite.Store, verb string,
	params []byte, mutate func(string) error) error {
	env, evidence, err := operatorAuthenticatedEnvelope(store, "authority", verb, string(params))
	if err != nil {
		return err
	}
	if err := store.RecordAttemptAuthenticated(ctx, env,
		actionsqlite.Decision{Outcome: "allow", Rule: operatorRule},
		action.StateAuthorized, evidence); err != nil {
		return fmt.Errorf("record authority act: %w", err)
	}
	mutationErr := mutate(env.ActionID)
	state := action.StateSucceeded
	if mutationErr != nil {
		state = action.StateFailed
	}
	// THE CLOSE IS HOUSEKEEPING, and its failure never becomes the caller's.
	//
	// `mutate` has already committed — for `intent bind --grant` it has revoked
	// a binding and written its replacement, which is irreversible. Returning
	// the close's error here made the command exit 1 and print a refusal over a
	// durable write, so the operator's belief and the ledger disagreed for good.
	// That is exactly the class `85013fd` cured for four store writers on
	// 2026-09-22 («a committed write never reports the cadence's failure»); this
	// was the fifth caller and the first whose mutation destroys a previous row.
	//
	// The failure is NOT swallowed: it goes to `c.note`, which the command
	// prints beside its success, so the act left open is visible.
	//
	// It is a METHOD and not a parameter on purpose. The first shape took a
	// `note func(error)` and its godoc claimed "nothing in the tree passes nil"
	// — an adversarial pass replaced it with nil in four of the five callers and
	// the whole package stayed green, because only one caller had a mould. A
	// structural promise with no guard is a promise about today. As a method
	// there is no nil to pass: every caller is a `*cli` and every `*cli` has a
	// `note`.
	if err := store.Finish(ctx, env.ActionID, state, time.Now().UTC()); err != nil {
		c.note(fmt.Errorf("close authority act: %w", err))
	}
	return mutationErr
}

// note reports a housekeeping failure that happened AFTER a caller's write
// committed. It is printed beside the command's own result, never in place of
// it: the write stands, and the operator is told what did not close.
func (c *cli) note(err error) {
	_, _ = fmt.Fprintf(c.stderr, "korvun: %v\n", err)
}

func (c *cli) authorityActivate(args []string) int {
	fs := flag.NewFlagSet("authority activate", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to config")
	profileID := fs.String("profile", "", "strict profile id")
	reason := fs.String("reason", "", "operator reason")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" || *profileID == "" || *reason == "" || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(c.stderr, "korvun authority activate: --config, --profile and --reason are required")
		return 2
	}
	store, err := openOperatorStoreSealed(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun authority activate: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	manifest, err := store.AuthorityActivationManifest(ctx)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun authority activate: %v\n", err)
		return 1
	}
	params := actionsqlite.CanonicalAuthorityActivation(*profileID, manifest, *reason)
	var digest string
	err = c.recordAuthorityAct(ctx, store, "activate", params, func(actionID string) error {
		var activateErr error
		digest, activateErr = store.ActivateAuthority(ctx, *profileID, actionID, *reason, time.Now().UTC())
		return activateErr
	})
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun authority activate: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(c.stdout, "authority activated for %s\nactivation_digest: %s\n", *profileID, digest)
	return 0
}

func readAuthorityGrantFile(path string) (action.AuthorityGrantV2, error) {
	// #nosec G304 -- the local path is an explicit operator CLI argument; the strict parser still owns the bytes.
	raw, err := os.ReadFile(path)
	if err != nil {
		return action.AuthorityGrantV2{}, err
	}
	return action.ParseAuthorityGrantV2(raw)
}

func (c *cli) authorityGrantFile(args []string, verb string, administrative bool) int {
	name := "authority " + verb
	if administrative {
		name = "authority admin-" + verb
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to config")
	file := fs.String("file", "", "AuthorityGrantV2 JSON file")
	reason := fs.String("reason", "", "administrative reason")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" || *file == "" || fs.NArg() != 0 || administrative && *reason == "" {
		_, _ = fmt.Fprintf(c.stderr, "korvun %s: --config and --file are required; administrative acts also require --reason\n", name)
		return 2
	}
	grant, err := readAuthorityGrantFile(*file)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun %s: %v\n", name, err)
		return 1
	}
	store, err := openOperatorStoreSealed(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun %s: %v\n", name, err)
		return 1
	}
	defer func() { _ = store.Close() }()
	params := grant.CanonicalBytes()
	if administrative {
		params = actionsqlite.CanonicalAdminAuthorityGrant(grant, *reason)
	}
	ctx := context.Background()
	err = c.recordAuthorityAct(ctx, store, verb, params, func(actionID string) error {
		now := time.Now().UTC()
		switch {
		case verb == "issue" && administrative:
			return store.AdminIssueAuthority(ctx, grant, actionID, *reason, now)
		case verb == "issue":
			return store.IssueAuthority(ctx, grant, actionID, now)
		case administrative:
			return store.AdminDelegateAuthority(ctx, grant, actionID, *reason, now)
		default:
			return store.DelegateAuthority(ctx, grant, actionID, now)
		}
	})
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun %s: %v\n", name, err)
		return 1
	}
	_, _ = fmt.Fprintf(c.stdout, "authority grant %s %sd\n", grant.GrantID, verb)
	return 0
}

func (c *cli) authorityRevoke(args []string, administrative bool) int {
	name := "authority revoke"
	if administrative {
		name = "authority admin-revoke"
	}
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to config")
	reason := fs.String("reason", "", "revocation reason")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" || *reason == "" || fs.NArg() != 1 {
		_, _ = fmt.Fprintf(c.stderr, "korvun %s: --config, --reason and one grant id are required\n", name)
		return 2
	}
	grantID := fs.Arg(0)
	store, err := openOperatorStoreSealed(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun %s: %v\n", name, err)
		return 1
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	params := actionsqlite.CanonicalAuthorityRevoke(grantID, *reason)
	err = c.recordAuthorityAct(ctx, store, "revoke", params, func(actionID string) error {
		if administrative {
			return store.AdminRevokeAuthority(ctx, grantID, actionID, *reason, time.Now().UTC())
		}
		return store.RevokeAuthority(ctx, grantID, actionID, *reason, time.Now().UTC())
	})
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun %s: %v\n", name, err)
		return 1
	}
	_, _ = fmt.Fprintf(c.stdout, "authority grant %s revoked\n", grantID)
	return 0
}

func (c *cli) authorityImportV1(args []string) int {
	fs := flag.NewFlagSet("authority import-v1", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to config")
	file := fs.String("file", "", "target AuthorityGrantV2 JSON file")
	legacyID := fs.String("legacy-grant", "", "legacy grant id")
	reason := fs.String("reason", "", "import reason")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" || *file == "" || *legacyID == "" || *reason == "" || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(c.stderr, "korvun authority import-v1: --config, --file, --legacy-grant and --reason are required")
		return 2
	}
	grant, err := readAuthorityGrantFile(*file)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun authority import-v1: %v\n", err)
		return 1
	}
	store, err := openOperatorStoreSealed(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun authority import-v1: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	params := actionsqlite.CanonicalLegacyAuthorityImport(*legacyID, grant, *reason)
	err = c.recordAuthorityAct(ctx, store, "import", params, func(actionID string) error {
		return store.ImportLegacyAuthority(ctx, *legacyID, grant, actionID, *reason, time.Now().UTC())
	})
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun authority import-v1: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(c.stdout, "legacy grant %s imported as authority v2 grant %s\n", *legacyID, grant.GrantID)
	return 0
}
