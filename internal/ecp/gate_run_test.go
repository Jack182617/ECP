package ecp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestGateRunLifecycleCompletesAndBindsEvidence(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Durable GateRun", Goal: "Bind Evidence to a completed execution lifecycle", Scope: []string{"src"},
		AcceptanceCriteria: []string{"GateRun history is complete"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "durable GateRun\n")
	result, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.RunID == "" || result.State != GateRunCompleted || len(result.Evidence) != 1 || result.Evidence[0].GateRunID != result.RunID {
		t.Fatalf("GateRun result did not bind its Evidence: %+v", result)
	}
	report, err := service.ListGateRuns(ctx, repo, change.ChangeID)
	if err != nil || len(report.Runs) != 1 {
		t.Fatalf("GateRun history was not queryable: %+v err=%v", report, err)
	}
	run := report.Runs[0]
	if run.RunID != result.RunID || run.State != GateRunCompleted || run.OutcomeCode != "SEQUENCE_COMPLETED" || len(run.EvidenceIDs) != 1 || run.EvidenceIDs[0] != result.Evidence[0].EvidenceID || run.FinishedAt.IsZero() {
		t.Fatalf("completed GateRun history is incomplete: %+v", run)
	}
}

func TestGateRunPartialFailureIsTerminalAndPreservesEvidence(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	mutateConfig := passingGitGate()
	mutateConfig.ID = "a-mutate-config"
	mutateConfig.Description = "Change candidate config after one Evidence record"
	mutateConfig.Command = []string{"sh", "-c", "printf ' ' >> .ecp/project.json"}
	second := passingGitGate()
	second.ID = "z-must-not-run"
	bootstrapProjectWithGates(t, ctx, service, repo, []GateConfig{mutateConfig, second})
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Partial GateRun", Goal: "Retain partial Evidence and close the failed run", Scope: []string{"src"},
		AcceptanceCriteria: []string{"partial Evidence remains auditable"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "partial GateRun\n")
	if err := assessPreservedTestChange(t, ctx, service, repo, change.ChangeID); err != nil {
		t.Fatal(err)
	}
	plan, err := service.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.RunGates(ctx, repo, change.ChangeID, plan.PlanDigest, nil)
	if err == nil || !isErrorCode(err, "CONFIG_CHANGED_DURING_GATE_SEQUENCE") || len(result.Evidence) != 1 || result.State != GateRunFailed {
		t.Fatalf("partial GateRun did not close as FAILED with Evidence: result=%+v err=%v", result, err)
	}
	report, historyErr := service.ListGateRuns(ctx, repo, change.ChangeID)
	if historyErr != nil || len(report.Runs) != 1 || report.Runs[0].State != GateRunFailed || report.Runs[0].OutcomeCode != "CONFIG_CHANGED_DURING_GATE_SEQUENCE" || len(report.Runs[0].EvidenceIDs) != 1 {
		t.Fatalf("failed GateRun history lost partial Evidence: %+v err=%v", report, historyErr)
	}
}

func TestReleasedGateRunIsRecoveredAsInterruptedBeforeNextRun(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Recover GateRun", Goal: "Record an abandoned execution before starting another", Scope: []string{"src"},
		AcceptanceCriteria: []string{"abandoned run becomes INTERRUPTED"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "recover GateRun\n")
	if err := assessPreservedTestChange(t, ctx, service, repo, change.ChangeID); err != nil {
		t.Fatal(err)
	}
	plan, err := service.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	openRun := appendOpenGateRun(t, ctx, service, repo, change, plan)
	status, err := service.ProjectStatus(ctx, repo)
	if err != nil || status.ActiveGateRun == nil || status.ActiveGateRun.RunID != openRun.RunID || status.Assurance != ProjectAssuranceIndeterminate {
		t.Fatalf("project status did not expose the unresolved GateRun: %+v err=%v", status, err)
	}
	current, err := service.Context(ctx, repo)
	if err != nil || current.ActiveGateRun == nil || current.ActiveGateRun.RunID != openRun.RunID {
		t.Fatalf("project context did not expose the unresolved GateRun: %+v err=%v", current, err)
	}

	verdict, err := service.Verdict(ctx, repo)
	if err != nil || verdict.Status != VerdictIndeterminate || !hasReason(verdict, "GATE_RUN_IN_PROGRESS_OR_INTERRUPTED") {
		t.Fatalf("open GateRun did not make Verdict indeterminate: %+v err=%v", verdict, err)
	}
	_, workspace, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	revision := workspace.Projection.Revision
	_, err = workspace.Store.Append(ctx, &revision, PendingEvent{Type: "change_cancelled", Origin: "test", Payload: ChangeCancellation{
		SchemaVersion: SchemaVersion, ChangeID: change.ChangeID, ActivationID: change.ActivationID,
		Actor: "test", Reason: "must not bypass active GateRun", Trust: "test", CancelledAt: time.Now().UTC(),
	}})
	if err == nil || !isErrorCode(err, "EVENT_DURING_ACTIVE_GATE_RUN") {
		t.Fatalf("terminal Change event bypassed the active GateRun: %v", err)
	}

	result, err := service.RunGates(ctx, repo, change.ChangeID, plan.PlanDigest, nil)
	if err != nil || result.State != GateRunCompleted {
		t.Fatalf("new GateRun could not recover and continue: %+v err=%v", result, err)
	}
	report, err := service.ListGateRuns(ctx, repo, change.ChangeID)
	if err != nil || len(report.Runs) != 2 {
		t.Fatalf("recovered GateRun history is incomplete: %+v err=%v", report, err)
	}
	if report.Runs[0].RunID != openRun.RunID || report.Runs[0].State != GateRunInterrupted || report.Runs[0].OutcomeCode != "PREVIOUS_PROCESS_EXITED" || report.Runs[1].RunID != result.RunID || report.Runs[1].State != GateRunCompleted {
		t.Fatalf("unexpected GateRun recovery history: %+v", report.Runs)
	}
}

