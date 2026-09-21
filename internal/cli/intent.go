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
	"crypto/ed25519"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
	"github.com/Sebastian197/korvun/internal/app"
	"github.com/Sebastian197/korvun/internal/config"
	identityv2 "github.com/Sebastian197/korvun/internal/identity"
)

// operatorRule labels the operator's own CLI acts in the audit grammar.
// A NEW finite label (flagged for adjudication like authority_inactive):
// the operator wields the root's standing authority directly, and calling
// that "granted" or "ungoverned" would lie to the trail.
const operatorRule = "operator"

// intentCmd dispatches the `intent` noun's verbs.
func (c *cli) intentCmd(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(c.stderr, "korvun intent: expected a subcommand: create | activate | revoke | list | show | create-v2 | activate-v2 | expire-v2 | revoke-v2 | verify-v2 | bind | adopt-root | import-legacy\nRun 'korvun help' for usage.\n")
		return 2
	}
	switch args[0] {
	case "-h", "--help":
		// A query, not a usage error (ADR-0032 «Exit codes»): the noun's usage
		// to stdout, exit 0 (v0.15.1 block B, P2-2).
		_, _ = fmt.Fprint(c.stdout, "Usage: korvun intent <create|activate|revoke|list|show> [flags]\n\nRun 'korvun intent <verb> -h' for the flags of one verb.\n")
		return 0
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
	case "create-v2":
		return c.intentCreateV2(args[1:])
	case "activate-v2":
		return c.intentTransitionV2(args[1:], "activate-v2", action.LifecycleActive)
	case "expire-v2":
		return c.intentTransitionV2(args[1:], "expire-v2", action.LifecycleExpired)
	case "revoke-v2":
		return c.intentTransitionV2(args[1:], "revoke-v2", action.LifecycleRevoked)
	case "verify-v2":
		return c.intentVerifyV2(args[1:])
	case "bind":
		return c.intentBind(args[1:])
	case "adopt-root":
		return c.intentImportLegacy(args[1:], true)
	case "import-legacy":
		return c.intentImportLegacy(args[1:], false)
	default:
		_, _ = fmt.Fprintf(c.stderr, "korvun intent: unknown subcommand %q\nRun 'korvun help' for usage.\n", args[0])
		return 2
	}
}

// openOperatorStore loads the config strictly and opens the kernel
// store READ-ONLY on the SAME resolved file the server uses (the R1
// door, cross-check law point 3): consultation and verification run no
// migration, no crash recovery, no prune, and their connection refuses
// every write at the SQLite level — a verify beside a LIVE server can
// never touch an in-flight action. Mutating operator commands use
// openOperatorStoreSealed, which keeps the full lifecycle open path.
func openOperatorStore(configPath string) (*actionsqlite.Store, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	return actionsqlite.OpenReadOnly(app.StoragePath(cfg))
}

// openOperatorStoreSealed opens the store WITH the profile's ink wired
// (Etapa 4 FR-VER): the operator's mutating acts leave SIGNED receipts,
// sealed with the same key the server boot uses (generated idempotently
// if the profile is fresh — the boot's own semantics). Read-only verbs
// keep the plain opener: verification issues no write and reads no key
// material. It is not a claim that nothing reaches the disk — see
// `OpenReadOnly`'s godoc for what the open itself writes (R14).
// C4: the acts go through the THIRD door — writing, but no recovery,
// no prune, and never a migration of an existing store; a CLI beside a
// live server must not close the server's in-flight work.
func openOperatorStoreSealed(configPath string) (*actionsqlite.Store, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, err
	}
	storage := app.StoragePath(cfg)
	store, err := actionsqlite.OpenOperator(storage)
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
	store.SetIntentV2Signer(func(contract action.IntentContractV2) action.SignedIntentContractV2 {
		return action.SignIntentContractV2(priv, contract)
	}, func(event action.IntentEventV1) action.SignedIntentEventV1 {
		return action.SignIntentEventV1(priv, event)
	})
	if err := wireOperatorIdentity(store, priv, time.Now().UTC()); err != nil {
		_ = store.Close()
		return nil, err
	}
	return store, nil
}

