package ecp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEstablishedTruthCanGovernADeclaredNewComponentPath(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	establishTruthForNewPathTest(t, ctx, service, repo)

	baseInput := StartChangeInput{
		Title: "Add a governed component", Goal: "Create a new source root and make its ownership durable", Scope: []string{"plugins/new-widget"},
		AcceptanceCriteria: []string{"the new component path is owned by accepted Project Truth"}, Risk: RiskModerate,
	}
	if _, err := startTestChange(t, ctx, service, repo, baseInput); err == nil || !isErrorCode(err, "DECLARED_IMPACT_INCOMPLETE") {
		t.Fatalf("established truth accepted an undeclared unmapped scope: %v", err)
	}

	baseInput.Impact = ChangeImpact{NewPathRoots: []string{"plugins/new-widget"}}
	change, err := startTestChange(t, ctx, service, repo, baseInput)
	if err != nil {
		t.Fatalf("explicit new_path_root did not open a bounded Change: %v", err)
	}
	if change.ContractVersion != ChangeContractVersionNewPathRoots {
		t.Fatalf("new Change used contract_version=%d want=%d", change.ContractVersion, ChangeContractVersionNewPathRoots)
	}

	newRoot := filepath.Join(repo, "plugins", "new-widget")
	if err := os.MkdirAll(newRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(newRoot, "main.txt"), []byte("new governed component\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	candidate, err := LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	candidate.Truth.Components = append(candidate.Truth.Components, TruthComponent{
		ID: "new-widget", Name: "New widget", Responsibility: "Own the newly introduced widget source.", PathRoots: []string{"plugins/new-widget"}, DependsOn: []string{},
	})
	if err := writePrettyJSON(filepath.Join(repo, ".ecp", "truth.json"), candidate.Truth, 0o644); err != nil {
		t.Fatal(err)
	}

	diff, err := service.TruthDiff(ctx, repo)
	if err != nil || !diff.Changed {
		t.Fatalf("new component truth delta was not detected: %+v err=%v", diff, err)
	}
	current, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := assessTestSemantic(t, ctx, service, repo, SemanticAssessmentInput{
		ExpectedAuthority: current.AuthorityID, ExpectedWorkspace: current.WorkspaceID, ExpectedActivation: current.ActivationToken,
		ExpectedChangeID: change.ChangeID, ExpectedSource: current.Source.Fingerprint, ExpectedPreviousTruth: diff.PreviousTruthDigest,
		ExpectedCandidateTruth: diff.CandidateTruthDigest, Behavior: SemanticBehaviorChanged,
		Summary: "The declared source root is now owned by one exact Project Truth component.", Categories: []string{"architecture", "project-truth"},
		Actor: "owner", Reason: "confirm the bounded new component ownership", ConfirmProtected: true,
	})
	if err != nil {
		t.Fatalf("declared new component could not be reconciled: %v", err)
	}
	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	verdict, err := service.Verdict(ctx, repo)
	if err != nil || verdict.Status != VerdictPass || verdict.SemanticAssessmentID != assessment.AssessmentID || len(verdict.InferredImpact.UnmappedPaths) != 0 {
		t.Fatalf("reconciled new component did not reach mapped PASS: %+v err=%v", verdict, err)
	}
	if _, err := service.CompleteChange(ctx, repo, change.ChangeID, verdict.SubjectDigest); err != nil {
		t.Fatalf("reconciled new component could not complete: %v", err)
	}
}

func TestNewPathRootMustBeCanonicalUnownedCoveredAndReconciled(t *testing.T) {
	truth := establishedTruthForNewPathTest()
	for _, test := range []struct {
		name  string
		roots []string
		code  string
	}{
		{name: "already owned", roots: []string{"src/new"}, code: "NEW_PATH_ROOT_ALREADY_OWNED"},
		{name: "overlapping", roots: []string{"plugins", "plugins/new"}, code: "OVERLAPPING_NEW_PATH_ROOTS"},
		{name: "control path", roots: []string{".ecp/generated"}, code: "INVALID_NEW_PATH_ROOTS"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateChangeImpact(ChangeImpact{NewPathRoots: test.roots}, truth); err == nil || !isErrorCode(err, test.code) {
				t.Fatalf("invalid new_path_roots were not rejected with %s: %v", test.code, err)
			}
		})
	}
	if err := validateNewPathRootScope([]string{"plugins/new"}, []string{"src"}); err == nil || !isErrorCode(err, "NEW_PATH_ROOT_OUTSIDE_SCOPE") {
		t.Fatalf("out-of-scope new_path_root was accepted: %v", err)
	}
	if err := validateNewPathRootTruthOutcome([]string{"plugins/new"}, truth, false); err == nil || !isErrorCode(err, "NEW_PATH_ROOT_NOT_RECONCILED") {
		t.Fatalf("unreconciled new_path_root was accepted: %v", err)
	}
	candidate := truth
	candidate.Components = append(candidate.Components, TruthComponent{ID: "new-component", Name: "New component", Responsibility: "Own the new path.", PathRoots: []string{"plugins/new"}, DependsOn: []string{}})
	if err := validateNewPathRootTruthOutcome([]string{"plugins/new"}, candidate, true); err != nil {
		t.Fatalf("exactly owned new_path_root was rejected: %v", err)
	}
	unbounded := candidate
	unbounded.Components = append([]TruthComponent(nil), candidate.Components...)
	unbounded.Components[len(unbounded.Components)-1].PathRoots = []string{"other/unrelated", "plugins/new"}
	if err := validateTruthDeltaAgainstImpact(
		[]TruthDelta{{Section: "component", ID: "new-component", Operation: "added", Protected: true}},
		ChangeImpact{NewPathRoots: []string{"plugins/new"}}, truth, unbounded,
	); err == nil || !isErrorCode(err, "TRUTH_DELTA_OUTSIDE_CHANGE_IMPACT") {
		t.Fatalf("new_path_root authorized an added component with undeclared ownership: %v", err)
	}
	if err := validateNewPathRootTouched([]string{"plugins/new"}, []string{"plugins/new/main.go"}); err != nil {
		t.Fatalf("touched new_path_root was rejected: %v", err)
	}
}

