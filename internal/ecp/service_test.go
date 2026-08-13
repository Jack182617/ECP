package ecp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWorkflowPassStaleRerunAndComplete(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())

	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title:              "Update app behavior",
		Goal:               "The tracked app file contains the new behavior",
		Scope:              []string{"src"},
		NonGoals:           []string{"Do not change project policy"},
		AcceptanceCriteria: []string{"git diff check passes"},
		Risk:               RiskModerate,
	})
	if err != nil {
		t.Fatalf("start change: %v", err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "new behavior\n")

	plan, err := service.PlanGates(ctx, repo)
	if err != nil {
		t.Fatalf("plan gates: %v", err)
	}
	if plan.AuthorityID == "" || plan.WorkspaceID == "" || plan.PlanDigest == "" || plan.ChangeID != change.ChangeID || len(plan.Gates) != 1 || plan.Gates[0].ID != "source-check" {
		t.Fatalf("unexpected gate plan: %+v", plan)
	}

	run, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil)
	if err != nil {
		t.Fatalf("run gates: %v", err)
	}
	if len(run.Evidence) != 1 || !run.Evidence[0].Passed() || run.Evidence[0].PlanDigest != plan.PlanDigest {
		t.Fatalf("expected passing evidence: %+v", run)
	}
	verdict, err := service.Verdict(ctx, repo)
	if err != nil {
		t.Fatalf("verdict: %v", err)
	}
	if verdict.Status != VerdictPass {
		t.Fatalf("expected PASS, got %+v", verdict)
	}

	appendFile(t, filepath.Join(repo, "src", "app.txt"), "changed after evidence\n")
	stale, err := service.Verdict(ctx, repo)
	if err != nil {
		t.Fatalf("stale verdict: %v", err)
	}
	if stale.Status != VerdictBlocked || !hasReason(stale, "GATE_EVIDENCE_STALE") {
		t.Fatalf("expected stale evidence to block, got %+v", stale)
	}

	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatalf("rerun gates: %v", err)
	}
	refreshed, err := service.Verdict(ctx, repo)
	if err != nil || refreshed.Status != VerdictPass {
		t.Fatalf("expected refreshed PASS, verdict=%+v err=%v", refreshed, err)
	}
	if _, err := service.CompleteChange(ctx, repo, change.ChangeID, digestBytes([]byte("stale completion subject"))); err == nil || !isErrorCode(err, "SUBJECT_DIGEST_MISMATCH") {
		t.Fatalf("completion accepted a stale subject digest: %v", err)
	}
	completion, err := service.CompleteChange(ctx, repo, change.ChangeID, refreshed.SubjectDigest)
	if err != nil {
		t.Fatalf("complete change: %v", err)
	}
	if completion.ChangeID != change.ChangeID || completion.SubjectDigest != refreshed.SubjectDigest {
		t.Fatalf("completion is not bound to final subject: %+v", completion)
	}
	contextResult, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatalf("context after completion: %v", err)
	}
	if contextResult.ActiveChange != nil {
		t.Fatalf("completed change remained active: %+v", contextResult.ActiveChange)
	}
	history, err := service.ListChanges(ctx, repo)
	if err != nil || len(history) != 1 || history[0].Completion == nil || history[0].Completion.FinalVerdict.Status != VerdictPass {
		t.Fatalf("completed Change history is not verifiable: %+v err=%v", history, err)
	}
	evidence, err := service.ListEvidence(ctx, repo, change.ChangeID)
	if err != nil || evidence.ChangeState != ChangeCompleted || len(evidence.Evidence) < 2 {
		t.Fatalf("completed Change Evidence is not queryable: %+v err=%v", evidence, err)
	}
}

func TestVerdictIsDeterministicApartFromObservationTime(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	observedAt := time.Date(2026, 8, 12, 1, 0, 0, 0, time.UTC)
	service.Clock = func() time.Time { return observedAt }
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Deterministic Verdict", Goal: "Prove repeated evaluation preserves the exact subject", Scope: []string{"src"},
		AcceptanceCriteria: []string{"repeated Verdict fields are stable"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "deterministic verdict\n")
	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	first, err := service.Verdict(ctx, repo)
	if err != nil || first.Status != VerdictPass {
		t.Fatalf("first Verdict was not PASS: %+v err=%v", first, err)
	}
	observedAt = observedAt.Add(5 * time.Minute)
	second, err := service.Verdict(ctx, repo)
	if err != nil || second.Status != VerdictPass {
		t.Fatalf("second Verdict was not PASS: %+v err=%v", second, err)
	}
	if first.EvaluatedAt == second.EvaluatedAt {
		t.Fatal("test did not exercise distinct Verdict observation metadata")
	}
	first.EvaluatedAt = time.Time{}
	second.EvaluatedAt = time.Time{}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same exact subject produced non-deterministic Verdict fields: first=%+v second=%+v", first, second)
	}
}

func TestSemanticReconciliationIsRequiredForPass(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Require semantic reconciliation", Goal: "Prove Gate PASS alone is insufficient", Scope: []string{"src"},
		AcceptanceCriteria: []string{"semantic reconciliation is exact"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "semantically preserved\n")
	plan, err := service.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RunGates(ctx, repo, change.ChangeID, plan.PlanDigest, nil); err != nil {
		t.Fatal(err)
	}
	blocked, err := service.Verdict(ctx, repo)
	if err != nil || blocked.Status != VerdictBlocked || !hasReason(blocked, "SEMANTIC_ASSESSMENT_REQUIRED") {
		t.Fatalf("Gate PASS bypassed semantic reconciliation: %+v err=%v", blocked, err)
	}
	current, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := service.TruthDiff(ctx, repo)
	if err != nil || diff.Changed {
		t.Fatalf("unexpected Project Truth diff: %+v err=%v", diff, err)
	}
	assessment, err := assessTestSemantic(t, ctx, service, repo, SemanticAssessmentInput{
		ExpectedAuthority:      current.AuthorityID,
		ExpectedWorkspace:      current.WorkspaceID,
		ExpectedActivation:     current.ActivationToken,
		ExpectedChangeID:       change.ChangeID,
		ExpectedSource:         current.Source.Fingerprint,
		ExpectedPreviousTruth:  diff.PreviousTruthDigest,
		ExpectedCandidateTruth: diff.CandidateTruthDigest,
		Behavior:               SemanticBehaviorPreserved,
		Summary:                "The user-visible and durable project semantics remain unchanged.",
		Categories:             []string{"behavior"},
		Actor:                  "reviewer",
		Reason:                 "reviewed the exact final source and accepted truth",
	})
	if err != nil || assessment.AssessmentID == "" {
		t.Fatalf("exact preserved assessment failed: %+v err=%v", assessment, err)
	}
	passed, err := service.Verdict(ctx, repo)
	if err != nil || passed.Status != VerdictPass || passed.SemanticAssessmentID != assessment.AssessmentID {
		t.Fatalf("applicable semantic reconciliation did not unlock PASS: %+v err=%v", passed, err)
	}
}

