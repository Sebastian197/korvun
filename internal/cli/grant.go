// Copyright 2026 Sebastián Moreno Saavedra
// SPDX-License-Identifier: Apache-2.0

// The operator's act, grants half (Trust Layer Etapa 2, lote 5):
// `korvun grant issue|delegate|revoke` under a stored intent, with the
// pure §14.3 attenuation validator as the wall HERE TOO — the operator's
// CLI cannot widen either: same validator, same denial naming the
// dimension, and a refused child never touches the disk. Inactive,
// expired or revoked authority fails CLOSED with its sealed rule, and
// every act — granted or refused — leaves its identified receipt.
package cli

import (
	"context"
	"flag"
	"fmt"
	"time"

	"github.com/Sebastian197/korvun/internal/action"
	actionsqlite "github.com/Sebastian197/korvun/internal/action/sqlite"
)

// attenuationRule labels a CLI delegation refused by the wall. A NEW
// finite audit label, flagged for adjudication with `operator`: the
// widened dimension stays in the human-facing error (finite grammar on
// the trail, unbounded detail on the terminal).
const attenuationRule = "attenuation_violated"

// grantCmd dispatches the `grant` noun's verbs.
func (c *cli) grantCmd(args []string) int {
	if len(args) == 0 {
		_, _ = fmt.Fprint(c.stderr, "korvun grant: expected a subcommand: issue | delegate | revoke\nRun 'korvun help' for usage.\n")
		return 2
	}
	switch args[0] {
	case "issue":
		return c.grantIssue(args[1:])
	case "delegate":
		return c.grantDelegate(args[1:])
	case "revoke":
		return c.grantRevoke(args[1:])
	default:
		_, _ = fmt.Fprintf(c.stderr, "korvun grant: unknown subcommand %q\nRun 'korvun help' for usage.\n", args[0])
		return 2
	}
}

// recordDeniedAct leaves the DENIED receipt of a pre-validated refusal:
// the act never runs, the trail says why.
func recordDeniedAct(ctx context.Context, store *actionsqlite.Store, namespace, name, params, rule string) error {
	env, identity, err := operatorEnvelope(namespace, name, params)
	if err != nil {
		return err
	}
	return store.RecordAttemptIdentified(ctx, env,
		actionsqlite.Decision{Outcome: "deny", Rule: rule},
		action.StateDenied, identity)
}

// grantFlags is the shared flag surface of issue and delegate.
type grantFlags struct {
	fs         *flag.FlagSet
	configPath *string
	subject    *string
	operations *string
	resources  *string
	maxActions *int
	validFrom  *string
	expires    *string
	depth      *int
	ceiling    *string
}

func newGrantFlags(name string, stderr *cli) *grantFlags {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr.stderr)
	return &grantFlags{
		fs:         fs,
		configPath: fs.String("config", "", "path to the korvun config (required)"),
		subject:    fs.String("subject", "", "the receiving principal id (required)"),
		operations: fs.String("operations", "", "comma-separated operation set (required)"),
		resources:  fs.String("resources", "*", "comma-separated coarse resource set"),
		maxActions: fs.Int("max-actions", -1, "total action budget (-1 = inherit/unlimited)"),
		validFrom:  fs.String("valid-from", "", "window start, RFC3339 (default: now)"),
		expires:    fs.String("expires", "", "window end, RFC3339 (default: inherit/no expiry)"),
		depth:      fs.Int("depth", -1, "delegation depth remaining (-1 = default)"),
		ceiling:    fs.String("effect-ceiling", "", "effect-class ceiling (pure|read_external|write_reversible|write_compensatable|write_irreversible|critical; empty = no ceiling / inherit)"),
	}
}

// parseCeilingFlag validates --effect-ceiling against the finite ladder:
// "" means no ceiling (or inherit, on delegate); anything off-ladder is
// a usage error naming the valid classes.
func parseCeilingFlag(raw string) (action.EffectClass, error) {
	if raw == "" {
		return "", nil
	}
	class := action.EffectClass(raw)
	if !class.Known() {
		return "", fmt.Errorf("--effect-ceiling: %q is not on the effect ladder (pure, read_external, write_reversible, write_compensatable, write_irreversible, critical)", raw)
	}
	return class, nil
}