const (
	localCLIRequester   = "principal_local_profile"
	localCLIResponsible = "principal_local_operator_role"
)

func localCLIRegistry() identityv2.Registry {
	return identityv2.Registry{
		Principals: []identityv2.Principal{
			{ID: localCLIRequester, Kind: identityv2.PrincipalExternalSystem,
				DisplayName: "Local profile credential"},
			{ID: action.OperatorPrincipal().PrincipalID, Kind: identityv2.PrincipalWorkload,
				DisplayName: "Local CLI operator workload"},
			{ID: localCLIResponsible, Kind: identityv2.PrincipalExternalSystem,
				DisplayName: "Local operator role"},
		},
		Bindings: []identityv2.Binding{{
			ID: "binding_cli", Provider: "cli", Channel: "cli",
			CredentialRef: "local_profile", SubjectNamespace: "local_profile",
			VerifiedSubject: "shared_local_profile",
			PrincipalID:     localCLIRequester, Generation: 1,
			Status: identityv2.BindingActive,
		}},
		Workloads: []identityv2.Workload{{
			Brain: "cli", PrincipalID: action.OperatorPrincipal().PrincipalID,
			ResponsiblePrincipalID: localCLIResponsible,
		}},
	}
}

func wireOperatorIdentity(store *actionsqlite.Store, privateKey ed25519.PrivateKey, at time.Time) error {
	store.SetIdentitySigners(
		func(e identityv2.Evidence) identityv2.SignedEvidence {
			return identityv2.SignEvidence(privateKey, e)
		},
		func(e identityv2.PrincipalEvent) identityv2.SignedPrincipalEvent {
			return identityv2.SignPrincipalEvent(privateKey, e)
		},
	)
	if err := store.RegisterIdentity(context.Background(), localCLIRegistry(), at); err != nil {
		return fmt.Errorf("register local CLI identity: %w", err)
	}
	return nil
}

func parseIntentVersion(raw string) (int, error) {
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("version must be a positive integer")
	}
	return v, nil
}

func (c *cli) intentCreateV2(args []string) int {
	fs := flag.NewFlagSet("intent create-v2", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to the korvun config")
	file := fs.String("file", "", "strict IntentContractV2 JSON file")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" || *file == "" || fs.NArg() != 0 {
		_, _ = fmt.Fprintln(c.stderr, "korvun intent create-v2: --config and --file are required")
		return 2
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent create-v2: %v\n", err)
		return 1
	}
	contract, err := action.ParseIntentContractV2(raw)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent create-v2: %v\n", err)
		return 1
	}
	store, err := openOperatorStoreSealed(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent create-v2: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()
	// The operator's act leaves ITS OWN record too, exactly as the v1 verbs do
	// and as this file's godoc promises. The eight v2 verbs used to skip it, so
	// the phase about identity and intent recorded no principal for the human
	// who acted (the twenty-second pass, P2-10).
	if err = recordOperatorAct(context.Background(), store, "intent", "create-v2", contract.IntentID, func() error {
		return store.CreateIntentV2(context.Background(), contract, action.OperatorPrincipal().PrincipalID, time.Now().UTC())
	}); err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent create-v2: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(c.stdout, "intent %s version %d created (DRAFT, signed event)\n", contract.IntentID, contract.Version)
	return 0
}

