// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator's act, intents half (Trust Layer Etapa 2, lote 5, sealed
// decision 3): `korvun intent create|activate|revoke|list|show` against
// the local v2 kernel store. Store access is BRIEF (open, act, close) on
// the shared WAL file — busy_timeout is the cross-writer safety net, and
// no CLI invocation ever holds a long write. Every mutation leaves its
// RECEIPT: an identified action row with the operator as principal and
// loopback_inprocess evidence — the human's act leaves a trace too.
package cli

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/config"
)

// operatorRule labels the operator's own CLI acts in the audit grammar.
// A NEW finite label (flagged for adjudication like authority_inactive):
// the operator wields the root's standing authority directly, and calling
// that "granted" or "ungoverned" would lie to the trail.
const operatorRule = "operator"

// intentCmd dispatches the `intent` noun's verbs.
func (c *cli) intentCmd(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(c.stderr, "korvun intent: expected a subcommand: create | activate | revoke | list | show\nRun 'korvun help' for usage.\n")
		return 2
	}
	switch args[0] {
	case "create":
		return c.intentCreate(args[1:])
	case "activate":
		return c.intentTransition(args[1:], "activate", action.LifecycleActive)
	case "revoke":
		return c.intentTransition(args[1:], "revoke", action.LifecycleRevoked)
	case "list":
		return c.intentList(args[1:])
	case "show":
		return c.intentShow(args[1:])
	default:
		_, _ = fmt.Fprintf(c.stderr, "korvun intent: unknown subcommand %q\nRun 'korvun help' for usage.\n", args[0])
		return 2
	}
}

// openOperatorStore loads the config strictly and opens the kernel store
// on the SAME resolved file the server uses. The caller closes it — the
// brief-access discipline of the shared single-writer file.
func openOperatorStore(configPath string) (*actionsqlite.Store, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	return actionsqlite.Open(app.StoragePath(cfg))
}

// openOperatorStoreSealed opens the store WITH the profile's ink wired
// (Etapa 4 FR-VER): the operator's mutating acts leave SIGNED receipts,
// sealed with the same key the server boot uses (generated idempotently
// if the profile is fresh — the boot's own semantics). Read-only verbs
// keep the plain opener: verification must never write, not even a key.
func openOperatorStoreSealed(configPath string) (*actionsqlite.Store, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	storage := app.StoragePath(cfg)
	store, err := actionsqlite.Open(storage)
	if err != nil {
		return nil, err
	}
	priv, err := app.EnsureSigningKey(context.Background(), store, filepath.Dir(storage))
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	store.SetReceiptSealer(func(r action.Receipt) action.Receipt {
		return action.SignReceipt(priv, r)
	})
	return store, nil
}

// recordOperatorAct wraps one CLI mutation in its receipt: the identified
// AUTHORIZED record (operator principal, loopback evidence) lands BEFORE
// the mutation, and the terminal state tells the truth about how it went.
// A pre-validated denial records DENIED instead and never runs.
func recordOperatorAct(ctx context.Context, store *actionsqlite.Store, namespace, name, params string, mutate func() error) error {
	env, identity, err := operatorEnvelope(namespace, name, params)
	if err != nil {
		return err
	}
	if err := store.RecordAttemptIdentified(ctx, env,
		actionsqlite.Decision{Outcome: "allow", Rule: operatorRule},
		action.StateAuthorized, identity); err != nil {
		return fmt.Errorf("record the act: %w", err)
	}
	mutErr := mutate()
	state := action.StateSucceeded
	if mutErr != nil {
		state = action.StateFailed
	}
	if err := store.Finish(ctx, env.ActionID, state, time.Now().UTC()); err != nil && mutErr == nil {
		return fmt.Errorf("close the receipt: %w", err)
	}
	return mutErr
}

// operatorEnvelope builds the identified envelope of one operator act:
// loopback provenance, the operator as principal, root standing intent.
func operatorEnvelope(namespace, name, params string) (action.Envelope, actionsqlite.AttemptIdentity, error) {
	registry := action.ProvenanceRegistry{
		"cli": {Class: "console", Credential: action.CredentialLoopbackInProcess},
	}
	principal, evidence, err := action.ResolvePrincipal(registry, "cli", "operator", time.Now().UTC())
	if err != nil {
		// Unreachable: the registry above always carries "cli".
		return action.Envelope{}, actionsqlite.AttemptIdentity{}, err
	}
	env := action.NewEnvelope(action.NewID(), "cli",
		action.Source{Kind: "operator", Protocol: "cli", Channel: "cli"},
		action.Operation{Namespace: namespace, Name: name, Version: 1},
		params, time.Now().UTC())
	env.Principal = action.PrincipalRef{
		PrincipalID:        principal.PrincipalID,
		EvidenceID:         evidence.EvidenceID,
		ResponsibleHumanID: principal.ResponsibleHumanID,
	}
	env.IntentID = action.RootIntentID
	return env, actionsqlite.AttemptIdentity{
		PrincipalID: principal.PrincipalID,
		IntentID:    action.RootIntentID,
		Evidence:    evidence,
	}, nil
}

// splitCSV parses a comma-separated flag into a trimmed, sorted set.
func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	sort.Strings(out)
	return out
}

// parseTimeFlag parses an RFC3339 time flag ("" = zero time).
func parseTimeFlag(raw, flagName string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("--%s: %w", flagName, err)
	}
	return t.UTC(), nil
}