// grantIssue implements `korvun grant issue`: a grant born directly from
// a stored intent, which must be IN FORCE at the issuing instant — the
// sealed validity rules deny everything else, receipt included.
func (c *cli) grantIssue(args []string) int {
	gf := newGrantFlags("grant issue", c)
	intentID := gf.fs.String("intent", "", "the intent the grant lives under (required)")
	if err := gf.fs.Parse(args); err != nil {
		return 2
	}
	if *gf.configPath == "" || *intentID == "" || *gf.subject == "" || *gf.operations == "" {
		_, _ = fmt.Fprint(c.stderr, "korvun grant issue: --config, --intent, --subject and --operations are required\n")
		return 2
	}
	store, err := openOperatorStoreSealed(*gf.configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun grant issue: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	now := time.Now().UTC()
	intent, err := store.GetIntent(ctx, *intentID)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun grant issue: %v\n", err)
		return 1
	}
	params := contractParams(map[string]any{
		"intent_id": *intentID, "subject": *gf.subject, "operations": *gf.operations,
	})
	// Fail-closed: only an intent IN FORCE can source authority.
	if rule := action.ValidateIntentAt(intent, now); rule != "" {
		if err := recordDeniedAct(ctx, store, "grant", "issue", params, rule); err != nil {
			_, _ = fmt.Fprintf(c.stderr, "korvun grant issue: record refusal: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(c.stderr, "korvun grant issue: denied (%s): intent %s does not authorize at %s\n",
			rule, *intentID, now.Format(time.RFC3339))
		return 1
	}
	grant, code := c.buildGrantFromFlags(gf, *intentID,
		action.OperatorPrincipal().PrincipalID, now, 0, "issue")
	if code != 0 {
		return code
	}
	if err := recordOperatorAct(ctx, store, "grant", "issue", params, func() error {
		return store.CreateGrant(ctx, grant)
	}); err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun grant issue: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(c.stdout, "grant %s issued under %s (ACTIVE)\n", grant.GrantID, *intentID)
	return 0
}

// buildGrantFromFlags assembles the grant a verb persists. defaultDepth
// is used when --depth is not given (issue: 0 unless set; delegate:
// parent-1). Returns a non-zero exit code on a flag error.
func (c *cli) buildGrantFromFlags(gf *grantFlags, intentID, issuer string, now time.Time, defaultDepth int, verb string) (action.AuthorityGrant, int) {
	from, err := parseTimeFlag(*gf.validFrom, "valid-from")
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun grant %s: %v\n", verb, err)
		return action.AuthorityGrant{}, 2
	}
	if from.IsZero() {
		from = now
	}
	until, err := parseTimeFlag(*gf.expires, "expires")
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun grant %s: %v\n", verb, err)
		return action.AuthorityGrant{}, 2
	}
	budget := 0
	if *gf.maxActions >= 0 {
		budget = *gf.maxActions
	}
	depth := defaultDepth
	if *gf.depth >= 0 {
		depth = *gf.depth
	}
	ceiling, err := parseCeilingFlag(*gf.ceiling)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun grant %s: %v\n", verb, err)
		return action.AuthorityGrant{}, 2
	}
	return action.AuthorityGrant{
		GrantID:                  action.NewGrantID(),
		IntentID:                 intentID,
		IssuerPrincipalID:        issuer,
		SubjectPrincipalID:       *gf.subject,
		Operations:               splitCSV(*gf.operations),
		ResourceScope:            splitCSV(*gf.resources),
		Budgets:                  action.Budgets{MaxActions: budget},
		ValidFrom:                from,
		ExpiresAt:                until,
		DelegationDepthRemaining: depth,
		EffectCeiling:            ceiling,
		Status:                   action.LifecycleActive,
	}, 0
}