func TestUnknownSemanticOutcomeCannotPass(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Unknown semantic outcome", Goal: "Keep uncertainty fail closed", Scope: []string{"src"},
		AcceptanceCriteria: []string{"UNKNOWN blocks completion"}, Risk: RiskModerate,
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
	if _, err := assessTestSemantic(t, ctx, service, repo, SemanticAssessmentInput{
		ExpectedAuthority:      current.AuthorityID,
		ExpectedWorkspace:      current.WorkspaceID,
		ExpectedActivation:     current.ActivationToken,
		ExpectedChangeID:       change.ChangeID,
		ExpectedSource:         current.Source.Fingerprint,
		ExpectedPreviousTruth:  diff.PreviousTruthDigest,
		ExpectedCandidateTruth: diff.CandidateTruthDigest,
		Behavior:               SemanticBehaviorUnknown,
		Summary:                "The available evidence cannot establish whether behavior is preserved.",
		Categories:             []string{"unknown"},
		Actor:                  "reviewer",
		Reason:                 "record unresolved uncertainty without granting PASS",
	}); err != nil {
		t.Fatal(err)
	}
	plan, err := service.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RunGates(ctx, repo, change.ChangeID, plan.PlanDigest, nil); err != nil {
		t.Fatal(err)
	}
	verdict, err := service.Verdict(ctx, repo)
	if err != nil || verdict.Status != VerdictBlocked || !hasReason(verdict, "SEMANTIC_OUTCOME_UNKNOWN") {
		t.Fatalf("UNKNOWN semantic outcome received a stronger verdict: %+v err=%v", verdict, err)
	}
}

func TestProtectedProjectTruthEvolutionRequiresExactConfirmation(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	invariantGate := passingGitGate()
	invariantGate.ID = "invariant-check"
	invariantGate.Description = "Validate the durable behavior invariant"
	invariantGate.RequiredFor = []Risk{RiskCritical}
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate(), invariantGate)
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Establish Project Truth", Goal: "Replace the bootstrap seed with evidence-backed truth", Scope: []string{"src"},
		AcceptanceCriteria: []string{"truth is established and accepted"}, Risk: RiskModerate,
		Impact: ChangeImpact{
			ProjectPurpose: true,
			ContractIDs:    []string{"project-boundaries"},
			UnknownIDs:     []string{"project-purpose-unreviewed"},
			Unknowns:       []string{"new established truth entities cannot be referenced before they are accepted"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "documented behavior\n")
	truth := ProjectTruthConfig{
		SchemaVersion: SchemaVersion,
		Maturity:      TruthMaturityEstablished,
		Purpose:       "Provide a deterministic test product whose governed changes preserve documented behavior.",
		Capabilities: []TruthCapability{{
			ID: "deliver-product", Name: "Deliver product", Description: "Expose the repository's documented product behavior.", Status: "active",
			ComponentIDs: []string{"product-core"}, InvariantIDs: []string{"preserve-durable-behavior"},
		}},
		Invariants: []TruthInvariant{{
			ID: "preserve-durable-behavior", Name: "Preserve durable behavior", Statement: "Governed changes must preserve or explicitly evolve documented behavior.",
			Category: "business", Risk: RiskHigh, GateIDs: []string{"invariant-check"}, SourceRefs: []string{"src/app.txt"},
		}},
		Components: []TruthComponent{{
			ID: "product-core", Name: "Product core", Responsibility: "Own the test product behavior.", PathRoots: []string{"src"}, DependsOn: []string{},
		}},
		Decisions: []TruthDecision{},
		Contracts: []TruthContractRef{{
			ID: "project-boundaries", Kind: "boundary", Path: "contracts/boundaries.md", Description: "Stable product and engineering boundaries.",
		}},
		Unknowns: []TruthUnknown{},
	}
	if err := writePrettyJSON(filepath.Join(repo, ".ecp", "truth.json"), truth, 0o644); err != nil {
		t.Fatal(err)
	}
	diff, err := service.TruthDiff(ctx, repo)
	if err != nil || !diff.Changed || !diff.ProtectedChange || len(diff.Delta) == 0 {
		t.Fatalf("protected truth delta was not exposed: %+v err=%v", diff, err)
	}
	if _, err := service.PlanGates(ctx, repo); err == nil || !isErrorCode(err, "PROJECT_TRUTH_NOT_ACCEPTED") {
		t.Fatalf("Gate planning used unaccepted Project Truth: %v", err)
	}
	current, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	input := SemanticAssessmentInput{
		ExpectedAuthority:      current.AuthorityID,
		ExpectedWorkspace:      current.WorkspaceID,
		ExpectedActivation:     current.ActivationToken,
		ExpectedChangeID:       change.ChangeID,
		ExpectedSource:         current.Source.Fingerprint,
		ExpectedPreviousTruth:  diff.PreviousTruthDigest,
		ExpectedCandidateTruth: diff.CandidateTruthDigest,
		Behavior:               SemanticBehaviorChanged,
		Summary:                "The onboarding Change establishes the durable product purpose, capability, invariant, and component boundary.",
		Categories:             []string{"project-truth", "architecture"},
		Actor:                  "owner",
		Reason:                 "reviewed and approved the exact protected Project Truth delta",
	}
	if _, err := assessTestSemantic(t, ctx, service, repo, input); err == nil || !isErrorCode(err, "PROTECTED_TRUTH_CONFIRMATION_REQUIRED") {
		t.Fatalf("protected Project Truth changed without explicit confirmation: %v", err)
	}
	input.ConfirmProtected = true
	assessment, err := assessTestSemantic(t, ctx, service, repo, input)
	if err != nil || !assessment.ProtectedConfirmed {
		t.Fatalf("exact protected reconciliation failed: %+v err=%v", assessment, err)
	}
	status, err := service.ProjectStatus(ctx, repo)
	if err != nil || status.TruthState != ProjectConfigAccepted || status.AcceptedTruthDigest != diff.CandidateTruthDigest {
		t.Fatalf("accepted Project Truth did not become authoritative: %+v err=%v", status, err)
	}
	previousView, err := service.AcceptedProjectTruthRevision(ctx, repo, change.TruthDigest)
	if err != nil || previousView.Truth.Maturity != TruthMaturitySeed {
		t.Fatalf("previous accepted Project Truth revision was not recoverable: %+v err=%v", previousView, err)
	}
	currentView, err := service.AcceptedProjectTruth(ctx, repo)
	if err != nil || currentView.Truth.Maturity != TruthMaturityEstablished || currentView.TruthDigest != diff.CandidateTruthDigest {
		t.Fatalf("current accepted Project Truth revision was not recoverable: %+v err=%v", currentView, err)
	}
	plan, err := service.PlanGates(ctx, repo)
	if err != nil || plan.EffectiveRisk != RiskHigh || len(plan.Gates) != 2 {
		t.Fatalf("impacted invariant did not raise risk and require its exact Gate: %+v err=%v", plan, err)
	}
	if _, err := service.RunGates(ctx, repo, change.ChangeID, plan.PlanDigest, []string{"source-check"}); err != nil {
		t.Fatal(err)
	}
	blocked, err := service.Verdict(ctx, repo)
	if err != nil || blocked.Status != VerdictBlocked || blocked.EffectiveRisk != RiskHigh || !hasReason(blocked, "ACKNOWLEDGEMENT_REQUIRED") || !hasReasonGate(blocked, "GATE_EVIDENCE_MISSING", "invariant-check") {
		t.Fatalf("high-risk invariant did not enforce acknowledgement and invariant Gate Evidence: %+v err=%v", blocked, err)
	}
	if _, err := service.RunGates(ctx, repo, change.ChangeID, plan.PlanDigest, []string{"invariant-check"}); err != nil {
		t.Fatal(err)
	}
	blocked, err = service.Verdict(ctx, repo)
	if err != nil || blocked.Status != VerdictBlocked || !hasReason(blocked, "ACKNOWLEDGEMENT_REQUIRED") || hasReason(blocked, "GATE_EVIDENCE_MISSING") {
		t.Fatalf("exact invariant Gate Evidence did not satisfy the executable requirement: %+v err=%v", blocked, err)
	}
	for _, reason := range blocked.Reasons {
		if reason.Code != "ACKNOWLEDGEMENT_REQUIRED" {
			t.Fatalf("high-risk truth evolution retained an unexpected non-acknowledgement blocker before acknowledgement: %+v", blocked)
		}
	}
	if _, err := service.RecordAcknowledgement(ctx, repo, change.ChangeID, blocked.SubjectDigest, "owner", "reviewed the exact high-risk Project Truth evolution subject"); err != nil {
		t.Fatal(err)
	}
	verdict, err := service.Verdict(ctx, repo)
	if err != nil || verdict.Status != VerdictPass || verdict.SemanticAssessmentID != assessment.AssessmentID {
		t.Fatalf("reconciled and acknowledged Project Truth did not bind final PASS: %+v err=%v", verdict, err)
	}
}

