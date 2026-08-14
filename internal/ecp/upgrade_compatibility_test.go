package ecp

import (
	"context"
	"path/filepath"
	"testing"
)

func TestCoreIdentityUpgradePreservesAuthorityAndRequiresFreshEvidence(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	stateDir := filepath.Join(t.TempDir(), "state")
	beforeUpgrade := Service{StateDir: stateDir, CoreIdentity: "upgrade-test-core-a"}
	bootstrapProjectWithGate(t, ctx, beforeUpgrade, repo, passingGitGate())
	change, err := startTestChange(t, ctx, beforeUpgrade, repo, StartChangeInput{
		Title: "Upgrade compatibility", Goal: "Preserve authority while invalidating evaluator-bound Evidence", Scope: []string{"src"},
		AcceptanceCriteria: []string{"history and mode survive while Evidence is rerun"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "upgrade-compatible edit\n")
	beforePlan, err := beforeUpgrade.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runTestGates(t, ctx, beforeUpgrade, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	beforeVerdict, err := beforeUpgrade.Verdict(ctx, repo)
	if err != nil || beforeVerdict.Status != VerdictPass {
		t.Fatalf("pre-upgrade subject did not reach PASS: %+v err=%v", beforeVerdict, err)
	}

	afterUpgrade := Service{StateDir: stateDir, CoreIdentity: "upgrade-test-core-b"}
	status, err := afterUpgrade.ProjectStatus(ctx, repo)
	if err != nil || !status.Enabled || !status.Registered || status.ActiveChange == nil || status.ActiveChange.ChangeID != change.ChangeID {
		t.Fatalf("upgrade lost Workspace mode or active Change: %+v err=%v", status, err)
	}
	history, err := afterUpgrade.ListChanges(ctx, repo)
	if err != nil || len(history) != 1 || history[0].ChangeID != change.ChangeID || history[0].State != ChangeActive {
		t.Fatalf("upgrade lost authority history: %+v err=%v", history, err)
	}
	stale, err := afterUpgrade.Verdict(ctx, repo)
	if err != nil || stale.Status != VerdictBlocked || !hasReason(stale, "GATE_EVIDENCE_STALE") {
		t.Fatalf("old-Core Evidence remained valid after upgrade: %+v err=%v", stale, err)
	}
	afterPlan, err := afterUpgrade.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if afterPlan.CoreIdentity != afterUpgrade.CoreIdentity || afterPlan.PlanDigest == beforePlan.PlanDigest {
		t.Fatalf("upgrade did not produce a new Core-bound Gate plan: plan=%+v verdict=%+v", afterPlan, beforeVerdict)
	}
	if _, err := afterUpgrade.RunGates(ctx, repo, change.ChangeID, afterPlan.PlanDigest, nil); err != nil {
		t.Fatal(err)
	}
	refreshed, err := afterUpgrade.Verdict(ctx, repo)
	if err != nil || refreshed.Status != VerdictPass || refreshed.EvaluatorVersion != afterUpgrade.CoreIdentity {
		t.Fatalf("fresh upgraded-Core Evidence did not restore PASS: %+v err=%v", refreshed, err)
	}
	if _, err := afterUpgrade.CompleteChange(ctx, repo, change.ChangeID, refreshed.SubjectDigest); err != nil {
		t.Fatal(err)
	}
	completedStatus, err := afterUpgrade.ProjectStatus(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := afterUpgrade.DisableProject(ctx, repo, completedStatus.AuthorityID, completedStatus.WorkspaceID, completedStatus.ActivationToken, "owner", "exercise disabled-mode upgrade compatibility"); err != nil {
		t.Fatal(err)
	}

	reopened := Service{StateDir: stateDir, CoreIdentity: "upgrade-test-core-c"}
	reopenedStatus, err := reopened.ProjectStatus(ctx, repo)
	if err != nil || reopenedStatus.Enabled || !reopenedStatus.Registered {
		t.Fatalf("later Core identity did not preserve disabled mode: %+v err=%v", reopenedStatus, err)
	}
	completed, err := reopened.GetChange(ctx, repo, change.ChangeID)
	if err != nil || completed.State != ChangeCompleted || completed.Completion == nil || completed.Completion.FinalVerdict.EvaluatorVersion != afterUpgrade.CoreIdentity {
		t.Fatalf("completed upgraded history was not durable: %+v err=%v", completed, err)
	}
}

func TestV2ChangeImpactRemainsReadableWithoutUnknownDisposition(t *testing.T) {
	truth := DefaultProjectTruth("v2-upgrade-compatibility")
	impact := ChangeImpact{UnknownIDs: []string{"project-purpose-unreviewed"}}
	if err := validateChangeImpact(impact, truth); err != nil {
		t.Fatal(err)
	}
	if err := validateUnknownDispositions(impact, truth, false); err != nil {
		t.Fatalf("legacy v2 impact was retroactively forced into the v3 disposition contract: %v", err)
	}
}