func (c *cli) intentTransitionV2(args []string, verb string, to action.LifecycleStatus) int {
	fs := flag.NewFlagSet("intent "+verb, flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to the korvun config")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" || fs.NArg() < 1 || fs.NArg() > 2 {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent %s: --config, intent id, and version for activation are required\n", verb)
		return 2
	}
	store, err := openOperatorStoreSealed(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent %s: %v\n", verb, err)
		return 1
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	at := time.Now().UTC()
	switch to {
	case action.LifecycleActive:
		if fs.NArg() != 2 {
			return 2
		}
		v, e := parseIntentVersion(fs.Arg(1))
		if e != nil {
			_, _ = fmt.Fprintf(c.stderr, "korvun intent %s: %v\n", verb, e)
			return 2
		}
		err = recordOperatorAct(ctx, store, "intent", "activate-v2", fs.Arg(0), func() error {
			return store.ActivateIntentV2(ctx, fs.Arg(0), v, action.OperatorPrincipal().PrincipalID, at)
		})
	case action.LifecycleExpired:
		err = recordOperatorAct(ctx, store, "intent", "expire-v2", fs.Arg(0), func() error {
			return store.ExpireIntentV2(ctx, fs.Arg(0), action.OperatorPrincipal().PrincipalID, at)
		})
	case action.LifecycleRevoked:
		err = recordOperatorAct(ctx, store, "intent", "revoke-v2", fs.Arg(0), func() error {
			return store.RevokeIntentV2(ctx, fs.Arg(0), action.OperatorPrincipal().PrincipalID, at)
		})
	}
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent %s: %v\n", verb, err)
		return 1
	}
	_, _ = fmt.Fprintf(c.stdout, "intent %s %s\n", fs.Arg(0), to)
	return 0
}

func (c *cli) intentVerifyV2(args []string) int {
	fs := flag.NewFlagSet("intent verify-v2", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to config")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" || fs.NArg() != 2 {
		return 2
	}
	v, err := parseIntentVersion(fs.Arg(1))
	if err != nil {
		return 2
	}
	store, err := openOperatorStore(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent verify-v2: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()
	signed, err := store.GetIntentV2(context.Background(), fs.Arg(0), v)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent verify-v2: signed v2 intent unavailable: %v\n", err)
		return 1
	}
	_, err = store.ResolveIntentV2(context.Background(), fs.Arg(0), v, signed.Digest, time.Now().UTC())
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent verify-v2: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(c.stdout, "intent %s version %d: OK\n", fs.Arg(0), v)
	return 0
}

func (c *cli) intentBind(args []string) int {
	fs := flag.NewFlagSet("intent bind", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to config")
	actor := fs.String("actor", "", "actor principal")
	channel := fs.String("channel", "", "channel")
	conversation := fs.String("conversation", "", "exact conversation id")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" || *actor == "" || *channel == "" || fs.NArg() != 2 {
		return 2
	}
	v, err := parseIntentVersion(fs.Arg(1))
	if err != nil {
		return 2
	}
	store, err := openOperatorStoreSealed(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent bind: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()
	signed, err := store.GetIntentV2(context.Background(), fs.Arg(0), v)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent bind: signed v2 intent unavailable: %v\n", err)
		return 1
	}
	b := action.ExecutionBinding{BindingID: "bind_" + action.NewID(), ActorPrincipalID: *actor, Channel: *channel, ConversationID: *conversation, IntentID: fs.Arg(0), IntentVersion: v, IntentDigest: signed.Digest, Revision: 1, Status: action.BindingActive}
	if err = recordOperatorAct(context.Background(), store, "intent", "bind", b.IntentID, func() error {
		return store.PutExecutionBinding(context.Background(), b)
	}); err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent bind: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(c.stdout, "binding %s -> %s version %d ACTIVE\n", b.BindingID, b.IntentID, b.IntentVersion)
	return 0
}

func (c *cli) intentImportLegacy(args []string, rootOnly bool) int {
	verb := "import-legacy"
	if rootOnly {
		verb = "adopt-root"
	}
	fs := flag.NewFlagSet("intent "+verb, flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to config")
	profile := fs.String("profile", "", "profile identifier")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	id := action.RootIntentID
	if !rootOnly {
		if fs.NArg() != 1 {
			return 2
		}
		id = fs.Arg(0)
	} else if fs.NArg() != 0 {
		return 2
	}
	if *configPath == "" || *profile == "" {
		return 2
	}
	store, err := openOperatorStoreSealed(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent %s: %v\n", verb, err)
		return 1
	}
	defer func() { _ = store.Close() }()
	legacy, err := store.GetIntent(context.Background(), id)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent %s: %v\n", verb, err)
		return 1
	}
	contract, err := action.ImportLegacyIntentV2(legacy, *profile)
	if err == nil {
		err = recordOperatorAct(context.Background(), store, "intent", "import-legacy", contract.IntentID, func() error {
			return store.CreateIntentV2(context.Background(), contract, action.OperatorPrincipal().PrincipalID, time.Now().UTC())
		})
	}
	if err == nil {
		err = recordOperatorAct(context.Background(), store, "intent", "adopt-root", contract.IntentID, func() error {
			return store.ActivateIntentV2(context.Background(), contract.IntentID, contract.Version, action.OperatorPrincipal().PrincipalID, time.Now().UTC())
		})
	}
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun intent %s: %v\n", verb, err)
		return 1
	}
	_, _ = fmt.Fprintf(c.stdout, "intent %s adopted as signed version %d ACTIVE\n", id, contract.Version)
	return 0
}

// recordOperatorAct wraps one CLI mutation in its receipt: the identified
// AUTHORIZED record (operator principal, loopback evidence) lands BEFORE
// the mutation, and the terminal state tells the truth about how it went.
// A pre-validated denial records DENIED instead and never runs.
func recordOperatorAct(ctx context.Context, store *actionsqlite.Store, namespace, name, params string, mutate func() error) error {
	env, evidence, err := operatorAuthenticatedEnvelope(store, namespace, name, params)
	if err != nil {
		return err
	}
	if err := store.RecordAttemptAuthenticated(ctx, env,
		actionsqlite.Decision{Outcome: "allow", Rule: operatorRule},
		action.StateAuthorized, evidence); err != nil {
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

// operatorAuthenticatedEnvelope mints the local operator's ingress. Its
// authentication cut is the OPEN SEALED STORE the caller hands it (FR-ID-04):
// holding it means the operator already crossed the local profile's own door —
// the profile directory, its permissions and the keystore the store opened. The
// store is a PARAMETER, not a comment, so no caller can mint a capability
// without it; a nil store is refused as missing ingress (the twentieth pass,
// P1-2). The resolver and issuer are per-act by design: a local act needs no
// long-lived adapter instance, and a seal born and consumed in one act cannot be
// transplanted into another.
func operatorAuthenticatedEnvelope(store *actionsqlite.Store, namespace, name, params string) (action.Envelope, identityv2.Evidence, error) {
	if store == nil {
		return action.Envelope{}, identityv2.Evidence{}, identityv2.ErrIdentityEvidenceMissing
	}
	now := time.Now().UTC()
	resolver, err := identityv2.NewResolver(localCLIRegistry(), func() time.Time { return now })
	if err != nil {
		return action.Envelope{}, identityv2.Evidence{}, err
	}
	issuer, err := resolver.NewIssuer(identityv2.IssuerConfig{
		BindingID: "binding_cli", Method: "local_profile",
		CredentialClass: "local_profile", TTL: time.Minute,
	})
	if err != nil {
		return action.Envelope{}, identityv2.Evidence{}, err
	}
	env := action.NewEnvelope(action.NewID(), "cli",
		action.Source{Kind: "operator", Protocol: "cli", Channel: "cli"},
		action.Operation{Namespace: namespace, Name: name, Version: 1},
		params, now)
	ingress, err := issuer.Issue(env.CorrelationID, "local_operator")
	if err != nil {
		return action.Envelope{}, identityv2.Evidence{}, err
	}
	evidence, err := resolver.Resolve(ingress, identityv2.ResolveRequest{
		ActionID: env.ActionID, RequestID: env.CorrelationID,
		Channel: "cli", Brain: "cli",
	})
	if err != nil {
		return action.Envelope{}, identityv2.Evidence{}, err
	}
	env.Principal = action.PrincipalRef{
		PrincipalID:        evidence.ActorPrincipalID,
		ResponsibleHumanID: evidence.ResponsiblePrincipalID,
		EvidenceID:         evidence.EvidenceID,
	}
	env.IntentID = action.RootIntentID
	return env, evidence, nil
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
	if _, _, code, done := c.parseStyled(fs, args); done {
		return code
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
	if _, _, code, done := c.parseStyled(fs, args); done {
		return code
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
	if _, _, code, done := c.parseStyled(fs, args); done {
		return code
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
	if _, _, code, done := c.parseStyled(fs, args); done {
		return code
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
