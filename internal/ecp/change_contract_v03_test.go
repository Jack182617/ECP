package ecp

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestChangeRequirementsRejectSilentAmbiguityAndUncoveredContract(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())

	base := StartChangeInput{
		Title: "Close requirement ambiguity", Goal: "Require an explicit decision for every contract item", Scope: []string{"src"},
		AcceptanceCriteria: []string{"loading, empty, failure, and success behavior are decided"}, Risk: RiskModerate,
		Impact: ChangeImpact{Unknowns: []string{"failure presentation was not stated in the original request"}},
	}
	withCurrentChangePreconditions(t, ctx, service, repo, &base)
	if _, err := service.StartChange(ctx, repo, base); err == nil || !isErrorCode(err, "CHANGE_REQUIREMENTS_REQUIRED") {
		t.Fatalf("Change without structured requirements was accepted: %v", err)
	}

	blocking := base
	blocking.Requirements = []ChangeRequirement{{
		ID: "failure-state", Statement: "Decide how failure is presented.", Status: RequirementBlockingUnknown,
		Verification: RequirementVerificationReview, Rationale: "The request did not specify it.", DecisionSource: "product clarification pending",
		Covers: RequirementCoverage{AcceptanceCriteria: base.AcceptanceCriteria, Unknowns: base.Impact.Unknowns},
	}}
	if _, err := service.StartChange(ctx, repo, blocking); err == nil || !isErrorCode(err, "UNRESOLVED_CHANGE_REQUIREMENT") {
		t.Fatalf("BLOCKING_UNKNOWN requirement was accepted: %v", err)
	}

	uncovered := base
	uncovered.Requirements = []ChangeRequirement{{
		ID: "failure-state", Statement: "Use an inline retry state.", Status: RequirementDecided,
		Verification: RequirementVerificationReview, Rationale: "It preserves context and offers recovery.", DecisionSource: "explicit product decision",
		Covers: RequirementCoverage{Unknowns: base.Impact.Unknowns},
	}}
	if _, err := service.StartChange(ctx, repo, uncovered); err == nil || !isErrorCode(err, "UNCOVERED_CHANGE_CONTRACT") {
		t.Fatalf("uncovered acceptance criterion was accepted: %v", err)
	}

	decided := base
	decided.Requirements = []ChangeRequirement{{
		ID: "failure-state", Statement: "Use an inline retry state and preserve entered data.", Status: RequirementDecided,
		Verification: RequirementVerificationReview, Rationale: "It keeps the recovery path deterministic.", DecisionSource: "explicit product decision",
		Covers: RequirementCoverage{AcceptanceCriteria: base.AcceptanceCriteria, Unknowns: base.Impact.Unknowns},
	}}
	if _, err := service.StartChange(ctx, repo, decided); err != nil {
		t.Fatalf("fully decided and covered requirement was rejected: %v", err)
	}
}