func TestTruthEvolutionCannotLowerItsOwnRiskOrDropItsStartingGate(t *testing.T) {
	starting := ProjectTruthConfig{
		Invariants: []TruthInvariant{{
			ID: "protected-invariant", Risk: RiskCritical, GateIDs: []string{"old-protection"},
		}},
	}
	current := ProjectTruthConfig{
		Invariants: []TruthInvariant{{
			ID: "protected-invariant", Risk: RiskLow, GateIDs: []string{},
		}},
	}
	change := Change{
		DeclaredRisk: RiskModerate,
		Impact:       ChangeImpact{InvariantIDs: []string{"protected-invariant"}},
	}
	risk := effectiveRiskForChange(change, nil, PolicyConfig{}, starting, current, true)
	if risk != RiskCritical {
		t.Fatalf("truth evolution lowered the starting invariant risk: %s", risk)
	}
	gates := requiredGatesForChange(GatesConfig{Gates: []GateConfig{{
		ID: "old-protection", RequiredFor: []Risk{RiskLow},
	}}}, starting, current, change, risk, true, nil)
	if len(gates) != 1 || gates[0].ID != "old-protection" {
		t.Fatalf("truth evolution dropped the starting invariant Gate: %+v", gates)
	}
}

func TestCoreBuildIdentityDoesNotRequireAuthorityStateOrHome(t *testing.T) {
	t.Setenv("HOME", "")
	identity, err := (Service{CoreIdentity: "test-core-identity"}).CoreBuildIdentity()
	if err != nil || identity != "test-core-identity" {
		t.Fatalf("version identity unexpectedly depended on authority state: %q err=%v", identity, err)
	}
}

func TestChangeCanAdoptPreexistingCandidateTruthDriftWithoutUsingItAsAuthority(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	appendFile(t, filepath.Join(repo, ".ecp", "contracts", "boundaries.md"), "candidate clarification\n")
	status, err := service.ProjectStatus(ctx, repo)
	if err != nil || status.TruthState != ProjectConfigPendingAcceptance || status.AcceptedTruthDigest == status.CandidateTruthDigest {
		t.Fatalf("preexisting truth drift was not visible: %+v err=%v", status, err)
	}
	current, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	change, err := service.StartChange(ctx, repo, StartChangeInput{
		Title: "Adopt candidate truth", Goal: "Review an existing candidate boundary clarification", Scope: []string{"."},
		AcceptanceCriteria: []string{"candidate truth is either reconciled or rejected"},
		Impact:             ChangeImpact{ContractIDs: []string{"project-boundaries"}},
		Requirements: []ChangeRequirement{{
			ID: "candidate-truth", Statement: "Review and either accept or reject the existing candidate boundary clarification.", Status: RequirementDecided,
			Verification: RequirementVerificationReview, Rationale: "The candidate already exists but is not authority.", DecisionSource: "focused recovery regression",
			Covers: RequirementCoverage{AcceptanceCriteria: []string{"candidate truth is either reconciled or rejected"}},
		}},
		Risk:               RiskModerate,
		ExpectedAuthority:  current.AuthorityID,
		ExpectedWorkspace:  current.WorkspaceID,
		ExpectedActivation: current.ActivationToken,
		ExpectedConfig:     current.CandidateConfig,
		ExpectedTruth:      current.AcceptedTruth,
		ExpectedSource:     current.Source.Fingerprint,
	})
	if err != nil || change.TruthDigest != current.AcceptedTruth || change.TruthDigest == current.CandidateTruth {
		t.Fatalf("candidate drift became authority or could not enter reconciliation: %+v err=%v", change, err)
	}
	diff, err := service.TruthDiff(ctx, repo)
	if err != nil || !diff.Changed || diff.PreviousTruthDigest != change.TruthDigest || diff.CandidateTruthDigest != current.CandidateTruth {
		t.Fatalf("active Change lost the preexisting truth delta: %+v err=%v", diff, err)
	}
}