func establishTruthForNewPathTest(t *testing.T, ctx context.Context, service Service, repo string) {
	t.Helper()
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Establish path ownership", Goal: "Create an established Project Truth baseline", Scope: []string{"src"},
		AcceptanceCriteria: []string{"Project Truth has established component ownership"}, Risk: RiskModerate,
		Impact: ChangeImpact{
			ProjectPurpose: true, ContractIDs: []string{"project-boundaries"}, UnknownIDs: []string{"project-purpose-unreviewed"},
			UnknownDispositions: []UnknownDisposition{{UnknownID: "project-purpose-unreviewed", Outcome: UnknownDispositionResolved}},
			Unknowns:            []string{"the seed facts are being established"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	truth := establishedTruthForNewPathTest()
	if err := writePrettyJSON(filepath.Join(repo, ".ecp", "truth.json"), truth, 0o644); err != nil {
		t.Fatal(err)
	}
	diff, err := service.TruthDiff(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	current, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := assessTestSemantic(t, ctx, service, repo, SemanticAssessmentInput{
		ExpectedAuthority: current.AuthorityID, ExpectedWorkspace: current.WorkspaceID, ExpectedActivation: current.ActivationToken,
		ExpectedChangeID: change.ChangeID, ExpectedSource: current.Source.Fingerprint, ExpectedPreviousTruth: diff.PreviousTruthDigest,
		ExpectedCandidateTruth: diff.CandidateTruthDigest, Behavior: SemanticBehaviorChanged,
		Summary: "The seed is replaced by an established path ownership baseline.", Categories: []string{"project-truth", "architecture"},
		Actor: "owner", Reason: "confirm the established test truth", ConfirmProtected: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	verdict, err := service.Verdict(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if verdict.Status == VerdictBlocked && containsReason(verdict.Reasons, "ACKNOWLEDGEMENT_REQUIRED") {
		if _, err := service.RecordAcknowledgement(ctx, repo, change.ChangeID, verdict.SubjectDigest, "owner", "review the exact established truth subject"); err != nil {
			t.Fatal(err)
		}
		verdict, err = service.Verdict(ctx, repo)
	}
	if err != nil || verdict.Status != VerdictPass {
		t.Fatalf("established truth baseline did not PASS: %+v err=%v", verdict, err)
	}
	if _, err := service.CompleteChange(ctx, repo, change.ChangeID, verdict.SubjectDigest); err != nil {
		t.Fatal(err)
	}
}

func establishedTruthForNewPathTest() ProjectTruthConfig {
	return ProjectTruthConfig{
		SchemaVersion: SchemaVersion, Maturity: TruthMaturityEstablished, Purpose: "Exercise established path ownership and bounded component creation.",
		Capabilities: []TruthCapability{{ID: "core-capability", Name: "Core capability", Description: "Provide the existing core behavior.", Status: "active", ComponentIDs: []string{"core-component"}, InvariantIDs: []string{"core-integrity"}}},
		Invariants:   []TruthInvariant{{ID: "core-integrity", Name: "Core integrity", Statement: "Existing core source remains locally verifiable.", Category: "architecture", Risk: RiskLow, GateIDs: []string{"source-check"}, SourceRefs: []string{"src/app.txt"}}},
		Components:   []TruthComponent{{ID: "core-component", Name: "Core component", Responsibility: "Own the established source root.", PathRoots: []string{"src"}, DependsOn: []string{}}},
		Decisions:    []TruthDecision{}, Contracts: DefaultProjectTruth("new-path-test").Contracts, Unknowns: []TruthUnknown{},
	}
}