func TestCancelledGateRunIsTerminalAndLiveLeaseIsNotRecovered(t *testing.T) {
	switch runtime.GOOS {
	case "darwin", "dragonfly", "freebsd", "linux", "netbsd", "openbsd":
	default:
		t.Skip("test requires process-group cancellation and advisory locks")
	}
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	marker := filepath.Join(t.TempDir(), "gate-started")
	gate := passingGitGate()
	gate.Command = []string{"sh", "-c", `touch "$1"; sleep 30`, "gate", marker}
	gate.TimeoutSeconds = 60
	bootstrapProjectWithGate(t, ctx, service, repo, gate)
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Cancel GateRun", Goal: "Close a cancelled sequence without misclassifying a live holder", Scope: []string{"src"},
		AcceptanceCriteria: []string{"cancelled GateRun is durable"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "cancel GateRun\n")
	if err := assessPreservedTestChange(t, ctx, service, repo, change.ChangeID); err != nil {
		t.Fatal(err)
	}
	plan, err := service.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}

	type gateOutcome struct {
		result GateRunResult
		err    error
	}
	runContext, cancel := context.WithCancel(ctx)
	outcome := make(chan gateOutcome, 1)
	go func() {
		result, runErr := service.RunGates(runContext, repo, change.ChangeID, plan.PlanDigest, nil)
		outcome <- gateOutcome{result: result, err: runErr}
	}()
	waitForTestFile(t, marker, 5*time.Second)

	contenderContext, stopContender := context.WithTimeout(ctx, 250*time.Millisecond)
	_, contenderErr := service.RunGates(contenderContext, repo, change.ChangeID, plan.PlanDigest, nil)
	stopContender()
	if !errors.Is(contenderErr, context.DeadlineExceeded) && !isErrorCode(contenderErr, "GATE_SEQUENCE_LOCKED") {
		cancel()
		t.Fatalf("concurrent GateRun did not remain behind the live lease: %v", contenderErr)
	}
	report, err := service.ListGateRuns(ctx, repo, change.ChangeID)
	if err != nil || len(report.Runs) != 1 || report.Runs[0].State != GateRunInProgress {
		cancel()
		t.Fatalf("live GateRun was recovered or duplicated by a contender: %+v err=%v", report, err)
	}

	cancel()
	select {
	case finished := <-outcome:
		if !errors.Is(finished.err, context.Canceled) || finished.result.State != GateRunCancelled || finished.result.RunID == "" || len(finished.result.Evidence) != 0 {
			t.Fatalf("cancelled GateRun returned the wrong terminal result: %+v err=%v", finished.result, finished.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled GateRun did not terminate")
	}
	report, err = service.ListGateRuns(ctx, repo, change.ChangeID)
	if err != nil || len(report.Runs) != 1 || report.Runs[0].State != GateRunCancelled || report.Runs[0].OutcomeCode != "CONTEXT_CANCELLED" || !report.Runs[0].FinishedAt.After(report.Runs[0].StartedAt) {
		t.Fatalf("cancelled GateRun history is incomplete: %+v err=%v", report, err)
	}
}

func TestKilledGateRunHolderIsRecoveredAfterAdvisoryLeaseRelease(t *testing.T) {
	switch runtime.GOOS {
	case "darwin", "dragonfly", "freebsd", "linux", "netbsd", "openbsd":
	default:
		t.Skip("platform uses the conservative sentinel-lock fallback")
	}
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Recover killed GateRun", Goal: "Recover durable execution state after its lease holder is killed", Scope: []string{"src"},
		AcceptanceCriteria: []string{"killed holder becomes INTERRUPTED"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "killed GateRun holder\n")
	if err := assessPreservedTestChange(t, ctx, service, repo, change.ChangeID); err != nil {
		t.Fatal(err)
	}
	plan, err := service.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	_, workspace, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	gateIDs := make([]string, 0, len(plan.Gates))
	for _, gate := range plan.Gates {
		gateIDs = append(gateIDs, gate.ID)
	}
	marker := filepath.Join(t.TempDir(), "holder-ready")
	command := exec.Command(os.Args[0], "-test.run=^TestKilledGateRunHolderHelper$")
	command.Env = append(os.Environ(),
		"ECP_TEST_GATE_RUN_CRASH_STATE="+service.StateDir,
		"ECP_TEST_GATE_RUN_CRASH_PROJECT="+workspace.ProjectID,
		"ECP_TEST_GATE_RUN_CRASH_WORKSPACE="+workspace.WorkspaceID,
		"ECP_TEST_GATE_RUN_CRASH_CHANGE="+change.ChangeID,
		"ECP_TEST_GATE_RUN_CRASH_ACTIVATION="+change.ActivationID,
		"ECP_TEST_GATE_RUN_CRASH_PLAN="+plan.PlanDigest,
		"ECP_TEST_GATE_RUN_CRASH_GATES="+strings.Join(gateIDs, ","),
		"ECP_TEST_GATE_RUN_CRASH_MARKER="+marker,
	)
	var childOutput bytes.Buffer
	command.Stdout = &childOutput
	command.Stderr = &childOutput
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	if err := waitForFile(marker, 5*time.Second); err != nil {
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatalf("crash holder did not establish the open GateRun: %v\n%s", err, childOutput.String())
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatalf("could not kill GateRun holder: %v", err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("killed GateRun holder exited successfully instead of observing SIGKILL")
	}

	report, err := service.ListGateRuns(ctx, repo, change.ChangeID)
	if err != nil || len(report.Runs) != 1 || report.Runs[0].State != GateRunInProgress {
		t.Fatalf("killed holder did not leave one durable IN_PROGRESS run: %+v err=%v", report, err)
	}
	result, err := service.RunGates(ctx, repo, change.ChangeID, plan.PlanDigest, nil)
	if err != nil || result.State != GateRunCompleted {
		t.Fatalf("released crash lease could not be recovered before the next run: %+v err=%v", result, err)
	}
	report, err = service.ListGateRuns(ctx, repo, change.ChangeID)
	if err != nil || len(report.Runs) != 2 || report.Runs[0].State != GateRunInterrupted || report.Runs[0].OutcomeCode != "PREVIOUS_PROCESS_EXITED" || report.Runs[1].State != GateRunCompleted {
		t.Fatalf("crash recovery history is incomplete: %+v err=%v", report, err)
	}
}

func TestKilledGateRunHolderHelper(t *testing.T) {
	stateDir := os.Getenv("ECP_TEST_GATE_RUN_CRASH_STATE")
	if stateDir == "" {
		return
	}
	store, err := NewStore(stateDir, os.Getenv("ECP_TEST_GATE_RUN_CRASH_PROJECT"), os.Getenv("ECP_TEST_GATE_RUN_CRASH_WORKSPACE"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	release, err := store.AcquireGateLease(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	projection, err := store.Load(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	run := GateRun{
		SchemaVersion: SchemaVersion,
		RunID:         "run-000000000000000000000099",
		ChangeID:      os.Getenv("ECP_TEST_GATE_RUN_CRASH_CHANGE"),
		ActivationID:  os.Getenv("ECP_TEST_GATE_RUN_CRASH_ACTIVATION"),
		PlanDigest:    os.Getenv("ECP_TEST_GATE_RUN_CRASH_PLAN"),
		GateIDs:       strings.Split(os.Getenv("ECP_TEST_GATE_RUN_CRASH_GATES"), ","),
		State:         GateRunInProgress,
		StartedAt:     time.Now().UTC(),
		EvidenceIDs:   []string{},
	}
	revision := projection.Revision
	if _, err := store.Append(context.Background(), &revision, PendingEvent{Type: "gate_run_started", Origin: "test-crash-holder", Payload: run}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(os.Getenv("ECP_TEST_GATE_RUN_CRASH_MARKER"), []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Hour)
}

func TestGateRunTerminalReplayRejectsImpossibleStates(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Validate GateRun terminal", Goal: "Reject impossible terminal history", Scope: []string{"src"},
		AcceptanceCriteria: []string{"terminal event replay fails closed"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := service.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	openRun := appendOpenGateRun(t, ctx, service, repo, change, plan)
	_, workspace, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	valid := GateRunTerminal{
		SchemaVersion: SchemaVersion, RunID: openRun.RunID, ChangeID: change.ChangeID, ActivationID: change.ActivationID,
		State: GateRunInterrupted, FinishedAt: openRun.StartedAt.Add(time.Second), EvidenceIDs: []string{},
		OutcomeCode: "PREVIOUS_PROCESS_EXITED", Reason: "later holder acquired the released lease",
	}
	cases := []GateRunTerminal{
		func() GateRunTerminal { candidate := valid; candidate.State = GateRunCompleted; return candidate }(),
		func() GateRunTerminal {
			candidate := valid
			candidate.FinishedAt = openRun.StartedAt.Add(-time.Second)
			return candidate
		}(),
		func() GateRunTerminal { candidate := valid; candidate.OutcomeCode = "BAD CODE"; return candidate }(),
		func() GateRunTerminal {
			candidate := valid
			candidate.Reason = strings.Repeat("x", 2001)
			return candidate
		}(),
	}
	for index, candidate := range cases {
		revision := workspace.Projection.Revision
		if _, err := workspace.Store.Append(ctx, &revision, PendingEvent{Type: "gate_run_finished", Origin: "test", Payload: candidate}); err == nil || !isErrorCode(err, "GATE_RUN_TERMINAL_INVALID") {
			t.Fatalf("impossible terminal case %d was accepted: %v", index, err)
		}
	}
}

func waitForTestFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()
	if err := waitForFile(path, timeout); err != nil {
		t.Fatal(err)
	}
}

func waitForFile(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return errors.New("timed out waiting for test marker")
}

func TestStalePlanDoesNotCreateGateRun(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Reject stale plan", Goal: "Do not create execution state for a stale request", Scope: []string{"src"},
		AcceptanceCriteria: []string{"no GateRun is appended"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := service.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "plan changed\n")
	if _, err := service.RunGates(ctx, repo, change.ChangeID, plan.PlanDigest, nil); err == nil || !isErrorCode(err, "GATE_PLAN_MISMATCH") {
		t.Fatalf("stale Gate plan was not rejected: %v", err)
	}
	report, err := service.ListGateRuns(ctx, repo, change.ChangeID)
	if err != nil || len(report.Runs) != 0 {
		t.Fatalf("stale plan created a GateRun: %+v err=%v", report, err)
	}
}

func appendOpenGateRun(t *testing.T, ctx context.Context, service Service, repo string, change Change, plan GatePlan) GateRun {
	t.Helper()
	_, workspace, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	gateIDs := make([]string, 0, len(plan.Gates))
	for _, gate := range plan.Gates {
		gateIDs = append(gateIDs, gate.ID)
	}
	run := GateRun{
		SchemaVersion: SchemaVersion,
		RunID:         "run-000000000000000000000001",
		ChangeID:      change.ChangeID,
		ActivationID:  change.ActivationID,
		PlanDigest:    plan.PlanDigest,
		GateIDs:       gateIDs,
		State:         GateRunInProgress,
		StartedAt:     time.Now().UTC(),
		EvidenceIDs:   []string{},
	}
	revision := workspace.Projection.Revision
	if _, err := workspace.Store.Append(ctx, &revision, PendingEvent{Type: "gate_run_started", Origin: "test", Payload: run}); err != nil {
		t.Fatal(err)
	}
	return run
}

func assessPreservedTestChange(t *testing.T, ctx context.Context, service Service, repo, changeID string) error {
	t.Helper()
	current, err := service.Context(ctx, repo)
	if err != nil {
		return err
	}
	diff, err := service.TruthDiff(ctx, repo)
	if err != nil {
		return err
	}
	_, err = assessTestSemantic(t, ctx, service, repo, SemanticAssessmentInput{
		ExpectedAuthority:      current.AuthorityID,
		ExpectedWorkspace:      current.WorkspaceID,
		ExpectedActivation:     current.ActivationToken,
		ExpectedChangeID:       changeID,
		ExpectedSource:         current.Source.Fingerprint,
		ExpectedPreviousTruth:  diff.PreviousTruthDigest,
		ExpectedCandidateTruth: diff.CandidateTruthDigest,
		Behavior:               SemanticBehaviorPreserved,
		Summary:                "GateRun lifecycle work preserves the accepted Project Truth.",
		Categories:             []string{"operations"},
		Actor:                  "test",
		Reason:                 "record exact semantic state before GateRun",
	})
	return err
}

func TestGateRunHistoryRemainsAuthorityOnlyWhenCandidateIsMalformed(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Query GateRun", Goal: "Recover execution history without candidate config", Scope: []string{"src"},
		AcceptanceCriteria: []string{"history remains readable"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "history query\n")
	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".ecp", "gates.json"), []byte("{malformed"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := service.ListGateRuns(ctx, repo, change.ChangeID)
	if err != nil || len(report.Runs) != 1 || report.Runs[0].State != GateRunCompleted {
		t.Fatalf("malformed candidate blocked GateRun history: %+v err=%v", report, err)
	}
}
