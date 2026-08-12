package ecp

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuthorityDirectoryChainsRejectIntermediateSymlinks(t *testing.T) {
	t.Run("state root replacement", func(t *testing.T) {
		parent := t.TempDir()
		state := filepath.Join(parent, "state")
		if err := os.Mkdir(state, 0o700); err != nil {
			t.Fatal(err)
		}
		store, err := NewStore(state, "prj-root-symlink", "ws-root-symlink", time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(state); err != nil {
			t.Fatal(err)
		}
		external := filepath.Join(t.TempDir(), "external")
		if err := os.Mkdir(external, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, state); err != nil {
			t.Fatal(err)
		}
		registration := ProjectRegistration{
			SchemaVersion: SchemaVersion, ProjectID: "prj-root-symlink", AuthorityID: store.authorityID, WorkspaceID: "ws-root-symlink",
			RegisteredAt: time.Now().UTC(), CoreVersion: CoreVersion, CoreIdentity: "test-core",
		}
		if _, err := store.Append(context.Background(), nil, PendingEvent{Type: "project_registered", Origin: "test", Payload: registration}); err == nil || !isErrorCode(err, "UNSAFE_STATE_DIR") {
			t.Fatalf("replaced state root symlink was not rejected: %v", err)
		}
		if mode := mustLstat(t, external).Mode().Perm(); mode != 0o755 {
			t.Fatalf("rejected state root symlink chmodded external directory: %o", mode)
		}
	})

	t.Run("projects write", func(t *testing.T) {
		state := filepath.Join(t.TempDir(), "state")
		external := filepath.Join(t.TempDir(), "external")
		if err := os.MkdirAll(state, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(external, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, filepath.Join(state, "projects")); err != nil {
			t.Fatal(err)
		}
		store, err := NewStore(state, "prj-symlink", "ws-symlink", time.Second)
		if err != nil {
			t.Fatal(err)
		}
		registration := ProjectRegistration{
			SchemaVersion: SchemaVersion, ProjectID: "prj-symlink", AuthorityID: store.authorityID, WorkspaceID: "ws-symlink",
			RegisteredAt: time.Now().UTC(), CoreVersion: CoreVersion, CoreIdentity: "test-core",
		}
		if _, err := store.Append(context.Background(), nil, PendingEvent{Type: "project_registered", Origin: "test", Payload: registration}); err == nil || !isErrorCode(err, "UNSAFE_STATE_DIR") {
			t.Fatalf("projects symlink was not rejected before writing: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(external, "prj-symlink")); !os.IsNotExist(err) {
			t.Fatalf("rejected authority path wrote outside state root: %v", err)
		}
		if mode := mustLstat(t, external).Mode().Perm(); mode != 0o755 {
			t.Fatalf("rejected authority path chmodded external directory: %o", mode)
		}
	})

	t.Run("artifacts write and verify", func(t *testing.T) {
		ctx := context.Background()
		repo := createTestRepository(t)
		service := newTestService(t)
		if _, err := service.InitProject(ctx, repo, "artifact-symlink"); err != nil {
			t.Fatal(err)
		}
		_, workspace, err := service.loadAuthority(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		external := filepath.Join(t.TempDir(), "external")
		if err := os.Mkdir(external, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, filepath.Join(workspace.Store.Directory(), "artifacts")); err != nil {
			t.Fatal(err)
		}
		if _, err := workspace.Store.WriteArtifacts("chg-safe", "evd-safe", []byte("out"), []byte("err")); err == nil || !isErrorCode(err, "UNSAFE_STATE_DIR") {
			t.Fatalf("artifact ancestor symlink was not rejected: %v", err)
		}
		if _, err := os.Lstat(filepath.Join(external, "chg-safe")); !os.IsNotExist(err) {
			t.Fatalf("rejected artifact write escaped authority root: %v", err)
		}

		if err := os.Remove(filepath.Join(workspace.Store.Directory(), "artifacts")); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(external, "chg-safe", "evd-safe"), 0o700); err != nil {
			t.Fatal(err)
		}
		content := []byte("forged")
		if err := os.WriteFile(filepath.Join(external, "chg-safe", "evd-safe", "stdout.log"), content, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, filepath.Join(workspace.Store.Directory(), "artifacts")); err != nil {
			t.Fatal(err)
		}
		if err := workspace.Store.VerifyArtifact("artifacts/chg-safe/evd-safe/stdout.log", digestBytes(content), int64(len(content))); err == nil || !isErrorCode(err, "ARTIFACT_UNSAFE") {
			t.Fatalf("VerifyArtifact followed an intermediate symlink: %v", err)
		}
	})

	for _, level := range []string{"artifacts", "change"} {
		t.Run("artifact ancestor "+level, func(t *testing.T) {
			ctx := context.Background()
			repo := createTestRepository(t)
			service := newTestService(t)
			if _, err := service.InitProject(ctx, repo, "artifact-level-"+level); err != nil {
				t.Fatal(err)
			}
			_, workspace, err := service.loadAuthority(ctx, repo)
			if err != nil {
				t.Fatal(err)
			}
			changeID, evidenceID := "chg-level", "evd-level"
			artifacts := filepath.Join(workspace.Store.Directory(), "artifacts")
			if level != "artifacts" {
				if err := os.Mkdir(artifacts, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			external := filepath.Join(t.TempDir(), "external")
			if err := os.Mkdir(external, 0o700); err != nil {
				t.Fatal(err)
			}
			link := artifacts
			switch level {
			case "change":
				link = filepath.Join(artifacts, changeID)
			}
			if err := os.Symlink(external, link); err != nil {
				t.Fatal(err)
			}
			if _, err := workspace.Store.WriteArtifacts(changeID, evidenceID, []byte("out"), []byte("err")); err == nil || !isErrorCode(err, "UNSAFE_STATE_DIR") {
				t.Fatalf("%s ancestor symlink was not rejected: %v", level, err)
			}
		})
	}

	t.Run("artifact evidence directory verify", func(t *testing.T) {
		ctx := context.Background()
		repo := createTestRepository(t)
		service := newTestService(t)
		if _, err := service.InitProject(ctx, repo, "artifact-evidence-level"); err != nil {
			t.Fatal(err)
		}
		_, workspace, err := service.loadAuthority(ctx, repo)
		if err != nil {
			t.Fatal(err)
		}
		changeID, evidenceID := "chg-level", "evd-level"
		changeDir := filepath.Join(workspace.Store.Directory(), "artifacts", changeID)
		if err := os.MkdirAll(changeDir, 0o700); err != nil {
			t.Fatal(err)
		}
		external := filepath.Join(t.TempDir(), "external")
		if err := os.Mkdir(external, 0o700); err != nil {
			t.Fatal(err)
		}
		content := []byte("escaped")
		if err := os.WriteFile(filepath.Join(external, "stdout.log"), content, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, filepath.Join(changeDir, evidenceID)); err != nil {
			t.Fatal(err)
		}
		if err := workspace.Store.VerifyArtifact("artifacts/"+changeID+"/"+evidenceID+"/stdout.log", digestBytes(content), int64(len(content))); err == nil || !isErrorCode(err, "ARTIFACT_UNSAFE") {
			t.Fatalf("evidence directory symlink was not rejected by VerifyArtifact: %v", err)
		}
	})

	t.Run("workspace bindings", func(t *testing.T) {
		state := filepath.Join(t.TempDir(), "state")
		external := filepath.Join(t.TempDir(), "external")
		if err := os.MkdirAll(state, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(external, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, filepath.Join(state, "workspace-bindings")); err != nil {
			t.Fatal(err)
		}
		binding := WorkspaceBinding{
			SchemaVersion: SchemaVersion, AuthorityID: "auth-00000000000000000000000000000000", WorkspaceID: "ws-safe", ProjectID: "prj-safe",
			InitialConfigDigest: digestBytes([]byte("config")), InitialTruthDigest: digestBytes([]byte("truth")), Actor: "test", Reason: "test", Trust: "test", CoreIdentity: "test", BoundAt: time.Now().UTC(),
		}
		if err := createWorkspaceBinding(context.Background(), state, binding); err == nil || !isErrorCode(err, "UNSAFE_STATE_DIR") {
			t.Fatalf("binding creation followed intermediate symlink: %v", err)
		}
		if _, err := loadWorkspaceBinding(state, binding.WorkspaceID); err == nil || !isErrorCode(err, "WORKSPACE_BINDING_UNSAFE") {
			t.Fatalf("binding read followed intermediate symlink: %v", err)
		}
	})
}

func TestProjectTruthRejectsDuplicateContractPathsRegardlessOfOrder(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		truth := DefaultProjectTruth("duplicate-contract-path")
		first := truth.Contracts[0]
		second := first
		second.ID = "duplicate-boundaries"
		if reverse {
			truth.Contracts = []TruthContractRef{second, first}
		} else {
			truth.Contracts = []TruthContractRef{first, second}
		}
		if err := validateProjectTruth(truth, GatesConfig{SchemaVersion: SchemaVersion}); err == nil || !isErrorCode(err, "DUPLICATE_TRUTH_CONTRACT_PATH") {
			t.Fatalf("duplicate contract path order reverse=%v was accepted: %v", reverse, err)
		}
	}
}

func TestEvidenceReplayRejectsNonCanonicalOrInconsistentRecords(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Evidence replay", Goal: "Reject forged Evidence", Scope: []string{"src"}, AcceptanceCriteria: []string{"forged records fail closed"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "evidence replay\n")
	result, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil)
	if err != nil || len(result.Evidence) != 1 {
		t.Fatalf("could not create valid Evidence fixture: %+v err=%v", result, err)
	}
	_, workspace, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	valid := result.Evidence[0]
	valid.EvidenceID = "evd-forged"
	valid.GateRunID = ""
	valid.StdoutArtifact = "artifacts/" + change.ChangeID + "/" + result.Evidence[0].EvidenceID + "/stdout.log"
	valid.StderrArtifact = "artifacts/" + change.ChangeID + "/" + result.Evidence[0].EvidenceID + "/stderr.log"
	before := workspace.Projection.Revision
	if _, err := workspace.Store.Append(ctx, &before, PendingEvent{Type: "evidence_recorded", Origin: "test", Payload: valid}); err == nil || !isErrorCode(err, "EVIDENCE_ARTIFACT_PATH_INVALID") {
		t.Fatalf("Evidence replay accepted canonical-looking references to another Evidence ID: %v", err)
	}
	valid.StdoutArtifact = "artifacts/" + change.ChangeID + "/" + valid.EvidenceID + "/stdout.log"
	valid.StderrArtifact = "artifacts/" + change.ChangeID + "/" + valid.EvidenceID + "/stderr.log"
	valid.StdoutBytes = -1
	if _, err := workspace.Store.Append(ctx, &before, PendingEvent{Type: "evidence_recorded", Origin: "test", Payload: valid}); err == nil || !isErrorCode(err, "EVIDENCE_OUTPUT_INVALID") {
		t.Fatalf("Evidence replay accepted inconsistent output metadata: %v", err)
	}
	after, err := workspace.Store.Load(ctx)
	if err != nil || after.Revision != before {
		t.Fatalf("rejected Evidence replay mutated authority: revision=%d want=%d err=%v", after.Revision, before, err)
	}
}

func TestGateRunCapacityHeadroomAndAtomicDisableRecovery(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Capacity recovery", Goal: "Keep terminal transitions available", Scope: []string{"src"}, AcceptanceCriteria: []string{"disable closes atomically"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "capacity recovery\n")
	if err := assessPreservedTestChange(t, ctx, service, repo, change.ChangeID); err != nil {
		t.Fatal(err)
	}
	plan, err := service.PlanGates(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	run := appendOpenGateRun(t, ctx, service, repo, change, plan)
	_, workspace, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	status, err := service.ProjectStatus(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	before := workspace.Projection.Revision
	disabled, err := service.DisableProject(ctx, repo, status.AuthorityID, status.WorkspaceID, status.ActivationToken, "owner", "atomically interrupt, cancel, and disable")
	if err != nil {
		t.Fatal(err)
	}
	_, finalWorkspace, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	stored := finalWorkspace.Projection.GateRuns[run.RunID]
	if disabled.Enabled || finalWorkspace.Projection.Revision != before+3 || stored == nil || stored.State != GateRunInterrupted || finalWorkspace.Projection.ActiveChange() != nil {
		t.Fatalf("disable did not atomically append terminal+cancellation+disable: status=%+v revision=%d want=%d run=%+v active=%+v", disabled, finalWorkspace.Projection.Revision, before+3, stored, finalWorkspace.Projection.ActiveChange())
	}

	// The Store test seams make a small history exercise the production reserve
	// policy without generating GiB-sized fixtures.
	// The estimator sizes the worst terminal from the durable pre-disable
	// projection and must stay conservative relative to a concrete terminal.
	terminalBytes, err := maximumGateRunTerminalSegmentBytes(workspace.Projection)
	if err != nil {
		t.Fatal(err)
	}
	resolvedService, err := service.withDefaults()
	if err != nil {
		t.Fatal(err)
	}
	terminal := buildGateRunTerminal(resolvedService, &run, GateRunInterrupted, "PREVIOUS_PROCESS_EXITED", "capacity test")
	encoded, err := json.Marshal(terminal)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) >= terminalBytes {
		t.Fatalf("terminal headroom estimator is not conservative: payload=%d estimate=%d", len(encoded), terminalBytes)
	}
}

func TestGateRunStartThroughTerminalUsesProtectedByteAndSegmentReserves(t *testing.T) {
	t.Run("byte reserve", func(t *testing.T) {
		ctx, service, _, workspace, change, plan := createCapacityRunReadyWorkspace(t)
		run := capacityRun(change, plan)
		start := PendingEvent{Type: "gate_run_started", Origin: "test", Payload: run}
		startSize := pendingEventSegmentSize(t, workspace.Projection, start)
		workspace.Store.maxEventStoreBytes = max(terminalEventReserve+1, storeHistoryBytesForTest(t, workspace.Store, ctx)+int64(startSize)+terminalEventReserve+4096)
		revision := workspace.Projection.Revision
		started, err := workspace.Store.Append(ctx, &revision, start)
		if err != nil || started.ActiveGateRun() == nil {
			t.Fatalf("near-byte-limit GateRun start was not admitted with terminal reserve: run=%+v err=%v", started.ActiveGateRun(), err)
		}
		terminal := capacityTerminal(t, service, run)
		revision = started.Revision
		closed, err := workspace.Store.Append(ctx, &revision, PendingEvent{Type: "gate_run_finished", Origin: "test", Payload: terminal})
		if err != nil || closed.ActiveGateRun() != nil {
			t.Fatalf("terminal could not use protected byte reserve: projection=%+v err=%v", closed.ActiveGateRun(), err)
		}
	})

	t.Run("segment reserve", func(t *testing.T) {
		ctx, service, _, workspace, change, plan := createCapacityRunReadyWorkspace(t)
		run := capacityRun(change, plan)
		start := PendingEvent{Type: "gate_run_started", Origin: "test", Payload: run}
		startSize := pendingEventSegmentSize(t, workspace.Projection, start)
		terminal := capacityTerminal(t, service, run)
		terminalSize := pendingEventSegmentSize(t, workspace.Projection, PendingEvent{Type: "gate_run_finished", Origin: "test", Payload: terminal})
		terminalHeadroom, err := maximumGateRunTerminalSegmentBytes(func() Projection {
			candidate := workspace.Projection
			candidate.GateRuns[run.RunID] = &run
			candidate.GateRunOrder = append(append([]string(nil), candidate.GateRunOrder...), run.RunID)
			return candidate
		}())
		if err != nil {
			t.Fatal(err)
		}
		workspace.Store.eventSegmentBytes = max(max(startSize, terminalSize), terminalHeadroom) + 1024
		workspace.Store.maxEventSegments = max(terminalSegmentReserve+1, storeHistorySegmentsForTest(t, workspace.Store, ctx)+terminalSegmentReserve+1)
		revision := workspace.Projection.Revision
		started, err := workspace.Store.Append(ctx, &revision, start)
		if err != nil || started.ActiveGateRun() == nil {
			t.Fatalf("near-segment-limit GateRun start was not admitted with terminal reserve: run=%+v err=%v", started.ActiveGateRun(), err)
		}
		revision = started.Revision
		closed, err := workspace.Store.Append(ctx, &revision, PendingEvent{Type: "gate_run_finished", Origin: "test", Payload: terminal})
		if err != nil || closed.ActiveGateRun() != nil {
			t.Fatalf("terminal could not use protected continuation segment: projection=%+v err=%v", closed.ActiveGateRun(), err)
		}
	})
}

func createCapacityRunReadyWorkspace(t *testing.T) (context.Context, Service, string, loadedWorkspace, Change, GatePlan) {
	t.Helper()
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Reserve fixture", Goal: "Exercise protected capacity", Scope: []string{"src"}, AcceptanceCriteria: []string{"terminal remains possible"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "reserve fixture\n")
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
	return ctx, service, repo, workspace, change, plan
}

func capacityRun(change Change, plan GatePlan) GateRun {
	gateIDs := make([]string, 0, len(plan.Gates))
	for _, gate := range plan.Gates {
		gateIDs = append(gateIDs, gate.ID)
	}
	return GateRun{
		SchemaVersion: SchemaVersion, RunID: "run-capacity", ChangeID: change.ChangeID, ActivationID: change.ActivationID,
		PlanDigest: plan.PlanDigest, GateIDs: gateIDs, State: GateRunInProgress, StartedAt: time.Now().UTC(), EvidenceIDs: []string{},
	}
}

func capacityTerminal(t *testing.T, service Service, run GateRun) GateRunTerminal {
	t.Helper()
	resolved, err := service.withDefaults()
	if err != nil {
		t.Fatal(err)
	}
	return buildGateRunTerminal(resolved, &run, GateRunInterrupted, "PREVIOUS_PROCESS_EXITED", "capacity regression")
}

func pendingEventSegmentSize(t *testing.T, projection Projection, pending PendingEvent) int {
	t.Helper()
	payload, err := json.Marshal(pending.Payload)
	if err != nil {
		t.Fatal(err)
	}
	event := Event{
		SchemaVersion: SchemaVersion, Sequence: projection.Revision + 1, EventID: "evt-capacity", Timestamp: time.Now().UTC(), Type: pending.Type, Origin: pending.Origin,
		PreviousHash: projection.EventHead, Payload: payload,
	}
	event.Hash, err = hashEvent(event)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := encodeEventSegment([]Event{event})
	if err != nil {
		t.Fatal(err)
	}
	return len(encoded)
}

func storeHistoryBytesForTest(t *testing.T, store *Store, ctx context.Context) int64 {
	t.Helper()
	history, err := store.loadEventHistory(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	return history.totalBytes
}

func storeHistorySegmentsForTest(t *testing.T, store *Store, ctx context.Context) int {
	t.Helper()
	history, err := store.loadEventHistory(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	return history.segmentCount
}

func mustLstat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}