func TestImpactUnknownCannotModifyExistingProtectedFact(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Bound unknown impact", Goal: "Prevent an uncertainty wildcard", Scope: []string{"src"},
		AcceptanceCriteria: []string{"existing protected facts require exact references"},
		Impact:             ChangeImpact{Unknowns: []string{"A genuinely new fact may be discovered"}},
		Risk:               RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, ".ecp", "contracts", "boundaries.md"), "silently broaden an existing contract\n")
	current, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := service.TruthDiff(ctx, repo)
	if err != nil || !diff.Changed {
		t.Fatalf("existing contract drift was not visible: %+v err=%v", diff, err)
	}
	_, err = assessTestSemantic(t, ctx, service, repo, SemanticAssessmentInput{
		ExpectedAuthority:      current.AuthorityID,
		ExpectedWorkspace:      current.WorkspaceID,
		ExpectedActivation:     current.ActivationToken,
		ExpectedChangeID:       change.ChangeID,
		ExpectedSource:         current.Source.Fingerprint,
		ExpectedPreviousTruth:  diff.PreviousTruthDigest,
		ExpectedCandidateTruth: diff.CandidateTruthDigest,
		Behavior:               SemanticBehaviorChanged,
		Summary:                "An existing contract changed outside the declared Impact.",
		Categories:             []string{"project-truth"},
		ConfirmProtected:       true,
		Actor:                  "reviewer",
		Reason:                 "prove that a generic unknown cannot expand the contract",
	})
	if err == nil || !isErrorCode(err, "TRUTH_DELTA_OUTSIDE_CHANGE_IMPACT") {
		t.Fatalf("generic impact unknown accepted an existing protected fact change: %v", err)
	}
}

func TestUnreferencedProjectTruthContractFileIsRejected(t *testing.T) {
	repo := createTestRepository(t)
	service := newTestService(t)
	if _, err := service.InitProject(context.Background(), repo, "orphan-contract"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".ecp", "contracts", "orphan.md"), []byte("# Hidden contract\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(repo); err == nil || !isErrorCode(err, "UNREFERENCED_TRUTH_FILE") {
		t.Fatalf("unreferenced Project Truth contract file was accepted: %v", err)
	}
}

