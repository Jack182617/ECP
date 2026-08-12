package ecp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectStatusDefaultsDisabledWithoutCreatingControlPlaneState(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	stateDir := filepath.Join(t.TempDir(), "authority")
	service := Service{StateDir: stateDir, CoreIdentity: "activation-status-test"}

	status, err := service.ProjectStatus(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || status.Registered || status.ConfigPresent || status.ConfigState != ProjectConfigAbsent || status.Assurance != ProjectAssuranceDisabled {
		t.Fatalf("unexpected default project status: %+v", status)
	}
	if !isSHA256Digest(status.ActivationToken) || status.AuthorityID == "" || status.WorkspaceID == "" {
		t.Fatalf("default status omitted its exact internal target: %+v", status)
	}
	if _, err := os.Lstat(filepath.Join(repo, ".ecp")); !os.IsNotExist(err) {
		t.Fatalf("read-only status created repository control files: %v", err)
	}
	if _, err := os.Lstat(stateDir); !os.IsNotExist(err) {
		t.Fatalf("read-only status created authority state: %v", err)
	}
	if _, err := service.InitProject(ctx, repo, "activation-race-test"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.DisableProject(ctx, repo, status.AuthorityID, status.WorkspaceID, status.ActivationToken, "owner", "stale pre-registration disable"); err == nil || !isErrorCode(err, "ACTIVATION_TOKEN_MISMATCH") {
		t.Fatalf("a pre-registration status token silently targeted newly registered authority state: %v", err)
	}
}

func TestProjectActivationIsWorkspaceScopedPersistentAndCloneLocal(t *testing.T) {
	ctx := context.Background()
	stateDir := filepath.Join(t.TempDir(), "authority")
	service := Service{StateDir: stateDir, CoreIdentity: "activation-scope-test"}
	repoA := createTestRepository(t)
	repoB := createTestRepository(t)
	configA := prepareDisabledProjectWithGate(t, ctx, service, repoA, passingGitGate())
	prepareDisabledProjectWithGate(t, ctx, service, repoB, passingGitGate())

	statusA, err := service.ProjectStatus(ctx, repoA)
	if err != nil {
		t.Fatal(err)
	}
	statusB, err := service.ProjectStatus(ctx, repoB)
	if err != nil {
		t.Fatal(err)
	}
	if statusA.Enabled || statusB.Enabled {
		t.Fatalf("configuration or registration enabled a Workspace implicitly: A=%+v B=%+v", statusA, statusB)
	}
	enabled, err := service.EnableProject(ctx, repoA, statusA.AuthorityID, statusA.WorkspaceID, statusA.ActivationToken, configA.Digest, configA.TruthDigest, "owner", "enable only repository A")
	if err != nil {
		t.Fatal(err)
	}
	if !enabled.Enabled || !enabled.Operational || enabled.Assurance != ProjectAssuranceReady || enabled.ActivationID == "" {
		t.Fatalf("repository A did not become operational: %+v", enabled)
	}
	statusB, err = service.ProjectStatus(ctx, repoB)
	if err != nil || statusB.Enabled {
		t.Fatalf("enabling repository A leaked to repository B: %+v err=%v", statusB, err)
	}

	restarted := Service{StateDir: stateDir, CoreIdentity: "activation-scope-test"}
	persisted, err := restarted.ProjectStatus(ctx, repoA)
	if err != nil || !persisted.Enabled || persisted.ActivationID != enabled.ActivationID {
		t.Fatalf("project activation did not persist across Service instances: %+v err=%v", persisted, err)
	}

	runGit(t, repoA, "add", ".ecp")
	runGit(t, repoA, "commit", "-m", "share ECP Draft Config")
	cloneParent := t.TempDir()
	clone := filepath.Join(cloneParent, "clone")
	runGit(t, cloneParent, "clone", repoA, clone)
	cloneStatus, err := restarted.ProjectStatus(ctx, clone)
	if err != nil {
		t.Fatal(err)
	}
	if cloneStatus.Enabled || cloneStatus.Registered || cloneStatus.ConfigState != ProjectConfigUnregistered || cloneStatus.CandidateProjectID != enabled.ProjectID {
		t.Fatalf("a new clone inherited local enablement: %+v", cloneStatus)
	}
}

func TestEnabledProjectNeverFallsBackOnConfigDriftAndCanStillDisable(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())

	appendFile(t, filepath.Join(repo, ".ecp", "contracts", "boundaries.md"), "valid policy drift\n")
	status, err := service.ProjectStatus(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || status.Operational || status.Assurance != ProjectAssuranceBlocked || status.ConfigState != ProjectConfigAccepted || status.TruthState != ProjectConfigPendingAcceptance {
		t.Fatalf("valid Project Truth drift downgraded or remained operational: %+v", status)
	}
	if err := os.WriteFile(filepath.Join(repo, ".ecp", "gates.json"), []byte("{\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err = service.ProjectStatus(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Enabled || status.Assurance != ProjectAssuranceIndeterminate || status.ConfigState != ProjectConfigInvalid {
		t.Fatalf("malformed config was mistaken for disabled mode: %+v", status)
	}
	disabled, err := service.DisableProject(ctx, repo, status.AuthorityID, status.WorkspaceID, status.ActivationToken, "owner", "disable despite malformed Draft Config")
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Enabled || disabled.Assurance != ProjectAssuranceDisabled || disabled.ConfigState != ProjectConfigInvalid {
		t.Fatalf("authority-only disable did not preserve an honest config diagnosis: %+v", disabled)
	}
}

func TestDisableAtomicallyCancelsObservedChangeAndPreservesEvidenceAndSource(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	initial, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	firstActivationID := initial.ActivationID
	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Disable active project", Goal: "Preserve work while ending ECP governance", Scope: []string{"src"},
		AcceptanceCriteria: []string{"disable is audited"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	appendFile(t, filepath.Join(repo, "src", "app.txt"), "kept after disable\n")
	gateResult, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil)
	if err != nil || len(gateResult.Evidence) != 1 {
		t.Fatalf("could not establish Evidence before disable: %+v err=%v", gateResult, err)
	}
	observed, err := service.ProjectStatus(ctx, repo)
	if err != nil || observed.ActiveChange == nil || observed.ActiveChange.ChangeID != change.ChangeID {
		t.Fatalf("status did not observe the exact active Change: %+v err=%v", observed, err)
	}
	disabled, err := service.DisableProject(ctx, repo, observed.AuthorityID, observed.WorkspaceID, observed.ActivationToken, "owner", "close ECP for this project")
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Enabled || disabled.ActiveChange != nil || disabled.Assurance != ProjectAssuranceDisabled || disabled.ActivationID == firstActivationID {
		t.Fatalf("disable did not create a terminal project epoch: %+v", disabled)
	}
	history, err := service.ListChanges(ctx, repo)
	if err != nil || len(history) != 1 || history[0].State != ChangeCancelled || history[0].Cancellation == nil {
		t.Fatalf("active Change was not atomically retained as CANCELLED history: %+v err=%v", history, err)
	}
	report, err := service.ListEvidence(ctx, repo, change.ChangeID)
	if err != nil || len(report.Evidence) != 1 {
		t.Fatalf("disable lost Evidence history: %+v err=%v", report, err)
	}
	contents, err := os.ReadFile(filepath.Join(repo, "src", "app.txt"))
	if err != nil || !strings.Contains(string(contents), "kept after disable") {
		t.Fatalf("disable rewrote the worktree: %q err=%v", contents, err)
	}

	if _, err := service.EnableProject(ctx, repo, disabled.AuthorityID, disabled.WorkspaceID, observed.ActivationToken, disabled.AcceptedConfigDigest, disabled.AcceptedTruthDigest, "owner", "stale re-enable attempt"); err == nil || !isErrorCode(err, "ACTIVATION_TOKEN_MISMATCH") {
		t.Fatalf("stale activation token crossed a disable epoch: %v", err)
	}
	reenabled, err := service.EnableProject(ctx, repo, disabled.AuthorityID, disabled.WorkspaceID, disabled.ActivationToken, disabled.AcceptedConfigDigest, disabled.AcceptedTruthDigest, "owner", "enable a new project epoch")
	if err != nil {
		t.Fatal(err)
	}
	current, err := service.Context(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.StartChange(ctx, repo, StartChangeInput{
		Title: "Stale context", Goal: "Must not cross activation epochs", Scope: []string{"src"}, AcceptanceCriteria: []string{"conflict"}, Risk: RiskModerate,
		ExpectedAuthority: current.AuthorityID, ExpectedWorkspace: current.WorkspaceID, ExpectedActivation: initial.ActivationToken,
		ExpectedConfig: current.CandidateConfig, ExpectedTruth: current.CandidateTruth, ExpectedSource: current.Source.Fingerprint,
		Impact: ChangeImpact{Unknowns: []string{"test impact intentionally not modeled"}},
	}); err == nil || !isErrorCode(err, "ACTIVATION_TOKEN_MISMATCH") {
		t.Fatalf("old Context started a Change in a new activation epoch: %v", err)
	}
	if reenabled.ActivationID == firstActivationID || reenabled.ActivationID == disabled.ActivationID {
		t.Fatalf("activation ID was reused across transitions: old=%s disabled=%s new=%s", firstActivationID, disabled.ActivationID, reenabled.ActivationID)
	}

	_, before, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	idempotent, err := service.DisableProject(ctx, repo, reenabled.AuthorityID, reenabled.WorkspaceID, reenabled.ActivationToken, "owner", "disable the re-enabled epoch")
	if err != nil {
		t.Fatal(err)
	}
	_, afterFirstDisable, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.DisableProject(ctx, repo, idempotent.AuthorityID, idempotent.WorkspaceID, idempotent.ActivationToken, "owner", "idempotent disabled request"); err != nil {
		t.Fatal(err)
	}
	_, afterSecondDisable, err := service.loadAuthority(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if before.Projection.Revision >= afterFirstDisable.Projection.Revision || afterSecondDisable.Projection.Revision != afterFirstDisable.Projection.Revision {
		t.Fatalf("disable transition/idempotency revisions are wrong: before=%d first=%d second=%d", before.Projection.Revision, afterFirstDisable.Projection.Revision, afterSecondDisable.Projection.Revision)
	}
}

func TestStatusAndEnableNeverExecuteConfiguredGate(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	marker := filepath.Join(t.TempDir(), "must-not-exist")
	gate := passingGitGate()
	gate.Command = []string{"sh", "-c", "touch " + marker}
	config := prepareDisabledProjectWithGate(t, ctx, service, repo, gate)
	status, err := service.ProjectStatus(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("project status executed configured project code: %v", err)
	}
	if _, err := service.EnableProject(ctx, repo, status.AuthorityID, status.WorkspaceID, status.ActivationToken, config.Digest, config.TruthDigest, "owner", "enable after reviewing Gate effects"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(marker); !os.IsNotExist(err) {
		t.Fatalf("project enable preflight executed configured project code: %v", err)
	}
}

func TestEventStoreReserveRemainsUntilProjectDisablement(t *testing.T) {
	enabled := NewProjection()
	enabled.Activation = &ProjectActivation{Enabled: true, ActivationID: "act-enabled"}
	if got := eventStoreLimit(enabled); got != maxEventStoreBytes-terminalEventReserve {
		t.Fatalf("enabled project lost its disablement reserve: got=%d", got)
	}
	// A completed or cancelled Change leaves no ACTIVE Change, but project mode
	// remains enabled and must still retain enough capacity to disable later.
	enabled.Changes["chg-terminal"] = &Change{ChangeID: "chg-terminal", State: ChangeCancelled}
	enabled.ChangeOrder = []string{"chg-terminal"}
	if got := eventStoreLimit(enabled); got != maxEventStoreBytes-terminalEventReserve {
		t.Fatalf("terminal Change incorrectly consumed the project disablement reserve: got=%d", got)
	}
	disabled := NewProjection()
	disabled.Activation = &ProjectActivation{Enabled: false, ActivationID: "act-disabled"}
	if got := eventStoreLimit(disabled); got != maxEventStoreBytes {
		t.Fatalf("fully disabled project did not receive the terminal state limit: got=%d", got)
	}
	if got := eventStoreLimit(NewProjection()); got != maxEventStoreBytes-terminalEventReserve {
		t.Fatalf("never-enabled project did not preserve room for its future lifecycle: got=%d", got)
	}
}

func TestStoreRejectsFirstRegistrationForAnotherAuthorityTarget(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name   string
		mutate func(*ProjectRegistration)
	}{
		{name: "project", mutate: func(value *ProjectRegistration) { value.ProjectID = "prj-wrong" }},
		{name: "workspace", mutate: func(value *ProjectRegistration) { value.WorkspaceID = "ws-wrong" }},
		{name: "authority", mutate: func(value *ProjectRegistration) { value.AuthorityID = "auth-00000000000000000000000000000000" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			stateDir := filepath.Join(t.TempDir(), "authority")
			store, err := NewStore(stateDir, "prj-expected", "ws-expected", 0)
			if err != nil {
				t.Fatal(err)
			}
			registration := ProjectRegistration{
				SchemaVersion: SchemaVersion,
				ProjectID:     "prj-expected",
				AuthorityID:   authorityIDForStateDir(store.baseDir),
				WorkspaceID:   "ws-expected",
				RegisteredAt:  store.clock().UTC(),
				CoreVersion:   CoreVersion,
				CoreIdentity:  "registration-target-test",
			}
			test.mutate(&registration)
			if _, err := store.Append(ctx, nil, PendingEvent{Type: "project_registered", Origin: "test", Payload: registration}); err == nil || !isErrorCode(err, "STATE_IDENTITY_MISMATCH") {
				t.Fatalf("wrong first registration target was accepted: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(store.Directory(), "events.json")); !os.IsNotExist(err) {
				t.Fatalf("rejected registration still created events.json: %v", err)
			}
		})
	}
}

func TestStoreRejectsEnablementAndChangeWithoutRequiredGateCoverage(t *testing.T) {
	ctx := context.Background()
	emptyRepo := createTestRepository(t)
	emptyService := newTestService(t)
	if _, err := emptyService.InitProject(ctx, emptyRepo, "empty-store-gates"); err != nil {
		t.Fatal(err)
	}
	resolved, emptyWorkspace, err := emptyService.load(ctx, emptyRepo)
	if err != nil {
		t.Fatal(err)
	}
	activation := ProjectActivation{
		SchemaVersion: SchemaVersion, ActivationID: "act-direct-empty", Enabled: true,
		ProjectID: emptyWorkspace.ProjectID, AuthorityID: resolved.authorityID, WorkspaceID: emptyWorkspace.WorkspaceID,
		ConfigDigest: emptyWorkspace.Config.Digest, TruthDigest: emptyWorkspace.Config.TruthDigest, Actor: "test", Reason: "must reject empty Gates", Trust: "test",
		CoreIdentity: resolved.CoreIdentity, ChangedAt: resolved.Clock().UTC(),
	}
	revision := emptyWorkspace.Projection.Revision
	if _, err := emptyWorkspace.Store.Append(ctx, &revision, PendingEvent{Type: "project_enabled", Origin: "test", Payload: activation}); err == nil || !isErrorCode(err, "PROJECT_ACTIVATION_GATES_MISSING") {
		t.Fatalf("Store enabled a project without default-risk Gate coverage: %v", err)
	}

	repo := createTestRepository(t)
	service := newTestService(t)
	moderate := passingGitGate()
	moderate.RequiredFor = []Risk{RiskModerate}
	bootstrapProjectWithGate(t, ctx, service, repo, moderate)
	bundle, err := LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	bundle.Policy.RiskRules = []PathRiskRule{{PathRoot: "src/sensitive", Risk: RiskHigh}}
	if err := writePrettyJSON(filepath.Join(repo, ".ecp", "policy.json"), bundle.Policy, 0o644); err != nil {
		t.Fatal(err)
	}
	candidate, err := LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acceptTestPolicy(t, ctx, service, repo, candidate.Digest, "owner", "accept high-risk path without high Gate"); err != nil {
		t.Fatal(err)
	}
	resolved, workspace, err := service.load(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := resolved.Git.Snapshot(ctx, repo, workspace.Config.Policy)
	if err != nil {
		t.Fatal(err)
	}
	change := Change{
		SchemaVersion: SchemaVersion, ChangeID: "chg-direct-no-high-gate", ActivationID: workspace.Projection.Activation.ActivationID,
		Title: "Sensitive scope", Goal: "Must not become ACTIVE", Scope: []string{"src"}, AcceptanceCriteria: []string{"high Gate exists"},
		Impact:       ChangeImpact{Unknowns: []string{"test impact intentionally not modeled"}},
		DeclaredRisk: RiskModerate, ConfigDigest: candidate.Digest, TruthDigest: candidate.TruthDigest, Baseline: baseline, State: ChangeActive, CreatedAt: resolved.Clock().UTC(),
	}
	change.ContractDigest, err = changeContractDigest(change)
	if err != nil {
		t.Fatal(err)
	}
	revision = workspace.Projection.Revision
	if _, err := workspace.Store.Append(ctx, &revision, PendingEvent{Type: "change_started", Origin: "test", Payload: change}); err == nil || !isErrorCode(err, "CHANGE_REQUIRED_GATES_MISSING") {
		t.Fatalf("Store started a Change whose reachable risk had no required Gate: %v", err)
	}
	loaded, err := workspace.Store.Load(ctx)
	if err != nil || loaded.Revision != revision || loaded.ActiveChange() != nil {
		t.Fatalf("rejected no-Gate Change mutated authority state: revision=%d active=%+v err=%v", loaded.Revision, loaded.ActiveChange(), err)
	}
}

func TestStoreRejectsInvalidActivationEpochChain(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	resolved, workspace, err := service.load(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	base := ProjectActivation{
		SchemaVersion: SchemaVersion, ActivationID: "act-next-valid", PreviousActivation: workspace.Projection.Activation.ActivationID, Enabled: false,
		ProjectID: workspace.ProjectID, AuthorityID: resolved.authorityID, WorkspaceID: workspace.WorkspaceID,
		ConfigDigest: workspace.Projection.AcceptedConfig.ConfigDigest, Actor: "test", Reason: "validate activation chain", Trust: "test",
		CoreIdentity: resolved.CoreIdentity, ChangedAt: resolved.Clock().UTC(),
	}
	for _, test := range []struct {
		name string
		code string
		edit func(*ProjectActivation)
	}{
		{name: "duplicate-id", code: "DUPLICATE_PROJECT_ACTIVATION_ID", edit: func(value *ProjectActivation) { value.ActivationID = workspace.Projection.Activation.ActivationID }},
		{name: "wrong-previous", code: "PROJECT_ACTIVATION_CHAIN_INVALID", edit: func(value *ProjectActivation) { value.PreviousActivation = "act-unobserved" }},
		{name: "wrong-config", code: "PROJECT_ACTIVATION_CONFIG_MISMATCH", edit: func(value *ProjectActivation) { value.ConfigDigest = "sha256:" + strings.Repeat("0", 64) }},
		{name: "wrong-workspace", code: "PROJECT_ACTIVATION_INVALID", edit: func(value *ProjectActivation) { value.WorkspaceID = "ws-other" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := base
			test.edit(&candidate)
			revision := workspace.Projection.Revision
			if _, err := workspace.Store.Append(ctx, &revision, PendingEvent{Type: "project_disabled", Origin: "test", Payload: candidate}); err == nil || !isErrorCode(err, test.code) {
				t.Fatalf("invalid activation transition was accepted: %v", err)
			}
			loaded, err := workspace.Store.Load(ctx)
			if err != nil || loaded.Revision != revision || !loaded.ECPEnabled() {
				t.Fatalf("rejected activation transition mutated state: revision=%d enabled=%v err=%v", loaded.Revision, loaded.ECPEnabled(), err)
			}
		})
	}
}

func TestStoreRejectsForgedChangeTerminalStateAndCompletionVerdict(t *testing.T) {
	ctx := context.Background()
	repo := createTestRepository(t)
	service := newTestService(t)
	bootstrapProjectWithGate(t, ctx, service, repo, passingGitGate())
	resolved, workspace, err := service.load(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := resolved.Git.Snapshot(ctx, repo, workspace.Config.Policy)
	if err != nil {
		t.Fatal(err)
	}
	forged := Change{
		SchemaVersion: SchemaVersion, ChangeID: "chg-forged-terminal", ActivationID: workspace.Projection.Activation.ActivationID,
		Title: "Forged terminal", Goal: "Must be rejected", Scope: []string{"src"}, AcceptanceCriteria: []string{"reject"},
		DeclaredRisk: RiskModerate, ConfigDigest: workspace.Config.Digest, Baseline: baseline, State: ChangeCompleted, CreatedAt: resolved.Clock().UTC(),
	}
	forged.ContractDigest, err = changeContractDigest(forged)
	if err != nil {
		t.Fatal(err)
	}
	revision := workspace.Projection.Revision
	if _, err := workspace.Store.Append(ctx, &revision, PendingEvent{Type: "change_started", Origin: "test", Payload: forged}); err == nil || !isErrorCode(err, "CHANGE_START_INVALID") {
		t.Fatalf("forged terminal Change start was accepted: %v", err)
	}
	loaded, err := workspace.Store.Load(ctx)
	if err != nil || loaded.Revision != revision || loaded.ActiveChange() != nil {
		t.Fatalf("rejected Change start mutated authority state: revision=%d active=%+v err=%v", loaded.Revision, loaded.ActiveChange(), err)
	}

	change, err := startTestChange(t, ctx, service, repo, StartChangeInput{
		Title: "Real Change", Goal: "Establish a real PASS", Scope: []string{"src"}, AcceptanceCriteria: []string{"Gate passes"}, Risk: RiskModerate,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runTestGates(t, ctx, service, repo, change.ChangeID, nil); err != nil {
		t.Fatal(err)
	}
	verdict, err := service.Verdict(ctx, repo)
	if err != nil || verdict.Status != VerdictPass {
		t.Fatalf("real PASS precondition failed: %+v err=%v", verdict, err)
	}
	realVerdict := verdict
	verdict.ContractDigest = "sha256:" + strings.Repeat("0", 64)
	verdict.SubjectDigest, err = verdictSubjectDigest(verdict)
	if err != nil {
		t.Fatal(err)
	}
	verdictDigest, err := digestJSON(verdict)
	if err != nil {
		t.Fatal(err)
	}
	completion := ChangeCompletion{
		SchemaVersion: SchemaVersion, ChangeID: change.ChangeID, ActivationID: change.ActivationID,
		SubjectDigest: verdict.SubjectDigest, VerdictDigest: verdictDigest, FinalVerdict: verdict, CompletedAt: resolved.Clock().UTC(),
	}
	_, currentWorkspace, err := service.load(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	revision = currentWorkspace.Projection.Revision
	if _, err := currentWorkspace.Store.Append(ctx, &revision, PendingEvent{Type: "change_completed", Origin: "test", Payload: completion}); err == nil || !isErrorCode(err, "COMPLETION_BINDING_INVALID") {
		t.Fatalf("completion with a forged contract-bound Verdict was accepted: %v", err)
	}
	riskVerdict := realVerdict
	riskVerdict.EffectiveRisk = RiskLow
	riskVerdict.SubjectDigest, err = verdictSubjectDigest(riskVerdict)
	if err != nil {
		t.Fatal(err)
	}
	riskVerdictDigest, err := digestJSON(riskVerdict)
	if err != nil {
		t.Fatal(err)
	}
	riskCompletion := ChangeCompletion{
		SchemaVersion: SchemaVersion, ChangeID: change.ChangeID, ActivationID: change.ActivationID,
		SubjectDigest: riskVerdict.SubjectDigest, VerdictDigest: riskVerdictDigest, FinalVerdict: riskVerdict, CompletedAt: resolved.Clock().UTC(),
	}
	if _, err := currentWorkspace.Store.Append(ctx, &revision, PendingEvent{Type: "change_completed", Origin: "test", Payload: riskCompletion}); err == nil || !isErrorCode(err, "COMPLETION_RISK_INVALID") {
		t.Fatalf("completion lowered effective risk below the Change contract: %v", err)
	}
	loaded, err = currentWorkspace.Store.Load(ctx)
	if err != nil || loaded.Revision != revision || loaded.ActiveChange() == nil || loaded.ActiveChange().ChangeID != change.ChangeID {
		t.Fatalf("rejected completion mutated the active Change: revision=%d active=%+v err=%v", loaded.Revision, loaded.ActiveChange(), err)
	}
}

func prepareDisabledProjectWithGate(t *testing.T, ctx context.Context, service Service, repo string, gate GateConfig) ConfigBundle {
	t.Helper()
	if _, err := service.InitProject(ctx, repo, "activation-test-project"); err != nil {
		t.Fatal(err)
	}
	writeGates(t, repo, GatesConfig{SchemaVersion: SchemaVersion, Gates: []GateConfig{gate}})
	config, err := LoadConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acceptTestPolicy(t, ctx, service, repo, config.Digest, "owner", "accept activation test Gate"); err != nil {
		t.Fatal(err)
	}
	status, err := service.ProjectStatus(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if status.Enabled || !status.Registered || status.ConfigState != ProjectConfigAccepted {
		t.Fatalf("prepared project was not disabled with accepted config: %+v", status)
	}
	return config
}