// grantDelegate implements `korvun grant delegate`: the child inherits
// the parent's intent, expiry and budget unless narrowed by flags, its
// issuer is the parent's SUBJECT, and the §14.3 wall runs BEFORE any
// write — a widening child never touches the disk, here either.
func (c *cli) grantDelegate(args []string) int {
	gf := newGrantFlags("grant delegate", c)
	parentID := gf.fs.String("parent", "", "the parent grant id (required)")
	if err := gf.fs.Parse(args); err != nil {
		return 2
	}
	if *gf.configPath == "" || *parentID == "" || *gf.subject == "" || *gf.operations == "" {
		_, _ = fmt.Fprint(c.stderr, "korvun grant delegate: --config, --parent, --subject and --operations are required\n")
		return 2
	}
	store, err := openOperatorStoreSealed(*gf.configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun grant delegate: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	now := time.Now().UTC()
	parent, err := store.GetGrant(ctx, *parentID)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun grant delegate: %v\n", err)
		return 1
	}
	params := contractParams(map[string]any{
		"parent": *parentID, "subject": *gf.subject, "operations": *gf.operations,
	})
	// Fail-closed: only a parent IN FORCE can be passed on.
	if rule := action.ValidateGrantAt(parent, now); rule != "" {
		if err := recordDeniedAct(ctx, store, "grant", "delegate", params, rule); err != nil {
			_, _ = fmt.Fprintf(c.stderr, "korvun grant delegate: record refusal: %v\n", err)
			return 1
		}
		_, _ = fmt.Fprintf(c.stderr, "korvun grant delegate: denied (%s): parent %s does not authorize at %s\n",
			rule, *parentID, now.Format(time.RFC3339))
		return 1
	}
	// Inherit-then-narrow defaults: the parent's expiry and budget unless
	// flags narrow them, depth parent-1.
	if *gf.expires == "" && !parent.ExpiresAt.IsZero() {
		inherited := parent.ExpiresAt.Format(time.RFC3339)
		gf.expires = &inherited
	}
	if *gf.maxActions < 0 && parent.Budgets.MaxActions != 0 {
		inherited := parent.Budgets.MaxActions
		gf.maxActions = &inherited
	}
	if *gf.ceiling == "" && parent.EffectCeiling != "" {
		inherited := string(parent.EffectCeiling)
		gf.ceiling = &inherited
	}
	child, code := c.buildGrantFromFlags(gf, parent.IntentID,
		parent.SubjectPrincipalID, now, parent.DelegationDepthRemaining-1, "delegate")
	if code != 0 {
		return code
	}
	child.ParentGrantID = parent.GrantID
	// THE WALL: the same pure validator, before any write.
	if err := action.ValidateAttenuation(parent, child); err != nil {
		if recErr := recordDeniedAct(ctx, store, "grant", "delegate", params, attenuationRule); recErr != nil {
			_, _ = fmt.Fprintf(c.stderr, "korvun grant delegate: record refusal: %v\n", recErr)
			return 1
		}
		_, _ = fmt.Fprintf(c.stderr, "korvun grant delegate: denied: %v\n", err)
		return 1
	}
	if err := recordOperatorAct(ctx, store, "grant", "delegate", params, func() error {
		return store.DelegateGrant(ctx, child)
	}); err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun grant delegate: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(c.stdout, "grant %s delegated from %s (ACTIVE)\n", child.GrantID, parent.GrantID)
	return 0
}

// grantRevoke implements `korvun grant revoke`.
func (c *cli) grantRevoke(args []string) int {
	fs := flag.NewFlagSet("grant revoke", flag.ContinueOnError)
	fs.SetOutput(c.stderr)
	configPath := fs.String("config", "", "path to the korvun config (required)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *configPath == "" || fs.NArg() != 1 {
		_, _ = fmt.Fprint(c.stderr, "korvun grant revoke: usage: korvun grant revoke --config <path> <grant-id>\n")
		return 2
	}
	id := fs.Arg(0)
	store, err := openOperatorStoreSealed(*configPath)
	if err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun grant revoke: %v\n", err)
		return 1
	}
	defer func() { _ = store.Close() }()
	ctx := context.Background()
	params := contractParams(map[string]any{"grant_id": id, "to": string(action.LifecycleRevoked)})
	if err := recordOperatorAct(ctx, store, "grant", "revoke", params, func() error {
		return store.TransitionGrant(ctx, id, action.LifecycleRevoked)
	}); err != nil {
		_, _ = fmt.Fprintf(c.stderr, "korvun grant revoke: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(c.stdout, "grant %s REVOKED\n", id)
	return 0
}