func TestAutomatedRequirementNeedsMappedCurrentEvidence(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	acceptance := "source remains free of whitespace errors"
	impactUnknown := "the focused test uses seed Project Truth"
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Bind requirement to Evidence", Goal: "Prevent a semantic claim from replacing Gate Evidence", Scope: []string{"src"},
		AcceptanceCriteria: []string{acceptance}, Risk: RiskModerate, Impact: ChangeImpact{Unknowns: []string{impactUnknown}},
		Requirements: []ChangeRequirement{{
			ID: "source-integrity", Statement: "The final source passes the configured whitespace check.", Status: RequirementDecided,
			Verification: RequirementVerificationAutomated, Rationale: "This property has a deterministic local check.", DecisionSource: "accepted Gate contract",
			RequiredGateIDs: []string{"source-check"}, Covers: RequirementCoverage{AcceptanceCriteria: []string{acceptance}, Unknowns: []string{impactUnknown}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := service.TruthDiff(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.AssessSemantic(ctx, repo, SemanticAssessmentInput{
		ExpectedAuthority: current.AuthorityID, ExpectedWorkspace: current.WorkspaceID, ExpectedActivation: current.ActivationToken,
		ExpectedChangeID: change.ChangeID, ExpectedSource: current.Source.Fingerprint, ExpectedPreviousTruth: diff.PreviousTruthDigest,
		ExpectedCandidateTruth: diff.CandidateTruthDigest, Behavior: SemanticBehaviorPreserved, Summary: "The requested behavior is preserved.",
		Categories: []string{"behavior"}, Actor: "reviewer", Reason: "map the exact automated requirement",
		RequirementAssessments: []RequirementAssessment{{RequirementID: "source-integrity", Outcome: RequirementOutcomeVerified, EvidenceGateIDs: []string{"source-check"}, Summary: "The requirement is mapped to the current source-check Gate."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := service.Verdict(ctx, repo)
	if err != nil || verdict.Status == VerdictPass || !containsReason(verdict.Reasons, "REQUIREMENT_EVIDENCE_UNSATISFIED") {
		t.Fatalf("requirement passed without mapped Evidence: %+v err=%v", verdict, err)
	}
	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	verdict, err = service.Verdict(ctx, repo)
	if err != nil || verdict.Status != VerdictPass {
		t.Fatalf("mapped current Evidence did not satisfy the requirement: %+v err=%v", verdict, err)
	}
}

func TestExternalRequirementRemainsPendingInsteadOfBecomingLocalPass(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	acceptance := "a real external environment confirms the integration"
	impactUnknown := "external integration behavior is outside local observation"
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Keep external evidence honest", Goal: "Do not convert a local review into external proof", Scope: []string{"src"},
		AcceptanceCriteria: []string{acceptance}, Risk: RiskModerate, Impact: ChangeImpact{Unknowns: []string{impactUnknown}},
		Requirements: []ChangeRequirement{{
			ID: "external-integration", Statement: "The deployed integration succeeds in its real environment.", Status: RequirementDecided,
			Verification: RequirementVerificationExternal, Rationale: "The local runner cannot observe the external system.", DecisionSource: "acceptance criterion",
			Covers: RequirementCoverage{AcceptanceCriteria: []string{acceptance}, Unknowns: []string{impactUnknown}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := service.TruthDiff(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.AssessSemantic(ctx, repo, SemanticAssessmentInput{
		ExpectedAuthority: current.AuthorityID, ExpectedWorkspace: current.WorkspaceID, ExpectedActivation: current.ActivationToken,
		ExpectedChangeID: change.ChangeID, ExpectedSource: current.Source.Fingerprint, ExpectedPreviousTruth: diff.PreviousTruthDigest,
		ExpectedCandidateTruth: diff.CandidateTruthDigest, Behavior: SemanticBehaviorPreserved, Summary: "Local behavior remains unchanged.",
		Categories: []string{"operations"}, Actor: "reviewer", Reason: "record the external boundary honestly",
		RequirementAssessments: []RequirementAssessment{{RequirementID: "external-integration", Outcome: RequirementOutcomeExternalPending, Summary: "No trusted external Evidence was imported."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	verdict, err := service.Verdict(ctx, repo)
	if err != nil || verdict.Status == VerdictPass || !containsReason(verdict.Reasons, "EXTERNAL_REQUIREMENT_PENDING") {
		t.Fatalf("external pending requirement was presented as local PASS: %+v err=%v", verdict, err)
	}
}

func TestCancelledChangeMustCarryOriginalBaseline(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	first, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "First attempt", Goal: "Create an edit that must remain attributable", Scope: []string{"src"},
		AcceptanceCriteria: []string{"edit remains in the governed delta"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "cancelled-attempt edit\n")
	if _, err := cancelTestChange(t, ctx, service, repo, first.ChangeID, "owner", "replace the contract without discarding the edit"); err != nil {
		t.Fatal(err)
	}
	if _, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Unlinked retry", Goal: "Try to hide the prior delta", Scope: []string{"src"},
		AcceptanceCriteria: []string{"prior edit remains visible"}, Risk: RiskModerate,
	}); err == nil || !isErrorCode(err, "CHANGE_BASELINE_CARRY_REQUIRED") {
		t.Fatalf("cancelled edits were accepted as a fresh baseline: %v", err)
	}
	second, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Linked retry", Goal: "Carry the original baseline forward", Scope: []string{"src"}, SupersedesChangeID: first.ChangeID,
		AcceptanceCriteria: []string{"prior edit remains visible"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Baseline.Fingerprint != first.Baseline.Fingerprint || second.LineageRootChangeID != first.ChangeID {
		t.Fatalf("superseding Change did not carry the original baseline: first=%+v second=%+v", first.Baseline.Ref(), second)
	}
	plan, err := service.PlanGates(ctx, repo)
	if err != nil || !containsString(plan.TouchedPaths, "src/app.txt") {
		t.Fatalf("carried edit disappeared from the new Change delta: %+v err=%v", plan, err)
	}
}

func TestInferredImpactSelectsAffectedGatesAndBlocksUnderDeclaration(t *testing.T) {
	gates := GatesConfig{SchemaVersion: SchemaVersion, Gates: []GateConfig{
		{ID: "global-fast", Description: "Universal fast check", Tier: GateTierFast, Command: []string{"git", "diff", "--check"}, WorkingDirectory: ".", TimeoutSeconds: 10, AllowedExitCodes: []int{0}, RequiredFor: []Risk{RiskHigh}, Environment: map[string]string{}},
		{ID: "api-affected", Description: "Dependent API check", Tier: GateTierAffected, Command: []string{"git", "diff", "--check"}, WorkingDirectory: ".", TimeoutSeconds: 10, AllowedExitCodes: []int{0}, RequiredFor: []Risk{RiskHigh}, ComponentIDs: []string{"api"}, Environment: map[string]string{}},
		{ID: "src-affected", Description: "Source component check", Tier: GateTierAffected, Command: []string{"git", "diff", "--check"}, WorkingDirectory: ".", TimeoutSeconds: 10, AllowedExitCodes: []int{0}, RequiredFor: []Risk{RiskHigh}, ComponentIDs: []string{"app"}, Environment: map[string]string{}},
		{ID: "docs-only", Description: "Documentation-only check", Tier: GateTierAffected, Command: []string{"git", "diff", "--check"}, WorkingDirectory: ".", TimeoutSeconds: 10, AllowedExitCodes: []int{0}, RequiredFor: []Risk{RiskHigh}, PathRoots: []string{"docs"}, Environment: map[string]string{}},
	}}
	truth := ProjectTruthConfig{
		SchemaVersion: SchemaVersion, Maturity: TruthMaturityEstablished, Purpose: "Exercise deterministic impact inference.",
		Components: []TruthComponent{
			{ID: "app", Name: "Application", Responsibility: "Own source behavior.", PathRoots: []string{"src"}, DependsOn: []string{}},
			{ID: "api", Name: "API", Responsibility: "Expose application behavior.", PathRoots: []string{"api"}, DependsOn: []string{"app"}},
		},
		Capabilities: []TruthCapability{
			{ID: "api-behavior", Name: "API behavior", Description: "Preserve dependent API behavior.", Status: "active", ComponentIDs: []string{"api"}, InvariantIDs: []string{"api-integrity"}},
			{ID: "app-behavior", Name: "Application behavior", Description: "Provide the test behavior.", Status: "active", ComponentIDs: []string{"app"}, InvariantIDs: []string{"app-integrity"}},
		},
		Invariants: []TruthInvariant{
			{ID: "api-integrity", Name: "API integrity", Statement: "Dependent API behavior remains verified.", Category: "architecture", Risk: RiskHigh, GateIDs: []string{"api-affected"}, SourceRefs: []string{"api"}},
			{ID: "app-integrity", Name: "Application integrity", Statement: "Source changes remain verified.", Category: "architecture", Risk: RiskHigh, GateIDs: []string{"src-affected"}, SourceRefs: []string{"src/app.txt"}},
		},
		Contracts: DefaultProjectTruth("inferred-impact").Contracts,
		Decisions: []TruthDecision{}, Unknowns: []TruthUnknown{},
	}
	if err := validateProjectTruth(truth, gates); err != nil {
		t.Fatal(err)
	}
	inferred := inferImpact([]string{"src/app.txt"}, truth)
	if len(inferred.ComponentIDs) != 2 || inferred.ComponentIDs[0] != "api" || inferred.ComponentIDs[1] != "app" ||
		len(inferred.CapabilityIDs) != 2 || inferred.CapabilityIDs[0] != "api-behavior" || inferred.CapabilityIDs[1] != "app-behavior" ||
		len(inferred.InvariantIDs) != 2 || inferred.InvariantIDs[0] != "api-integrity" || inferred.InvariantIDs[1] != "app-integrity" {
		t.Fatalf("path did not infer its Project Truth closure: %+v", inferred)
	}
	if missing := undeclaredInferredImpact(ChangeImpact{}, inferred); impactInferenceEmpty(missing) {
		t.Fatalf("under-declared impact was not detected: %+v", missing)
	}
	change := Change{ContractVersion: 2, DeclaredRisk: RiskHigh, Impact: ChangeImpact{
		ComponentIDs: []string{"api", "app"}, CapabilityIDs: []string{"api-behavior", "app-behavior"}, InvariantIDs: []string{"api-integrity", "app-integrity"},
	}}
	required := requiredGatesForChange(gates, truth, truth, change, RiskHigh, false, []string{"src/app.txt"})
	ids := make([]string, 0, len(required))
	for _, gate := range required {
		ids = append(ids, gate.ID)
	}
	if len(ids) != 3 || ids[0] != "global-fast" || ids[1] != "api-affected" || ids[2] != "src-affected" {
		t.Fatalf("affected Gate selection was not minimal and sufficient: %v", ids)
	}
}

func TestLegacyChangeKeepsV02RiskGateAndSubjectSemantics(t *testing.T) {
	truth := ProjectTruthConfig{
		SchemaVersion: SchemaVersion,
		Maturity:      TruthMaturityEstablished,
		Purpose:       "Preserve already-recorded v0.2 Change semantics.",
		Components: []TruthComponent{{
			ID: "core", Name: "Core", Responsibility: "Own the compatibility-sensitive source.", PathRoots: []string{"src"}, DependsOn: []string{},
		}},
		Capabilities: []TruthCapability{{
			ID: "core-behavior", Name: "Core behavior", Description: "Exercise compatibility.", Status: "active", ComponentIDs: []string{"core"}, InvariantIDs: []string{"critical-core"},
		}},
		Invariants: []TruthInvariant{{
			ID: "critical-core", Name: "Critical core", Statement: "The core path is critical for new contracts.", Category: "architecture", Risk: RiskCritical, GateIDs: []string{"critical-check"}, SourceRefs: []string{"src/app.txt"},
		}},
		Contracts: DefaultProjectTruth("legacy-compatibility").Contracts,
		Decisions: []TruthDecision{}, Unknowns: []TruthUnknown{},
	}
	gates := GatesConfig{SchemaVersion: SchemaVersion, Gates: []GateConfig{
		{ID: "moderate-check", Description: "Legacy risk check", Tier: GateTierAffected, Command: []string{"git", "diff", "--check"}, WorkingDirectory: ".", TimeoutSeconds: 10, AllowedExitCodes: []int{0}, RequiredFor: []Risk{RiskModerate}, Environment: map[string]string{}},
		{ID: "critical-check", Description: "New inferred critical check", Tier: GateTierAffected, Command: []string{"git", "diff", "--check"}, WorkingDirectory: ".", TimeoutSeconds: 10, AllowedExitCodes: []int{0}, RequiredFor: []Risk{RiskCritical}, ComponentIDs: []string{"core"}, Environment: map[string]string{}},
	}}
	legacy := Change{ContractVersion: 0, DeclaredRisk: RiskModerate, Impact: ChangeImpact{Unknowns: []string{"legacy contract had no path-derived impact"}}}
	risk := effectiveRiskForContract(legacy, []string{"src/app.txt"}, PolicyConfig{}, truth, truth, false)
	if risk != RiskModerate {
		t.Fatalf("legacy Change was retroactively raised by v0.3 inference: %s", risk)
	}
	required := requiredGatesForChange(gates, truth, truth, legacy, risk, false, []string{"src/app.txt"})
	if len(required) != 1 || required[0].ID != "moderate-check" {
		t.Fatalf("legacy Change received v0.3 selector/inference semantics: %+v", required)
	}
	digest := strings.Repeat("0", 64)
	verdict := Verdict{
		SchemaVersion: SchemaVersion, ProjectID: "project", AuthorityID: "authority", WorkspaceID: "workspace", ChangeID: "change", ActivationID: "activation",
		ContractDigest: digest, ConfigDigest: digest, TruthDigest: digest, SemanticAssessmentID: "assessment", Source: SnapshotRef{Fingerprint: digest},
		EffectiveRisk: RiskModerate, TouchedPaths: []string{"src/app.txt"}, InferredImpact: inferImpact([]string{"src/app.txt"}, truth), GateAssessments: []GateAssessment{{GateID: "moderate-check", Required: true, State: "PASS", EvidenceID: "evidence"}},
	}
	legacyDigest, err := verdictSubjectDigestForChange(legacy, verdict)
	if err != nil {
		t.Fatal(err)
	}
	v03Digest, err := verdictSubjectDigest(verdict)
	if err != nil {
		t.Fatal(err)
	}
	if legacyDigest == v03Digest {
		t.Fatal("legacy and v0.3 subject shapes unexpectedly produced the same digest")
	}
}

func withCurrentChangePreconditions(t *testing.T, ctx context.Context, service Service, repo string, input *StartChangeInput) {
	t.Helper()
	current, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedAuthority = current.AuthorityID
	input.ExpectedWorkspace = current.WorkspaceID
	input.ExpectedActivation = current.ActivationToken
	input.ExpectedConfig = current.CandidateConfig
	input.ExpectedTruth = current.AcceptedTruth
	input.ExpectedSource = current.Source.Fingerprint
}

func containsReason(reasons []VerdictReason, code string) bool {
	for _, reason := range reasons {
		if reason.Code == code {
			return true
		}
	}
	return false
}
