package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"ecp/internal/ecp"
)

type CLI struct {
	Service ecp.Service
	Stdout  io.Writer
	Stderr  io.Writer
}

type envelope struct {
	SchemaVersion int       `json:"schema_version"`
	Operation     string    `json:"operation"`
	OK            bool      `json:"ok"`
	Result        any       `json:"result,omitempty"`
	PartialResult any       `json:"partial_result,omitempty"`
	Error         *errorDTO `json:"error,omitempty"`
}

type errorDTO struct {
	Kind    string `json:"kind"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type stringList []string

func (s *stringList) String() string { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func (c CLI) Run(ctx context.Context, args []string) int {
	if c.Stdout == nil {
		c.Stdout = io.Discard
	}
	if c.Stderr == nil {
		c.Stderr = io.Discard
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(c.Stdout, helpText)
		return 0
	}
	for _, argument := range args[1:] {
		if argument == "--help" || argument == "-h" {
			fmt.Fprint(c.Stdout, helpText)
			return 0
		}
	}
	operation := operationName(args)
	result, code, err := c.dispatch(ctx, args)
	if err != nil {
		var partialResult any
		if gateResult, ok := result.(ecp.GateRunResult); ok && gateResult.RunID != "" {
			partialResult = gateResult
		}
		return c.writeError(operation, err, partialResult)
	}
	if err := writeJSON(c.Stdout, envelope{SchemaVersion: ecp.SchemaVersion, Operation: operation, OK: true, Result: result}); err != nil {
		_ = writeJSON(c.Stderr, envelope{SchemaVersion: ecp.SchemaVersion, Operation: operation, OK: false, Error: &errorDTO{Kind: "RUNTIME", Code: "OUTPUT_FAILED", Message: err.Error()}})
		return 1
	}
	return code
}

func (c CLI) dispatch(ctx context.Context, args []string) (any, int, error) {
	switch args[0] {
	case "schema":
		if len(args) != 2 || args[1] != "get" {
			return nil, 0, usageError("schema requires get")
		}
		return ecp.ContractSchema(), 0, nil
	case "version":
		if len(args) != 1 {
			return nil, 0, usageError("version accepts no arguments")
		}
		identity, err := c.Service.CoreBuildIdentity()
		if err != nil {
			return nil, 0, err
		}
		return map[string]any{"core_version": ecp.CoreVersion, "core_identity": identity, "schema_version": ecp.SchemaVersion}, 0, nil
	case "project":
		return c.project(ctx, args[1:])
	case "context":
		return c.context(ctx, args[1:])
	case "policy":
		return c.policy(ctx, args[1:])
	case "truth":
		return c.truth(ctx, args[1:])
	case "change":
		return c.change(ctx, args[1:])
	case "gate":
		return c.gate(ctx, args[1:])
	case "verdict":
		return c.verdict(ctx, args[1:])
	case "acknowledgement":
		return c.acknowledgement(ctx, args[1:])
	case "evidence":
		return c.evidence(ctx, args[1:])
	case "authority":
		return c.authority(ctx, args[1:])
	default:
		return nil, 0, usageError("unknown command %q", args[0])
	}
}

func (c CLI) authority(ctx context.Context, args []string) (any, int, error) {
	if len(args) == 0 {
		return nil, 0, usageError("authority requires health, export, or verify")
	}
	switch args[0] {
	case "health":
		set := newFlagSet("authority health")
		root := set.String("root", ".", "repository path")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.AuthorityHealth(ctx, *root)
		return result, authorityHealthExitCode(result), err
	case "export":
		set := newFlagSet("authority export")
		root := set.String("root", ".", "repository path")
		output := set.String("output", "", "new private export directory outside the repository and live authority")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.ExportAuthority(ctx, *root, *output)
		return result, 0, err
	case "verify":
		set := newFlagSet("authority verify")
		bundle := set.String("bundle", "", "authority export directory")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := ecp.VerifyAuthorityExport(ctx, *bundle)
		return result, 0, err
	default:
		return nil, 0, usageError("unknown authority command %q", args[0])
	}
}

func authorityHealthExitCode(report ecp.AuthorityHealthReport) int {
	switch report.Status {
	case ecp.AuthorityHealthAttention:
		return 3
	case ecp.AuthorityHealthIndeterminate:
		return 4
	default:
		return 0
	}
}

func (c CLI) project(ctx context.Context, args []string) (any, int, error) {
	if len(args) == 0 {
		return nil, 0, usageError("project requires init, inspect, register, status, enable, or disable")
	}
	switch args[0] {
	case "init":
		set := newFlagSet("project init")
		root := set.String("root", ".", "repository path")
		name := set.String("name", "", "project name")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.InitProject(ctx, *root, *name)
		return result, 0, err
	case "inspect":
		set := newFlagSet("project inspect")
		root := set.String("root", ".", "repository path")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.InspectProject(ctx, *root)
		return result, 0, err
	case "status":
		set := newFlagSet("project status")
		root := set.String("root", ".", "repository path")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.ProjectStatus(ctx, *root)
		return result, projectStatusExitCode(result), err
	case "enable":
		set := newFlagSet("project enable")
		root := set.String("root", ".", "repository path")
		authorityID := set.String("authority", "", "exact authority ID from project status")
		workspaceID := set.String("workspace", "", "exact Workspace ID from project status")
		activationToken := set.String("activation-token", "", "exact project activation token from project status")
		configDigest := set.String("config-digest", "", "exact current accepted config digest")
		truthDigest := set.String("truth-digest", "", "exact current accepted Project Truth digest")
		actor := set.String("actor", "", "local actor label")
		reason := set.String("reason", "", "project enablement reason")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.EnableProject(ctx, *root, *authorityID, *workspaceID, *activationToken, *configDigest, *truthDigest, *actor, *reason)
		return result, projectStatusExitCode(result), err
	case "disable":
		set := newFlagSet("project disable")
		root := set.String("root", ".", "repository path")
		authorityID := set.String("authority", "", "exact authority ID from project status")
		workspaceID := set.String("workspace", "", "exact Workspace ID from project status")
		activationToken := set.String("activation-token", "", "exact project activation token from project status")
		actor := set.String("actor", "", "local actor label")
		reason := set.String("reason", "", "project disablement reason")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.DisableProject(ctx, *root, *authorityID, *workspaceID, *activationToken, *actor, *reason)
		return result, projectStatusExitCode(result), err
	case "register":
		set := newFlagSet("project register")
		root := set.String("root", ".", "repository path")
		authorityID := set.String("authority", "", "exact authority ID from project inspect")
		workspaceID := set.String("workspace", "", "exact Workspace ID from project inspect")
		configDigest := set.String("config-digest", "", "exact candidate config digest")
		truthDigest := set.String("truth-digest", "", "exact candidate Project Truth digest")
		actor := set.String("actor", "", "local actor label")
		reason := set.String("reason", "", "review reason")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.RegisterProject(ctx, *root, *authorityID, *workspaceID, *configDigest, *truthDigest, *actor, *reason)
		return result, 0, err
	default:
		return nil, 0, usageError("unknown project command %q", args[0])
	}
}

func projectStatusExitCode(status ecp.ProjectStatus) int {
	if !status.Enabled {
		return 0
	}
	switch status.Assurance {
	case ecp.ProjectAssuranceBlocked:
		return 3
	case ecp.ProjectAssuranceIndeterminate:
		return 4
	default:
		return 0
	}
}

func (c CLI) context(ctx context.Context, args []string) (any, int, error) {
	if len(args) == 0 || args[0] != "get" {
		return nil, 0, usageError("context requires get")
	}
	set := newFlagSet("context get")
	root := set.String("root", ".", "repository path")
	if err := parseFlags(set, args[1:]); err != nil {
		return nil, 0, err
	}
	result, err := c.Service.Context(ctx, *root)
	return result, 0, err
}

func (c CLI) policy(ctx context.Context, args []string) (any, int, error) {
	if len(args) == 0 {
		return nil, 0, usageError("policy requires get or accept")
	}
	switch args[0] {
	case "get":
		set := newFlagSet("policy get")
		root := set.String("root", ".", "repository path")
		digest := set.String("digest", "", "optional exact historical accepted config digest")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.AcceptedControlConfigRevision(ctx, *root, *digest)
		return result, 0, err
	case "accept":
		set := newFlagSet("policy accept")
		root := set.String("root", ".", "repository path")
		authorityID := set.String("authority", "", "exact authority ID from context")
		workspaceID := set.String("workspace", "", "exact Workspace ID from context")
		configDigest := set.String("config-digest", "", "exact candidate config digest")
		actor := set.String("actor", "", "local actor label")
		reason := set.String("reason", "", "review reason")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.AcceptPolicy(ctx, *root, *authorityID, *workspaceID, *configDigest, *actor, *reason)
		return result, 0, err
	default:
		return nil, 0, usageError("unknown policy command %q", args[0])
	}
}

func (c CLI) truth(ctx context.Context, args []string) (any, int, error) {
	if len(args) == 0 {
		return nil, 0, usageError("truth requires get, diff, or reconcile")
	}
	switch args[0] {
	case "get":
		set := newFlagSet("truth get")
		root := set.String("root", ".", "repository path")
		digest := set.String("digest", "", "optional exact historical accepted truth digest")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.AcceptedProjectTruthRevision(ctx, *root, *digest)
		return result, 0, err
	case "diff":
		set := newFlagSet("truth diff")
		root := set.String("root", ".", "repository path")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.TruthDiff(ctx, *root)
		return result, 0, err
	case "reconcile":
		set := newFlagSet("truth reconcile")
		root := set.String("root", ".", "repository path")
		authorityID := set.String("authority", "", "exact authority ID from context")
		workspaceID := set.String("workspace", "", "exact Workspace ID from context")
		activationToken := set.String("activation-token", "", "exact project activation token from context")
		changeID := set.String("change", "", "exact active Change ID")
		sourceFingerprint := set.String("source-fingerprint", "", "exact source fingerprint from context")
		previousTruth := set.String("previous-truth-digest", "", "exact accepted Project Truth digest")
		candidateTruth := set.String("candidate-truth-digest", "", "exact candidate Project Truth digest")
		behavior := set.String("behavior", "", "PRESERVED, CHANGED, or UNKNOWN")
		summary := set.String("summary", "", "semantic outcome in product language")
		actor := set.String("actor", "", "local actor label")
		reason := set.String("reason", "", "reconciliation reason")
		confirmProtected := set.Bool("confirm-protected", false, "confirm the exact protected Project Truth delta")
		var categories stringList
		var requirementResults stringList
		set.Var(&categories, "category", "affected semantic category; repeatable")
		set.Var(&requirementResults, "requirement-result", "exact RequirementAssessment JSON object; repeatable and sorted by requirement_id")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		parsedRequirementResults := make([]ecp.RequirementAssessment, 0, len(requirementResults))
		for _, value := range requirementResults {
			parsed, err := ecp.ParseRequirementAssessmentJSON(value)
			if err != nil {
				return nil, 0, err
			}
			parsedRequirementResults = append(parsedRequirementResults, parsed)
		}
		result, err := c.Service.AssessSemantic(ctx, *root, ecp.SemanticAssessmentInput{
			ExpectedAuthority:      *authorityID,
			ExpectedWorkspace:      *workspaceID,
			ExpectedActivation:     *activationToken,
			ExpectedChangeID:       *changeID,
			ExpectedSource:         *sourceFingerprint,
			ExpectedPreviousTruth:  *previousTruth,
			ExpectedCandidateTruth: *candidateTruth,
			Behavior:               ecp.SemanticBehavior(strings.ToUpper(strings.TrimSpace(*behavior))),
			Summary:                *summary,
			Categories:             categories,
			RequirementAssessments: parsedRequirementResults,
			Actor:                  *actor,
			Reason:                 *reason,
			ConfirmProtected:       *confirmProtected,
		})
		return result, 0, err
	default:
		return nil, 0, usageError("unknown truth command %q", args[0])
	}
}

func (c CLI) change(ctx context.Context, args []string) (any, int, error) {
	if len(args) == 0 {
		return nil, 0, usageError("change requires start, list, get, cancel, or complete")
	}
	switch args[0] {
	case "start":
		set := newFlagSet("change start")
		root := set.String("root", ".", "repository path")
		title := set.String("title", "", "bounded change title")
		goal := set.String("goal", "", "desired outcome")
		risk := set.String("risk", "", "low, moderate, high, or critical")
		authorityID := set.String("authority", "", "exact authority ID from context")
		workspaceID := set.String("workspace", "", "exact Workspace ID from context")
		activationToken := set.String("activation-token", "", "exact project activation token from context")
		configDigest := set.String("config-digest", "", "exact current accepted config digest from context")
		truthDigest := set.String("truth-digest", "", "exact accepted Project Truth digest from context")
		sourceFingerprint := set.String("source-fingerprint", "", "exact source fingerprint from context")
		supersedesChange := set.String("supersedes-change", "", "latest cancelled Change whose original baseline must be carried forward")
		projectPurpose := set.Bool("impact-project-purpose", false, "declare that durable project purpose or maturity may change")
		var scope, nonGoals, acceptance stringList
		var capabilities, invariants, components, decisions, contracts, truthUnknownIDs stringList
		var userJourneys, dataEffects, operationalEffects, expectedChanges, expectedPreservations, impactUnknowns stringList
		var requirementJSON, unknownDispositionJSON stringList
		set.Var(&scope, "scope", "repository-relative path root; repeatable")
		set.Var(&nonGoals, "non-goal", "explicit non-goal; repeatable")
		set.Var(&acceptance, "acceptance", "acceptance criterion; repeatable")
		set.Var(&capabilities, "impact-capability", "affected Project Truth capability ID; repeatable")
		set.Var(&invariants, "impact-invariant", "affected Project Truth invariant ID; repeatable")
		set.Var(&components, "impact-component", "affected Project Truth component ID; repeatable")
		set.Var(&decisions, "impact-decision", "affected Project Truth decision ID; repeatable")
		set.Var(&contracts, "impact-contract", "affected Project Truth contract ID; repeatable")
		set.Var(&truthUnknownIDs, "impact-unknown-id", "affected accepted Project Truth unknown ID; repeatable")
		set.Var(&unknownDispositionJSON, "unknown-disposition", "exact UnknownDisposition JSON object for an affected accepted unknown; repeatable and sorted by unknown_id")
		set.Var(&userJourneys, "impact-journey", "affected user journey; repeatable")
		set.Var(&dataEffects, "impact-data", "data effect; repeatable")
		set.Var(&operationalEffects, "impact-operation", "operational effect; repeatable")
		set.Var(&expectedChanges, "expect-change", "durable semantic outcome expected to change; repeatable")
		set.Var(&expectedPreservations, "expect-preserve", "durable semantic outcome that must remain true; repeatable")
		set.Var(&impactUnknowns, "impact-unknown", "explicit impact uncertainty; repeatable")
		set.Var(&requirementJSON, "requirement", "exact ChangeRequirement JSON object; repeatable and sorted by id")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		parsedRequirements := make([]ecp.ChangeRequirement, 0, len(requirementJSON))
		for _, value := range requirementJSON {
			parsed, err := ecp.ParseChangeRequirementJSON(value)
			if err != nil {
				return nil, 0, err
			}
			parsedRequirements = append(parsedRequirements, parsed)
		}
		parsedUnknownDispositions := make([]ecp.UnknownDisposition, 0, len(unknownDispositionJSON))
		for _, value := range unknownDispositionJSON {
			parsed, err := ecp.ParseUnknownDispositionJSON(value)
			if err != nil {
				return nil, 0, err
			}
			parsedUnknownDispositions = append(parsedUnknownDispositions, parsed)
		}
		result, err := c.Service.StartChange(ctx, *root, ecp.StartChangeInput{
			Title:              *title,
			Goal:               *goal,
			Scope:              scope,
			NonGoals:           nonGoals,
			AcceptanceCriteria: acceptance,
			Impact: ecp.ChangeImpact{
				ProjectPurpose:        *projectPurpose,
				CapabilityIDs:         capabilities,
				InvariantIDs:          invariants,
				ComponentIDs:          components,
				DecisionIDs:           decisions,
				ContractIDs:           contracts,
				UnknownIDs:            truthUnknownIDs,
				UnknownDispositions:   parsedUnknownDispositions,
				UserJourneys:          userJourneys,
				DataEffects:           dataEffects,
				OperationalEffects:    operationalEffects,
				ExpectedChanges:       expectedChanges,
				ExpectedPreservations: expectedPreservations,
				Unknowns:              impactUnknowns,
			},
			Requirements:       parsedRequirements,
			SupersedesChangeID: *supersedesChange,
			Risk:               ecp.Risk(strings.TrimSpace(*risk)),
			ExpectedAuthority:  *authorityID,
			ExpectedWorkspace:  *workspaceID,
			ExpectedActivation: *activationToken,
			ExpectedConfig:     *configDigest,
			ExpectedTruth:      *truthDigest,
			ExpectedSource:     *sourceFingerprint,
		})
		return result, 0, err
	case "complete":
		set := newFlagSet("change complete")
		root := set.String("root", ".", "repository path")
		changeID := set.String("change", "", "exact active Change ID")
		subjectDigest := set.String("subject-digest", "", "exact current Verdict subject digest")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.CompleteChange(ctx, *root, *changeID, *subjectDigest)
		return result, 0, err
	case "list":
		set := newFlagSet("change list")
		root := set.String("root", ".", "repository path")
		summary := set.Bool("summary", false, "return newest-first bounded lifecycle summaries instead of full contracts")
		state := set.String("state", "", "optional ACTIVE, COMPLETED, or CANCELLED filter for --summary")
		limit := set.Int("limit", 0, "optional maximum summary count, 1-1000; zero means all")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		if *summary {
			result, err := c.Service.ListChangeSummaries(ctx, *root, ecp.ChangeState(strings.ToUpper(strings.TrimSpace(*state))), *limit)
			return result, 0, err
		}
		if strings.TrimSpace(*state) != "" || *limit != 0 {
			return nil, 0, usageError("--state and --limit require --summary")
		}
		result, err := c.Service.ListChanges(ctx, *root)
		return result, 0, err
	case "get":
		set := newFlagSet("change get")
		root := set.String("root", ".", "repository path")
		changeID := set.String("change", "", "exact Change ID")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.GetChange(ctx, *root, *changeID)
		return result, 0, err
	case "cancel":
		set := newFlagSet("change cancel")
		root := set.String("root", ".", "repository path")
		authorityID := set.String("authority", "", "exact authority ID from change list")
		workspaceID := set.String("workspace", "", "exact Workspace ID from change list")
		changeID := set.String("change", "", "exact active Change ID")
		actor := set.String("actor", "", "local actor label")
		reason := set.String("reason", "", "cancellation reason")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.CancelChange(ctx, *root, *authorityID, *workspaceID, *changeID, *actor, *reason)
		return result, 0, err
	default:
		return nil, 0, usageError("unknown change command %q", args[0])
	}
}

func (c CLI) evidence(ctx context.Context, args []string) (any, int, error) {
	if len(args) == 0 || args[0] != "list" {
		return nil, 0, usageError("evidence requires list")
	}
	set := newFlagSet("evidence list")
	root := set.String("root", ".", "repository path")
	changeID := set.String("change", "", "Change ID; defaults to active or most recent")
	if err := parseFlags(set, args[1:]); err != nil {
		return nil, 0, err
	}
	result, err := c.Service.ListEvidence(ctx, *root, *changeID)
	return result, 0, err
}

func (c CLI) gate(ctx context.Context, args []string) (any, int, error) {
	if len(args) == 0 {
		return nil, 0, usageError("gate requires plan, run, or history")
	}
	switch args[0] {
	case "plan":
		set := newFlagSet("gate plan")
		root := set.String("root", ".", "repository path")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.PlanGates(ctx, *root)
		return result, 0, err
	case "run":
		set := newFlagSet("gate run")
		root := set.String("root", ".", "repository path")
		changeID := set.String("change", "", "exact active Change ID")
		planDigest := set.String("plan-digest", "", "exact digest from the current Gate plan")
		var gates stringList
		set.Var(&gates, "gate", "specific configured Gate ID; repeatable")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.RunGates(ctx, *root, *changeID, *planDigest, gates)
		if err != nil {
			if result.RunID != "" {
				return result, 0, err
			}
			return nil, 0, err
		}
		code := 0
		for _, evidence := range result.Evidence {
			if !evidence.Passed() {
				code = 3
				break
			}
		}
		return result, code, nil
	case "history":
		set := newFlagSet("gate history")
		root := set.String("root", ".", "repository path")
		changeID := set.String("change", "", "optional exact Change ID")
		if err := parseFlags(set, args[1:]); err != nil {
			return nil, 0, err
		}
		result, err := c.Service.ListGateRuns(ctx, *root, *changeID)
		return result, 0, err
	default:
		return nil, 0, usageError("unknown gate command %q", args[0])
	}
}

func (c CLI) verdict(ctx context.Context, args []string) (any, int, error) {
	set := newFlagSet("verdict")
	root := set.String("root", ".", "repository path")
	if err := parseFlags(set, args); err != nil {
		return nil, 0, err
	}
	result, err := c.Service.Verdict(ctx, *root)
	if err != nil {
		return nil, 0, err
	}
	code := 0
	if result.Status == ecp.VerdictBlocked {
		code = 3
	} else if result.Status == ecp.VerdictIndeterminate {
		code = 4
	}
	return result, code, nil
}

func (c CLI) acknowledgement(ctx context.Context, args []string) (any, int, error) {
	if len(args) == 0 || args[0] != "record" {
		return nil, 0, usageError("acknowledgement requires record")
	}
	set := newFlagSet("acknowledgement record")
	root := set.String("root", ".", "repository path")
	changeID := set.String("change", "", "exact active Change ID")
	subjectDigest := set.String("subject-digest", "", "exact current Verdict subject digest")
	actor := set.String("actor", "", "local actor label")
	reason := set.String("reason", "", "acknowledgement reason")
	if err := parseFlags(set, args[1:]); err != nil {
		return nil, 0, err
	}
	result, err := c.Service.RecordAcknowledgement(ctx, *root, *changeID, *subjectDigest, *actor, *reason)
	return result, 0, err
}

func (c CLI) writeError(operation string, err error, partialResult any) int {
	kind := ecp.KindRuntime
	code := "UNEXPECTED_ERROR"
	message := err.Error()
	exitCode := 1
	var typed *ecp.ECPError
	if errors.As(err, &typed) {
		kind = typed.Kind
		code = typed.Code
		message = typed.Message
		switch typed.Kind {
		case ecp.KindUsage:
			exitCode = 2
		case ecp.KindBlocked, ecp.KindConflict, ecp.KindNotFound:
			exitCode = 3
		case ecp.KindIntegrity:
			exitCode = 4
		default:
			exitCode = 1
		}
	}
	primary := envelope{
		SchemaVersion: ecp.SchemaVersion,
		Operation:     operation,
		OK:            false,
		PartialResult: partialResult,
		Error:         &errorDTO{Kind: string(kind), Code: code, Message: message},
	}
	if writeErr := writeJSON(c.Stderr, primary); writeErr != nil {
		// A permanently broken stderr cannot carry any contract. Preserve a
		// machine-readable failure on stdout when it remains available, while
		// returning runtime failure rather than the original business exit code.
		_ = writeJSON(c.Stdout, envelope{
			SchemaVersion: ecp.SchemaVersion,
			Operation:     operation,
			OK:            false,
			PartialResult: partialResult,
			Error: &errorDTO{
				Kind:    string(ecp.KindRuntime),
				Code:    "ERROR_OUTPUT_FAILED",
				Message: fmt.Sprintf("could not write the %s error envelope to stderr", code),
			},
		})
		return 1
	}
	return exitCode
}

func newFlagSet(name string) *flag.FlagSet {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	return set
}

func parseFlags(set *flag.FlagSet, args []string) error {
	if err := set.Parse(args); err != nil {
		return usageError("%s", err)
	}
	if set.NArg() != 0 {
		return usageError("unexpected positional arguments: %s", strings.Join(set.Args(), " "))
	}
	return nil
}

func usageError(format string, args ...any) error {
	return &ecp.ECPError{Kind: ecp.KindUsage, Code: "USAGE", Message: fmt.Sprintf(format, args...)}
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func operationName(args []string) string {
	if len(args) == 0 {
		return "help"
	}
	switch args[0] {
	case "schema", "project", "context", "policy", "truth", "change", "gate", "acknowledgement", "evidence", "authority":
		if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
			return args[0] + "." + args[1]
		}
	}
	return args[0]
}

const helpText = `ECP — AI-native Project Continuity and Change Control v0.3

Usage:
  ecp schema get
  ecp project init --name NAME [--root PATH]
  ecp project inspect [--root PATH]
  ecp project register --authority AUTHORITY_ID --workspace WORKSPACE_ID
                       --config-digest SHA256 --truth-digest SHA256
                       --actor ACTOR --reason REASON [--root PATH]
  ecp project status [--root PATH]
  ecp project enable --authority AUTHORITY_ID --workspace WORKSPACE_ID --activation-token SHA256
                     --config-digest SHA256 --truth-digest SHA256
                     --actor ACTOR --reason REASON [--root PATH]
  ecp project disable --authority AUTHORITY_ID --workspace WORKSPACE_ID --activation-token SHA256
                      --actor ACTOR --reason REASON [--root PATH]
  ecp context get [--root PATH]
  ecp policy get [--digest SHA256] [--root PATH]
  ecp policy accept --authority AUTHORITY_ID --workspace WORKSPACE_ID
                    --config-digest SHA256 --actor ACTOR --reason REASON [--root PATH]
  ecp truth get [--digest SHA256] [--root PATH]
  ecp truth diff [--root PATH]
  ecp truth reconcile --authority AUTHORITY_ID --workspace WORKSPACE_ID
                      --activation-token SHA256 --change CHANGE_ID
                      --source-fingerprint SHA256
                      --previous-truth-digest SHA256 --candidate-truth-digest SHA256
                      --behavior PRESERVED|CHANGED|UNKNOWN --summary TEXT
                      --category TEXT [--category TEXT ...]
                      --requirement-result JSON [--requirement-result JSON ...]
                      [--confirm-protected]
                      --actor ACTOR --reason REASON [--root PATH]
  ecp change start --title TITLE --goal GOAL --scope PATH [--scope PATH ...]
                   --acceptance TEXT [--acceptance TEXT ...]
                   [--non-goal TEXT ...] [--risk low|moderate|high|critical]
                   --authority AUTHORITY_ID --workspace WORKSPACE_ID
                   --activation-token SHA256
                   --config-digest SHA256 --truth-digest SHA256
                   --source-fingerprint SHA256
                   [--impact-capability ID ...] [--impact-invariant ID ...]
                   [--impact-component ID ...] [--impact-decision ID ...]
                   [--impact-contract ID ...] [--impact-unknown-id ID ...]
                   [--unknown-disposition JSON ...]
                   [--impact-project-purpose]
                   [--impact-journey TEXT ...] [--impact-data TEXT ...]
                   [--impact-operation TEXT ...]
                   [--expect-change TEXT ...] [--expect-preserve TEXT ...]
                   [--impact-unknown TEXT ...]
                   --requirement JSON [--requirement JSON ...]
                   [--supersedes-change CANCELLED_CHANGE_ID]
                   [--root PATH]
  ecp change list [--summary [--state ACTIVE|COMPLETED|CANCELLED] [--limit N]] [--root PATH]
  ecp change get --change CHANGE_ID [--root PATH]
  ecp change cancel --authority AUTHORITY_ID --workspace WORKSPACE_ID
                    --change CHANGE_ID --actor ACTOR --reason REASON [--root PATH]
  ecp gate plan [--root PATH]
  ecp gate run --change CHANGE_ID --plan-digest SHA256 [--gate ID ...] [--root PATH]
  ecp gate history [--change CHANGE_ID] [--root PATH]
  ecp evidence list [--change CHANGE_ID] [--root PATH]
  ecp authority health [--root PATH]
  ecp authority export --output NEW_DIRECTORY [--root PATH]
  ecp authority verify --bundle DIRECTORY
  ecp verdict [--root PATH]
  ecp acknowledgement record --change CHANGE_ID --subject-digest SHA256
                             --actor ACTOR --reason REASON [--root PATH]
  ecp change complete --change CHANGE_ID --subject-digest SHA256 [--root PATH]
  ecp version

All operation results are versioned JSON. Project status is read-only and
defaults to disabled for every unregistered Workspace. Project enablement,
disablement, existing-config registration, Draft Config acceptance, and local
acknowledgement require explicit current user intent; a Codex adapter may carry
their exact opaque preconditions internally after that intent is established.
A local PASS is not a commit, deployment, release, device, or production
attestation. Cancellation is an audited terminal state, not PASS or a policy
waiver.
`