func TestAcceptedProjectTruthContentSurvivesCandidateDriftAndDetectsCorruption(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())

	before, err := service.AcceptedProjectTruth(ctx, repo)
	if err != nil || before.TruthDigest == "" || len(before.Files) != 2 {
		t.Fatalf("accepted Project Truth was not recoverable: %+v err=%v", before, err)
	}
	var acceptedContract AcceptedTruthFile
	for _, file := range before.Files {
		if file.Path == "contracts/boundaries.md" {
			acceptedContract = file
		}
	}
	if acceptedContract.Digest == "" || !strings.Contains(acceptedContract.Content, "Project Boundaries") {
		t.Fatalf("accepted contract content is incomplete: %+v", acceptedContract)
	}

	appendFile(t, filepath.Join(repo, ".ecp", "contracts", "boundaries.md"), "unaccepted candidate drift\n")
	after, err := service.AcceptedProjectTruth(ctx, repo)
	if err != nil || after.TruthDigest != before.TruthDigest {
		t.Fatalf("candidate drift displaced accepted truth: before=%+v after=%+v err=%v", before, after, err)
	}
	for _, file := range after.Files {
		if file.Path == acceptedContract.Path && file.Content != acceptedContract.Content {
			t.Fatalf("authority returned candidate bytes instead of accepted bytes: before=%q after=%q", acceptedContract.Content, file.Content)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, ".ecp", "truth.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	malformedCandidate, err := service.AcceptedProjectTruth(ctx, repo)
	if err != nil || malformedCandidate.TruthDigest != before.TruthDigest {
		t.Fatalf("malformed candidate prevented authority-only truth recovery: %+v err=%v", malformedCandidate, err)
	}

	_, workspace, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	blob := filepath.Join(workspace.Store.Directory(), "truth-blobs", strings.TrimPrefix(acceptedContract.Digest, "sha256:"))
	if err := os.WriteFile(blob, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AcceptedProjectTruth(ctx, repo); err == nil || !isErrorCode(err, "TRUTH_BLOB_DIGEST_MISMATCH") {
		t.Fatalf("corrupt accepted truth blob did not fail closed: %v", err)
	}
}

func TestAcceptedControlConfigSurvivesMalformedCandidate(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	accepted, err := service.AcceptedControlConfigRevision(ctx, repo, "")
	if err != nil || accepted.ConfigDigest == "" || len(accepted.Gates.Gates) != 1 {
		t.Fatalf("accepted control config was not recoverable: %+v err=%v", accepted, err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".ecp", "gates.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recovered, err := service.AcceptedControlConfigRevision(ctx, repo, accepted.ConfigDigest)
	if err != nil || recovered.ConfigDigest != accepted.ConfigDigest || !reflect.DeepEqual(recovered.Policy, accepted.Policy) || !reflect.DeepEqual(recovered.Gates, accepted.Gates) {
		t.Fatalf("malformed candidate displaced accepted control config: before=%+v after=%+v err=%v", accepted, recovered, err)
	}
}

func TestAcceptedControlConfigRecoversAnExactHistoricalEpoch(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	originalGate := passingGitGate()
	bootstrapProjectWithGate(t, ctx, service, repo, originalGate)

	original, err := service.AcceptedControlConfigRevision(ctx, repo, "")
	if err != nil {
		t.Fatal(err)
	}
	updatedGate := originalGate
	updatedGate.Description = "A later accepted description that must not overwrite history"
	writeGates(t, repo, GatesConfig{SchemaVersion: SchemaVersion, Gates: []GateConfig{updatedGate}})
	candidate, err := LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if candidate.Digest == original.ConfigDigest {
		t.Fatal("updated control config did not create a new digest")
	}
	if _, err := acceptTestPolicy(t, ctx, service, repo, candidate.Digest, "owner", "accept a second control epoch"); err != nil {
		t.Fatal(err)
	}

	latest, err := service.AcceptedControlConfigRevision(ctx, repo, "")
	if err != nil || latest.ConfigDigest != candidate.Digest || latest.Gates.Gates[0].Description != updatedGate.Description {
		t.Fatalf("latest accepted control epoch was not recoverable: %+v err=%v", latest, err)
	}
	historical, err := service.AcceptedControlConfigRevision(ctx, repo, original.ConfigDigest)
	if err != nil || historical.ConfigDigest != original.ConfigDigest || !reflect.DeepEqual(historical.Project, original.Project) || !reflect.DeepEqual(historical.Policy, original.Policy) || !reflect.DeepEqual(historical.Gates, original.Gates) {
		t.Fatalf("exact historical control epoch was displaced by a later acceptance: before=%+v recovered=%+v err=%v", original, historical, err)
	}
	if _, err := service.AcceptedControlConfigRevision(ctx, repo, digestBytes([]byte("never accepted"))); err == nil || !isErrorCode(err, "CONFIG_NOT_ACCEPTED") {
		t.Fatalf("an unaccepted control digest was returned as authority: %v", err)
	}
}

func TestConfigDriftCannotCreateVacuousPass(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Change source", Goal: "Exercise config drift", Scope: []string{"src"},
		AcceptanceCriteria: []string{"source check passes"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "change\n")
	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	before, err := service.Verdict(ctx, repo)
	if err != nil || before.Status != VerdictPass {
		t.Fatalf("precondition PASS failed: %+v %v", before, err)
	}

	writeGates(t, repo, GatesConfig{SchemaVersion: SchemaVersion, Gates: []GateConfig{}})
	after, err := service.Verdict(ctx, repo)
	if err != nil {
		t.Fatalf("verdict after drift: %v", err)
	}
	if after.Status != VerdictBlocked || !hasReason(after, "CONFIG_NOT_ACCEPTED") || !hasReason(after, "GATE_EVIDENCE_STALE") {
		t.Fatalf("config weakening did not fail closed: %+v", after)
	}
	driftConfig, err := LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acceptTestPolicy(t, ctx, service, repo, driftConfig.Digest, "agent", "weaken policy during change"); err == nil || !isErrorCode(err, "ACTIVE_CHANGE_POLICY_DRIFT") {
		t.Fatalf("expected policy acceptance to be blocked during active Change, got %v", err)
	}
}

func TestOutOfScopePathBlocksVerdict(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Scoped source change", Goal: "Only src changes", Scope: []string{"src"},
		AcceptanceCriteria: []string{"source check passes"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "outside.md"), []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	verdict, err := service.Verdict(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Status != VerdictBlocked || !hasReasonPath(verdict, "OUT_OF_SCOPE", "docs/outside.md") {
		t.Fatalf("expected out-of-scope block: %+v", verdict)
	}
}

func TestListEvidenceReturnsNonNilEmptyCollection(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Empty Evidence history", Goal: "Expose an empty collection", Scope: []string{"src"},
		AcceptanceCriteria: []string{"Evidence list is an array"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}

	report, err := service.ListEvidence(ctx, repo, change.ChangeID)
	if err != nil {
		t.Fatalf("list empty Evidence: %v", err)
	}
	if report.Evidence == nil || len(report.Evidence) != 0 {
		t.Fatalf("empty Evidence history must be a non-nil empty collection: %#v", report.Evidence)
	}
}

func TestHighRiskAcknowledgementBindsExactSubject(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "High risk source change", Goal: "Require acknowledgement", Scope: []string{"src"},
		AcceptanceCriteria: []string{"source check passes"}, Risk: RiskHigh,
	})
	if err != nil {
		t.Fatal(err)
	}
	preGate, err := service.Verdict(ctx, repo)
	if err != nil || preGate.Status != VerdictBlocked || !hasReason(preGate, "ACKNOWLEDGEMENT_REQUIRED") || !hasReason(preGate, "GATE_EVIDENCE_MISSING") {
		t.Fatalf("expected pre-Gate non-acknowledgeable Verdict: %+v %v", preGate, err)
	}
	_, beforeAcknowledgement, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	beforeRevision := beforeAcknowledgement.Projection.Revision
	beforeEventHead := beforeAcknowledgement.Projection.EventHead
	beforeCount := len(beforeAcknowledgement.Projection.Acknowledgements[change.ChangeID])
	if _, err := service.RecordAcknowledgement(ctx, repo, change.ChangeID, preGate.SubjectDigest, "owner", "premature acknowledgement"); err == nil || !isErrorCode(err, "SUBJECT_NOT_ACKNOWLEDGEABLE") {
		t.Fatalf("expected pre-Gate acknowledgement to be rejected, got %v", err)
	}
	_, afterAcknowledgement, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if afterAcknowledgement.Projection.Revision != beforeRevision || afterAcknowledgement.Projection.EventHead != beforeEventHead || len(afterAcknowledgement.Projection.Acknowledgements[change.ChangeID]) != beforeCount {
		t.Fatalf("rejected pre-Gate acknowledgement wrote authority state: before_revision=%d after_revision=%d before_head=%q after_head=%q before_count=%d after_count=%d",
			beforeRevision, afterAcknowledgement.Projection.Revision, beforeEventHead, afterAcknowledgement.Projection.EventHead,
			beforeCount, len(afterAcknowledgement.Projection.Acknowledgements[change.ChangeID]))
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "high risk\n")
	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	blocked, err := service.Verdict(ctx, repo)
	if err != nil || blocked.Status != VerdictBlocked || !hasReason(blocked, "ACKNOWLEDGEMENT_REQUIRED") {
		t.Fatalf("expected acknowledgement block: %+v %v", blocked, err)
	}
	if _, err := service.RecordAcknowledgement(ctx, repo, change.ChangeID, digestBytes([]byte("stale acknowledgement subject")), "owner", "stale subject"); err == nil || !isErrorCode(err, "SUBJECT_DIGEST_MISMATCH") {
		t.Fatalf("acknowledgement accepted a stale subject digest: %v", err)
	}
	ack, err := service.RecordAcknowledgement(ctx, repo, change.ChangeID, blocked.SubjectDigest, "owner", "reviewed exact local subject")
	if err != nil {
		t.Fatalf("record acknowledgement: %v", err)
	}
	passed, err := service.Verdict(ctx, repo)
	if err != nil || passed.Status != VerdictPass || passed.Acknowledgement != ack.ID {
		t.Fatalf("expected acknowledged PASS: %+v %v", passed, err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "invalidate acknowledgement\n")
	changed, err := service.Verdict(ctx, repo)
	if err != nil || changed.Status != VerdictBlocked || changed.Acknowledgement != "" {
		t.Fatalf("acknowledgement applied to a changed subject: %+v %v", changed, err)
	}
}

func TestEvidenceArtifactCorruptionIsIndeterminate(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Corruption check", Goal: "Detect artifact tampering", Scope: []string{"src"},
		AcceptanceCriteria: []string{"source check passes"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "artifact test\n")
	run, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil)
	if err != nil {
		t.Fatal(err)
	}
	config, err := LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := service.Git.WorkspaceID(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewStore(service.StateDir, config.Project.ProjectID, workspaceID, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(store.Directory(), filepath.FromSlash(run.Evidence[0].StdoutArtifact))
	if _, err := os.Lstat(artifact); err != nil {
		var found []string
		_ = filepath.Walk(store.Directory(), func(path string, _ os.FileInfo, walkErr error) error {
			if walkErr == nil {
				found = append(found, path)
			}
			return nil
		})
		t.Fatalf("expected artifact %s is missing: %v; store contents=%v; evidence=%+v", artifact, err, found, run.Evidence[0])
	}
	if err := os.WriteFile(artifact, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	verdict, err := service.Verdict(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Status != VerdictIndeterminate || !hasReason(verdict, "EVIDENCE_ARTIFACT_INVALID") {
		t.Fatalf("artifact corruption was not indeterminate: %+v", verdict)
	}
}

func TestGateMutationCannotPass(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	mutating := GateConfig{
		ID: "mutating-gate", Description: "Mutates source while pretending to validate",
		Command: []string{"sh", "-c", "printf mutation >> src/app.txt"}, WorkingDirectory: ".",
		TimeoutSeconds: 10, AllowedExitCodes: []int{0},
		RequiredFor: []Risk{RiskLow, RiskModerate, RiskHigh, RiskCritical},
		Environment: map[string]string{}, InheritEnvironment: []string{}, MaxOutputBytes: 4096,
	}
	bootstrapProjectWithGate(t, ctx, service, repo, mutating)
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Mutation detection", Goal: "Gate must not mutate", Scope: []string{"src"},
		AcceptanceCriteria: []string{"mutation is detected"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Evidence) != 1 || !run.Evidence[0].SourceMutated || run.Evidence[0].Passed() {
		t.Fatalf("mutating Gate incorrectly passed: %+v", run)
	}
	verdict, err := service.Verdict(ctx, repo)
	if err != nil || verdict.Status == VerdictPass {
		t.Fatalf("mutating Gate yielded PASS: %+v %v", verdict, err)
	}
}

func TestStrictConfigRejectsDuplicateAndUnknownFields(t *testing.T) {
	var target struct {
		Value int `json:"value"`
	}
	if err := decodeStrictJSON([]byte(`{"value":1,"value":2}`), &target); err == nil {
		t.Fatal("duplicate JSON key was accepted")
	}
	if err := decodeStrictJSON([]byte(`{"value":1,"unknown":2}`), &target); err == nil {
		t.Fatal("unknown JSON field was accepted")
	}
	if err := decodeStrictJSON([]byte(`{"value":1} {"value":2}`), &target); err == nil {
		t.Fatal("trailing JSON value was accepted")
	}
	if err := decodeStrictJSON([]byte(`{"Value":1}`), &target); err == nil {
		t.Fatal("case-insensitive JSON field alias was accepted")
	}
	if err := decodeStrictJSON([]byte(`{"value":1,"VALUE":2}`), &target); err == nil {
		t.Fatal("case-insensitive duplicate field alias was accepted")
	}
	var policy PolicyConfig
	if err := decodeStrictJSON([]byte(`{"schema_version":1,"Denied_Path_Roots":[".ecp"]}`), &policy); err == nil {
		t.Fatal("policy field with inexact casing was accepted")
	}
	var gates GatesConfig
	if err := decodeStrictJSON([]byte(`{"schema_version":1,"gates":[{"Command":["/usr/bin/true"]}]}`), &gates); err == nil {
		t.Fatal("nested Gate field with inexact casing was accepted")
	}
	var dynamicMap struct {
		Environment map[string]string `json:"environment"`
	}
	if err := decodeStrictJSON([]byte(`{"environment":{"Mixed_Case_Key":"value"}}`), &dynamicMap); err != nil {
		t.Fatalf("dynamic map key was incorrectly treated as a schema field: %v", err)
	}
}

func TestConfigFileSizeLimitFailsClosed(t *testing.T) {
	repo := createTestRepository(t)
	service := newTestService(t)
	if _, err := service.InitProject(context.Background(), repo, "bounded-config"); err != nil {
		t.Fatal(err)
	}
	oversized := bytes.Repeat([]byte{'x'}, maxConfigFileBytes+1)
	if err := os.WriteFile(filepath.Join(repo, ".ecp", "gates.json"), oversized, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(repo); err == nil || !isErrorCode(err, "CONFIG_FILE_TOO_LARGE") {
		t.Fatalf("oversized config did not fail closed: %v", err)
	}
}

func TestSnapshotDetectsCommitAndModeChanges(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	git := Git{}
	policy := PolicyConfig{MaxSourceFiles: 1000, MaxSourceBytes: 10 << 20}
	baseline, err := git.Snapshot(ctx, repo, policy)
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "committed change\n")
	runGit(t, repo, "add", "src/app.txt")
	runGit(t, repo, "commit", "-m", "change app")
	committed, err := git.Snapshot(ctx, repo, policy)
	if err != nil {
		t.Fatal(err)
	}
	if committed.Fingerprint == baseline.Fingerprint || !containsString(TouchedPaths(baseline, committed), "src/app.txt") {
		t.Fatalf("committed change disappeared from baseline delta")
	}
	if err := os.Chmod(filepath.Join(repo, "src", "app.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	modeChanged, err := git.Snapshot(ctx, repo, policy)
	if err != nil {
		t.Fatal(err)
	}
	if modeChanged.Fingerprint == committed.Fingerprint {
		t.Fatal("mode change did not change source fingerprint")
	}
}

func createTestRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "-b", "main")
	runGit(t, repo, "config", "user.name", "ECP Test")
	runGit(t, repo, "config", "user.email", "ecp-test@example.invalid")
	if err := os.MkdirAll(filepath.Join(repo, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "src", "app.txt"), []byte("baseline\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".ecp-state/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "baseline")
	return repo
}

func newTestService(t *testing.T) Service {
	t.Helper()
	return Service{StateDir: filepath.Join(t.TempDir(), "state")}
}

func startTestChange(t *testing.T, ctx context.Context, service Service, repo string, input StartChangeInput) (Change, error) {
	t.Helper()
	current, err := service.Context(ctx, repo)
	if err != nil {
		return Change{}, err
	}
	input.ExpectedWorkspace = current.WorkspaceID
	input.ExpectedAuthority = current.AuthorityID
	input.ExpectedActivation = current.ActivationToken
	input.ExpectedConfig = current.CandidateConfig
	input.ExpectedTruth = current.AcceptedTruth
	input.ExpectedSource = current.Source.Fingerprint
	if !input.Impact.ProjectPurpose && len(input.Impact.CapabilityIDs)+len(input.Impact.InvariantIDs)+len(input.Impact.ComponentIDs)+len(input.Impact.DecisionIDs)+len(input.Impact.ContractIDs)+len(input.Impact.UnknownIDs)+len(input.Impact.UserJourneys)+len(input.Impact.DataEffects)+len(input.Impact.OperationalEffects)+len(input.Impact.ExpectedChanges)+len(input.Impact.ExpectedPreservations)+len(input.Impact.Unknowns) == 0 {
		input.Impact.Unknowns = []string{"test impact intentionally not modeled"}
	}
	if len(input.Requirements) == 0 {
		input.Requirements = []ChangeRequirement{{
			ID:             "test-contract",
			Statement:      "Exercise the exact test Change contract.",
			Status:         RequirementDecided,
			Verification:   RequirementVerificationReview,
			Rationale:      "The test supplies its own focused assertions.",
			DecisionSource: "test fixture",
			Covers: RequirementCoverage{
				AcceptanceCriteria:    append([]string(nil), input.AcceptanceCriteria...),
				UserJourneys:          append([]string(nil), input.Impact.UserJourneys...),
				DataEffects:           append([]string(nil), input.Impact.DataEffects...),
				OperationalEffects:    append([]string(nil), input.Impact.OperationalEffects...),
				ExpectedChanges:       append([]string(nil), input.Impact.ExpectedChanges...),
				ExpectedPreservations: append([]string(nil), input.Impact.ExpectedPreservations...),
				Unknowns:              append([]string(nil), input.Impact.Unknowns...),
			},
		}}
	}
	return service.StartChange(ctx, repo, input)
}

func assessTestSemantic(t *testing.T, ctx context.Context, service Service, repo string, input SemanticAssessmentInput) (SemanticAssessment, error) {
	t.Helper()
	if len(input.RequirementAssessments) == 0 {
		history, err := service.ListChanges(ctx, repo)
		if err != nil {
			return SemanticAssessment{}, err
		}
		for index := len(history) - 1; index >= 0; index-- {
			if history[index].State != ChangeActive {
				continue
			}
			for _, requirement := range history[index].Requirements {
				assessment := RequirementAssessment{RequirementID: requirement.ID, Summary: "Focused test review reconciled this requirement."}
				switch {
				case requirement.Status == RequirementNotApplicable:
					assessment.Outcome = RequirementOutcomeNotApplicable
				case requirement.Status == RequirementDeferredSafe:
					assessment.Outcome = RequirementOutcomeDeferredSafe
				case requirement.Verification == RequirementVerificationExternal:
					assessment.Outcome = RequirementOutcomeExternalPending
				case requirement.Verification == RequirementVerificationAutomated:
					assessment.Outcome = RequirementOutcomeVerified
					assessment.EvidenceGateIDs = append([]string(nil), requirement.RequiredGateIDs...)
				default:
					assessment.Outcome = RequirementOutcomeVerified
				}
				input.RequirementAssessments = append(input.RequirementAssessments, assessment)
			}
			break
		}
	}
	return service.AssessSemantic(ctx, repo, input)
}

func TestSemanticCategoryContractRejectsUnversionedValues(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Reject arbitrary semantic category", Goal: "Keep semantic history comparable", Scope: []string{"src"},
		AcceptanceCriteria: []string{"unversioned semantic categories are rejected"}, Risk: RiskModerate,
		Impact: ChangeImpact{ExpectedPreservations: []string{"Current user-visible behavior remains unchanged"}},
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
	_, err = assessTestSemantic(t, ctx, service, repo, SemanticAssessmentInput{
		ExpectedAuthority:      current.AuthorityID,
		ExpectedWorkspace:      current.WorkspaceID,
		ExpectedActivation:     current.ActivationToken,
		ExpectedChangeID:       change.ChangeID,
		ExpectedSource:         current.Source.Fingerprint,
		ExpectedPreviousTruth:  diff.PreviousTruthDigest,
		ExpectedCandidateTruth: diff.CandidateTruthDigest,
		Behavior:               SemanticBehaviorPreserved,
		Summary:                "The product behavior remains unchanged.",
		Categories:             []string{"made-up-category"},
		Actor:                  "reviewer",
		Reason:                 "exercise the versioned semantic category contract",
	})
	if err == nil || !isErrorCode(err, "SEMANTIC_CATEGORIES_INVALID") {
		t.Fatalf("arbitrary semantic category was accepted: %v", err)
	}
}

func TestSemanticChangeWithoutProjectTruthDeltaCanPassButCannotBeMarkedPreserved(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Change behavior within accepted capability", Goal: "Record a semantic change without duplicating implementation detail in Project Truth", Scope: []string{"src"},
		AcceptanceCriteria: []string{"semantic change is current and evidenced"}, Risk: RiskModerate,
		Impact: ChangeImpact{
			ExpectedChanges:       []string{"The test product exposes the requested updated behavior"},
			ExpectedPreservations: []string{"The accepted durable Project Truth remains accurate"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "new behavior within the existing durable contract\n")
	current, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := service.TruthDiff(ctx, repo)
	if err != nil || diff.Changed {
		t.Fatalf("unexpected Project Truth delta: %+v err=%v", diff, err)
	}
	input := SemanticAssessmentInput{
		ExpectedAuthority:      current.AuthorityID,
		ExpectedWorkspace:      current.WorkspaceID,
		ExpectedActivation:     current.ActivationToken,
		ExpectedChangeID:       change.ChangeID,
		ExpectedSource:         current.Source.Fingerprint,
		ExpectedPreviousTruth:  diff.PreviousTruthDigest,
		ExpectedCandidateTruth: diff.CandidateTruthDigest,
		Behavior:               SemanticBehaviorPreserved,
		Summary:                "This incorrectly claims the expected behavior change was preserved.",
		Categories:             []string{"behavior"},
		Actor:                  "reviewer",
		Reason:                 "exercise expected-change enforcement",
	}
	if _, err := assessTestSemantic(t, ctx, service, repo, input); err == nil || !isErrorCode(err, "EXPECTED_SEMANTIC_CHANGE_NOT_RECONCILED") {
		t.Fatalf("expected semantic change was incorrectly reconciled as PRESERVED: %v", err)
	}
	input.Behavior = SemanticBehaviorChanged
	input.Summary = "The requested behavior changed while the accepted durable Project Truth remains accurate."
	assessment, err := assessTestSemantic(t, ctx, service, repo, input)
	if err != nil || assessment.Behavior != SemanticBehaviorChanged || assessment.CurrentTruthDigest != assessment.PreviousTruthDigest || len(assessment.TruthDelta) != 0 {
		t.Fatalf("semantic change without durable truth delta was rejected or distorted: %+v err=%v", assessment, err)
	}
	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	verdict, err := service.Verdict(ctx, repo)
	if err != nil || verdict.Status != VerdictPass || verdict.SemanticAssessmentID != assessment.AssessmentID {
		t.Fatalf("evidenced semantic change without truth delta did not PASS: %+v err=%v", verdict, err)
	}
}

func runTestGates(t *testing.T, ctx context.Context, service Service, repo, changeID string, selected []string) (GateRunResult, error) {
	t.Helper()
	current, err := service.Context(ctx, repo)
	if err != nil {
		return GateRunResult{}, err
	}
	verdict, err := service.Verdict(ctx, repo)
	if err != nil {
		return GateRunResult{}, err
	}
	if verdict.SemanticAssessmentID == "" {
		diff, err := service.TruthDiff(ctx, repo)
		if err != nil {
			return GateRunResult{}, err
		}
		if diff.Changed {
			return GateRunResult{}, newError(KindBlocked, "TEST_TRUTH_RECONCILIATION_REQUIRED", "test helper will not implicitly accept changed Project Truth", nil)
		}
		if _, err := assessTestSemantic(t, ctx, service, repo, SemanticAssessmentInput{
			ExpectedAuthority:      current.AuthorityID,
			ExpectedWorkspace:      current.WorkspaceID,
			ExpectedActivation:     current.ActivationToken,
			ExpectedChangeID:       changeID,
			ExpectedSource:         current.Source.Fingerprint,
			ExpectedPreviousTruth:  diff.PreviousTruthDigest,
			ExpectedCandidateTruth: diff.CandidateTruthDigest,
			Behavior:               SemanticBehaviorPreserved,
			Summary:                "Test confirms the accepted Project Truth remains valid for this source.",
			Categories:             []string{"behavior"},
			Actor:                  "test",
			Reason:                 "record semantic reconciliation before Gate Evidence",
		}); err != nil {
			return GateRunResult{}, err
		}
	}
	plan, err := service.PlanGates(ctx, repo)
	if err != nil {
		return GateRunResult{}, err
	}
	if plan.ChangeID != changeID {
		t.Fatalf("Gate plan targeted %q, expected %q", plan.ChangeID, changeID)
	}
	return service.RunGates(ctx, repo, changeID, plan.PlanDigest, selected)
}

func bootstrapProjectWithGate(t *testing.T, ctx context.Context, service Service, repo string, gates ...GateConfig) {
	t.Helper()
	if _, err := service.InitProject(ctx, repo, "test-project"); err != nil {
		t.Fatalf("init project: %v", err)
	}
	writeGates(t, repo, GatesConfig{SchemaVersion: SchemaVersion, Gates: gates})
	config, err := LoadConfig(repo)
	if err != nil {
		t.Fatalf("load candidate config: %v", err)
	}
	if _, err := acceptTestPolicy(t, ctx, service, repo, config.Digest, "test-owner", "accept test Gate definition"); err != nil {
		t.Fatalf("accept policy: %v", err)
	}
	if _, err := enableTestProject(t, ctx, service, repo, config.Digest, "test-owner", "enable ECP for test Workspace"); err != nil {
		t.Fatalf("enable project: %v", err)
	}
}

func authorityTargetForTest(t *testing.T, ctx context.Context, service Service, repo string) (string, string) {
	t.Helper()
	resolved, err := service.withDefaults()
	if err != nil {
		t.Fatal(err)
	}
	workspaceID, err := resolved.Git.WorkspaceID(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	return resolved.authorityID, workspaceID
}

func acceptTestPolicy(t *testing.T, ctx context.Context, service Service, repo, configDigest, actor, reason string) (ConfigAcceptance, error) {
	t.Helper()
	authorityID, workspaceID := authorityTargetForTest(t, ctx, service, repo)
	return service.AcceptPolicy(ctx, repo, authorityID, workspaceID, configDigest, actor, reason)
}

func enableTestProject(t *testing.T, ctx context.Context, service Service, repo, configDigest, actor, reason string) (ProjectStatus, error) {
	t.Helper()
	status, err := service.ProjectStatus(ctx, repo)
	if err != nil {
		return ProjectStatus{}, err
	}
	return service.EnableProject(ctx, repo, status.AuthorityID, status.WorkspaceID, status.ActivationToken, configDigest, status.CandidateTruthDigest, actor, reason)
}

func registerTestProject(t *testing.T, ctx context.Context, service Service, repo, configDigest, actor, reason string) (ProjectContext, error) {
	t.Helper()
	authorityID, workspaceID := authorityTargetForTest(t, ctx, service, repo)
	config, err := LoadConfig(repo)
	if err != nil {
		return ProjectContext{}, err
	}
	return service.RegisterProject(ctx, repo, authorityID, workspaceID, configDigest, config.TruthDigest, actor, reason)
}

func cancelTestChange(t *testing.T, ctx context.Context, service Service, repo, changeID, actor, reason string) (ChangeCancellation, error) {
	t.Helper()
	authorityID, workspaceID := authorityTargetForTest(t, ctx, service, repo)
	return service.CancelChange(ctx, repo, authorityID, workspaceID, changeID, actor, reason)
}

func passingGitGate() GateConfig {
	return GateConfig{
		ID: "source-check", Description: "Check Git diff whitespace errors",
		Command: []string{"git", "diff", "--check"}, WorkingDirectory: ".",
		TimeoutSeconds: 10, AllowedExitCodes: []int{0},
		RequiredFor: []Risk{RiskLow, RiskModerate, RiskHigh, RiskCritical},
		Environment: map[string]string{}, InheritEnvironment: []string{}, MaxOutputBytes: 4096,
	}
}

func writeGates(t *testing.T, repo string, gates GatesConfig) {
	t.Helper()
	b, err := json.MarshalIndent(gates, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(filepath.Join(repo, ".ecp", "gates.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output.String())
	}
}

func appendFile(t *testing.T, path, content string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(content); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func hasReason(verdict Verdict, code string) bool {
	for _, reason := range verdict.Reasons {
		if reason.Code == code {
			return true
		}
	}
	return false
}

func hasReasonGate(verdict Verdict, code, gateID string) bool {
	for _, reason := range verdict.Reasons {
		if reason.Code == code && reason.GateID == gateID {
			return true
		}
	}
	return false
}

func hasReasonPath(verdict Verdict, code, path string) bool {
	for _, reason := range verdict.Reasons {
		if reason.Code == code && reason.Path == path {
			return true
		}
	}
	return false
}

func isErrorCode(err error, code string) bool {
	var typed *ECPError
	return errors.As(err, &typed) && typed.Code == code
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