// intentCreate implements `korvun intent create`.
func (c *cli) intentCreate(args []string) int {
	fs := flag.NewFlagSet("intent create", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to the korvun config (required)")
	purpose := fs.String("purpose", "", "the authorized outcome in words (required)")
	operations := fs.String("operations", "", "comma-separated operation set (required)")
	resources := fs.String("resources", "*", "comma-separated coarse resource set")
	maxActions := fs.Int("max-actions", 0, "total action budget (0 = unlimited)")
	validFrom := fs.String("valid-from", "", "window start, RFC3339 (default: now)")
	expires := fs.String("expires", "", "window end, RFC3339 (default: no expiry)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" || *purpose == "" || *operations == "" {
		_, _ = fmt.Fprint(c.stderr, "korvun intent create: --config, --purpose and --operations are required\n")
		return 2
	}
	from, err := parseTimeFlag(*validFrom, "valid-from")
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent create: %v\n", err)
		return 2
	}
	if from.IsZero() {
		from = time.Now().UTC()
	}
	until, err := parseTimeFlag(*expires, "expires")
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent create: %v\n", err)
		return 2
	}
	intent := action.IntentContract{
		IntentID:          action.NewIntentID(),
		SchemaVersion:     1,
		OwnerPrincipalID:  action.OperatorPrincipal().PrincipalID,
		Purpose:           *purpose,
		AllowedOperations: splitCSV(*operations),
		AllowedResources:  splitCSV(*resources),
		Budgets:           action.Budgets{MaxActions: *maxActions},
		ValidFrom:         from,
		ExpiresAt:         until,
		Status:            action.LifecycleDraft,
		Version:           1,
	}
	store, err := openOperatorStoreSealed(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent create: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	params := contractParams(map[string]any{
		"intent_id": intent.IntentID, "purpose": intent.Purpose,
		"operations": intent.AllowedOperations, "resources": intent.AllowedResources,
		"max_actions": intent.Budgets.MaxActions,
		"valid_from":  from.Format(time.RFC3339Nano), "expires_at": *expires,
	})
	if err := recordOperatorAct(ctx, store, "intent", "create", params, func() error {
		return store.CreateIntent(ctx, intent)
	}); err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent create: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(c.stdout, "intent %s created (DRAFT)\n", intent.IntentID)
	return 0
}

// contractParams renders one act's terms for its receipt digest.
func contractParams(terms map[string]any) string {
	raw, err := json.Marshal(terms)
	if err != nil {
		// Unreachable for the plain types above; kept for honesty.
		return "unmarshalable"
	}
	return string(raw)
}

// intentTransition implements activate/revoke: one positional id.
func (c *cli) intentTransition(args []string, verb string, to action.LifecycleStatus) int {
	fs := flag.NewFlagSet("intent "+verb, flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to the korvun config (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" || fs.NArg() != 1 {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent %s: usage: korvun intent %s --config <path> <intent-id>\n", verb, verb)
		return 2
	}
	id := fs.Arg(0)
	store, err := openOperatorStoreSealed(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent %s: %v\n", verb, err)
		return 1
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	params := contractParams(map[string]any{"intent_id": id, "to": string(to)})
	if err := recordOperatorAct(ctx, store, "intent", verb, params, func() error {
		return store.TransitionIntent(ctx, id, to)
	}); err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent %s: %v\n", verb, err)
		return 1
	}
	_, _ = fmt.Fprintf(c.stdout, "intent %s %s\n", id, to)
	return 0
}

// intentList implements `korvun intent list`.
func (c *cli) intentList(args []string) int {
	fs := flag.NewFlagSet("intent list", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to the korvun config (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" {
		_, _ = fmt.Fprint(c.stderr, "korvun intent list: --config is required\n")
		return 2
	}
	store, err := openOperatorStore(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent list: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()
	intents, err := store.ListIntents(context.Background())
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent list: %v\n", err)
		return 1
	}
	if len(intents) == 0 {
		_, _ = fmt.Fprintln(c.stdout, "no intents stored")
		return 0
	}
	_, _ = fmt.Fprintf(c.stdout, "%-38s %-8s %s\n", "ID", "STATUS", "PURPOSE")
	for _, intent := range intents {
		_, _ = fmt.Fprintf(c.stdout, "%-38s %-8s %s\n", intent.IntentID, intent.Status, intent.Purpose)
	}
	return 0
}

// intentShow implements `korvun intent show`.
func (c *cli) intentShow(args []string) int {
	fs := flag.NewFlagSet("intent show", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to the korvun config (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" || fs.NArg() != 1 {
		_, _ = fmt.Fprint(c.stderr, "korvun intent show: usage: korvun intent show --config <path> <intent-id>\n")
		return 2
	}
	store, err := openOperatorStore(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent show: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()
	intent, err := store.GetIntent(context.Background(), fs.Arg(0))
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent show: %v\n", err)
		return 1
	}
	expiry := "never"
	if !intent.ExpiresAt.IsZero() {
		expiry = intent.ExpiresAt.Format(time.RFC3339)
	}
	budget := "unlimited"
	if intent.Budgets.MaxActions != 0 {
		budget = fmt.Sprintf("%d actions", intent.Budgets.MaxActions)
	}
	_, _ = fmt.Fprintf(c.stdout,
		"id:         %s\nstatus:     %s\npurpose:    %s\noperations: %s\nresources:  %s\nbudget:     %s\nvalid from: %s\nexpires:    %s\nowner:      %s\ndigest:     %s\n",
		intent.IntentID, intent.Status, intent.Purpose,
		strings.Join(intent.AllowedOperations, ", "),
		strings.Join(intent.AllowedResources, ", "),
		budget, intent.ValidFrom.Format(time.RFC3339), expiry,
		intent.OwnerPrincipalID, intent.Digest())
	return 0
}
