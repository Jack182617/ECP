package ecp

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

type Service struct {
	StateDir     string
	CoreIdentity string
	Clock        func() time.Time
	Git          Git
	Runner       Runner
	authorityID  string
}

type StartChangeInput struct {
	Title              string
	Goal               string
	Scope              []string
	NonGoals           []string
	AcceptanceCriteria []string
	Impact             ChangeImpact
	Requirements       []ChangeRequirement
	SupersedesChangeID string
	Risk               Risk
	ExpectedAuthority  string
	ExpectedWorkspace  string
	ExpectedActivation string
	ExpectedConfig     string
	ExpectedTruth      string
	ExpectedSource     string
}

type loadedWorkspace struct {
	Root        string
	ProjectID   string
	Config      ConfigBundle
	Binding     WorkspaceBinding
	WorkspaceID string
	Store       *Store
	Projection  Projection
}

func (s Service) withDefaults() (Service, error) {
	if s.Clock == nil {
		s.Clock = time.Now
	}
	if s.Runner.Clock == nil {
		s.Runner.Clock = s.Clock
	}
	if s.StateDir == "" {
		stateDir, err := DefaultStateDir()
		if err != nil {
			return Service{}, err
		}
		s.StateDir = stateDir
	} else if err := validateExplicitStateDir(s.StateDir); err != nil {
		return Service{}, err
	}
	stateDir, err := canonicalPotentialPath(s.StateDir)
	if err != nil {
		return Service{}, err
	}
	s.StateDir = stateDir
	s.authorityID = authorityIDForStateDir(stateDir)
	if strings.TrimSpace(s.CoreIdentity) == "" {
		identity, err := runtimeCoreIdentity()
		if err != nil {
			return Service{}, err
		}
		s.CoreIdentity = identity
	}
	return s, nil
}

func (s Service) CoreBuildIdentity() (string, error) {
	if strings.TrimSpace(s.CoreIdentity) != "" {
		return s.CoreIdentity, nil
	}
	return runtimeCoreIdentity()
}

// ProjectStatus returns the project-level ECP activation state without
// executing project code or walking the source tree. A Workspace with no
// authority binding is disabled by definition.
func (s Service) ProjectStatus(ctx context.Context, start string) (ProjectStatus, error) {
	service, err := s.withDefaults()
	if err != nil {
		return ProjectStatus{}, err
	}
	if err := requireAuthorityPlatform(); err != nil {
		return ProjectStatus{}, err
	}
	root, err := service.Git.DiscoverRoot(ctx, start)
	if err != nil {
		return ProjectStatus{}, err
	}
	if err := ensureOutsideRoot(root, service.StateDir); err != nil {
		return ProjectStatus{}, err
	}
	workspaceID, err := service.Git.WorkspaceID(ctx, root)
	if err != nil {
		return ProjectStatus{}, err
	}
	status := ProjectStatus{
		SchemaVersion: SchemaVersion,
		Root:          root,
		AuthorityID:   service.authorityID,
		WorkspaceID:   workspaceID,
		ConfigState:   ProjectConfigAbsent,
		TruthState:    ProjectConfigAbsent,
		Assurance:     ProjectAssuranceDisabled,
		Diagnostics:   []VerdictReason{},
	}
	status.ActivationToken, err = projectActivationToken(service.authorityID, workspaceID, "", false, NewProjection())
	if err != nil {
		return ProjectStatus{}, err
	}

	var config ConfigBundle
	var configErr error
	if _, err := os.Lstat(filepath.Join(root, ".ecp")); err == nil {
		status.ConfigPresent = true
		config, configErr = LoadConfig(root)
		if configErr == nil {
			status.CandidateProjectID = config.Project.ProjectID
			status.CandidateConfigDigest = config.Digest
			status.CandidateTruthDigest = config.TruthDigest
		}
	} else if !os.IsNotExist(err) {
		status.ConfigPresent = true
		configErr = newError(KindRuntime, "CONFIG_STAT_FAILED", "could not inspect existing .ecp", err)
	}

	binding, bindingErr := loadWorkspaceBinding(service.StateDir, workspaceID)
	if bindingErr != nil {
		if !isErrorCodeValue(bindingErr, "WORKSPACE_NOT_REGISTERED") {
			return ProjectStatus{}, bindingErr
		}
		if status.ConfigPresent {
			if configErr != nil {
				status.ConfigState = ProjectConfigInvalid
				status.Diagnostics = append(status.Diagnostics, diagnosticFromError(configErr))
			} else {
				status.ConfigState = ProjectConfigUnregistered
				status.TruthState = ProjectConfigUnregistered
			}
		}
		return status, nil
	}
	if binding.AuthorityID != service.authorityID {
		return ProjectStatus{}, newError(KindIntegrity, "AUTHORITY_BINDING_MISMATCH", "Workspace binding belongs to a different authority-state location", nil)
	}
	store, err := NewStore(service.StateDir, binding.ProjectID, workspaceID, 5*time.Second)
	if err != nil {
		return ProjectStatus{}, err
	}
	projection, err := store.Load(ctx)
	if err != nil {
		return ProjectStatus{}, err
	}
	if projection.AcceptedTruth != nil {
		if _, err := store.ReadTruthFiles(projection.AcceptedTruth.Files); err != nil {
			return ProjectStatus{}, err
		}
	}
	if projection.Registration == nil || projection.Registration.AuthorityID != service.authorityID || projection.Registration.ProjectID != binding.ProjectID {
		return ProjectStatus{}, newError(KindIntegrity, "AUTHORITY_REGISTRATION_MISMATCH", "authority history belongs to a different project or authority-state location", nil)
	}
	status.Registered = true
	status.ProjectID = binding.ProjectID
	status.Enabled = projection.ECPEnabled()
	if projection.Activation != nil {
		status.ActivationID = projection.Activation.ActivationID
	}
	status.ActivationToken, err = projectActivationToken(service.authorityID, workspaceID, binding.ProjectID, true, projection)
	if err != nil {
		return ProjectStatus{}, err
	}
	if active := projection.ActiveChange(); active != nil {
		status.ActiveChange = summarizeChange(active)
	}
	status.ActiveGateRun = projection.ActiveGateRun()
	if !status.ConfigPresent {
		status.ConfigState = ProjectConfigMissing
		status.TruthState = ProjectConfigMissing
		status.Diagnostics = append(status.Diagnostics, VerdictReason{Code: "REGISTERED_CONFIG_MISSING", Message: "registered Workspace is missing required Draft Config"})
		if status.Enabled {
			status.Assurance = ProjectAssuranceIndeterminate
		}
		return status, nil
	}
	if configErr != nil {
		status.ConfigState = ProjectConfigInvalid
		status.TruthState = ProjectConfigInvalid
		status.Diagnostics = append(status.Diagnostics, diagnosticFromError(configErr))
		if status.Enabled {
			status.Assurance = ProjectAssuranceIndeterminate
		}
		return status, nil
	}
	if config.Project.ProjectID != binding.ProjectID {
		status.ConfigState = ProjectConfigPendingAcceptance
		status.Diagnostics = append(status.Diagnostics, VerdictReason{Code: "PROJECT_ID_IMMUTABLE", Message: "Draft project_id differs from the Workspace authority binding"})
	} else if projection.AcceptedConfig == nil || projection.AcceptedConfig.ConfigDigest != config.Digest {
		status.ConfigState = ProjectConfigPendingAcceptance
		status.Diagnostics = append(status.Diagnostics, VerdictReason{Code: "CONFIG_NOT_ACCEPTED", Message: "candidate config does not match the accepted config epoch"})
	} else {
		status.ConfigState = ProjectConfigAccepted
		status.AcceptedConfigDigest = projection.AcceptedConfig.ConfigDigest
	}
	if projection.AcceptedTruth == nil || projection.AcceptedTruth.TruthDigest != config.TruthDigest {
		status.TruthState = ProjectConfigPendingAcceptance
		status.Diagnostics = append(status.Diagnostics, VerdictReason{Code: "PROJECT_TRUTH_NOT_ACCEPTED", Message: "candidate Project Truth does not match the accepted truth epoch"})
	} else {
		status.TruthState = ProjectConfigAccepted
		status.AcceptedTruthDigest = projection.AcceptedTruth.TruthDigest
	}
	if !status.Enabled {
		return status, nil
	}
	if status.ConfigState != ProjectConfigAccepted || status.TruthState != ProjectConfigAccepted {
		status.Assurance = ProjectAssuranceBlocked
		return status, nil
	}
	if config.Truth.Maturity == TruthMaturitySeed {
		status.Diagnostics = append(status.Diagnostics, VerdictReason{Code: "PROJECT_TRUTH_SEED", Message: "Project Truth is still a bootstrap seed; establish purpose, capabilities, invariants, and components in a bounded onboarding Change"})
	}
	required := requiredGates(config.Gates, config.Policy.DefaultRisk)
	if len(required) == 0 {
		status.Assurance = ProjectAssuranceBlocked
		status.Diagnostics = append(status.Diagnostics, VerdictReason{Code: "NO_REQUIRED_GATES", Message: "the accepted config has no required Gate for the default risk"})
		return status, nil
	}
	for _, gate := range required {
		if _, err := resolveGateExecutionContext(root, config.Policy, gate); err != nil {
			status.Assurance = ProjectAssuranceBlocked
			status.Diagnostics = append(status.Diagnostics, VerdictReason{Code: "GATE_EXECUTION_CONTEXT_UNAVAILABLE", Message: fmt.Sprintf("required Gate %q cannot run in the current Workspace: %v", gate.ID, err), GateID: gate.ID})
			return status, nil
		}
	}
	status.Operational = true
	status.Assurance = ProjectAssuranceReady
	if status.ActiveGateRun != nil {
		status.Assurance = ProjectAssuranceIndeterminate
		status.Diagnostics = append(status.Diagnostics, VerdictReason{Code: "GATE_RUN_IN_PROGRESS_OR_INTERRUPTED", Message: "a durable GateRun is IN_PROGRESS; it may still be executing or may require interruption recovery by the next lease holder"})
	} else if status.ActiveChange != nil {
		status.Assurance = ProjectAssuranceActive
	}
	return status, nil
}

func diagnosticFromError(err error) VerdictReason {
	var typed *ECPError
	if errors.As(err, &typed) {
		return VerdictReason{Code: typed.Code, Message: typed.Message}
	}
	return VerdictReason{Code: "STATUS_ERROR", Message: err.Error()}
}

func summarizeChange(change *Change) *ChangeSummary {
	if change == nil {
		return nil
	}
	return &ChangeSummary{
		ChangeID:            change.ChangeID,
		ActivationID:        change.ActivationID,
		Title:               change.Title,
		State:               change.State,
		DeclaredRisk:        change.DeclaredRisk,
		Scope:               append([]string(nil), change.Scope...),
		ConfigDigest:        change.ConfigDigest,
		TruthDigest:         change.TruthDigest,
		Baseline:            change.Baseline.Ref(),
		SupersedesChangeID:  change.SupersedesChangeID,
		LineageRootChangeID: change.LineageRootChangeID,
	}
}

func projectActivationToken(authorityID, workspaceID, projectID string, registered bool, projection Projection) (string, error) {
	activationID := ""
	enabled := false
	if projection.Activation != nil {
		activationID = projection.Activation.ActivationID
		enabled = projection.Activation.Enabled
	}
	token, err := digestJSON(struct {
		SchemaVersion int    `json:"schema_version"`
		AuthorityID   string `json:"authority_id"`
		WorkspaceID   string `json:"workspace_id"`
		ProjectID     string `json:"project_id,omitempty"`
		Registered    bool   `json:"registered"`
		Revision      uint64 `json:"authority_revision"`
		EventHead     string `json:"event_head,omitempty"`
		ActivationID  string `json:"activation_id,omitempty"`
		Enabled       bool   `json:"enabled"`
	}{SchemaVersion, authorityID, workspaceID, projectID, registered, projection.Revision, projection.EventHead, activationID, enabled})
	if err != nil {
		return "", newError(KindRuntime, "ACTIVATION_TOKEN_FAILED", "could not derive the project activation token", err)
	}
	return token, nil
}

func activationTokenForWorkspace(service Service, workspace loadedWorkspace) (string, error) {
	return projectActivationToken(service.authorityID, workspace.WorkspaceID, workspace.ProjectID, true, workspace.Projection)
}

func changeContractDigest(change Change) (string, error) {
	if change.ContractVersion == 0 {
		return digestJSON(struct {
			SchemaVersion      int          `json:"schema_version"`
			ChangeID           string       `json:"change_id"`
			ActivationID       string       `json:"activation_id"`
			Title              string       `json:"title"`
			Goal               string       `json:"goal"`
			Scope              []string     `json:"scope"`
			NonGoals           []string     `json:"non_goals"`
			AcceptanceCriteria []string     `json:"acceptance_criteria"`
			Impact             ChangeImpact `json:"impact"`
			Risk               Risk         `json:"declared_risk"`
		}{change.SchemaVersion, change.ChangeID, change.ActivationID, change.Title, change.Goal, change.Scope, change.NonGoals, change.AcceptanceCriteria, change.Impact, change.DeclaredRisk})
	}
	return digestJSON(struct {
		SchemaVersion       int                 `json:"schema_version"`
		ContractVersion     int                 `json:"contract_version"`
		ChangeID            string              `json:"change_id"`
		ActivationID        string              `json:"activation_id"`
		Title               string              `json:"title"`
		Goal                string              `json:"goal"`
		Scope               []string            `json:"scope"`
		NonGoals            []string            `json:"non_goals"`
		AcceptanceCriteria  []string            `json:"acceptance_criteria"`
		Impact              ChangeImpact        `json:"impact"`
		Requirements        []ChangeRequirement `json:"requirements"`
		SupersedesChangeID  string              `json:"supersedes_change_id,omitempty"`
		LineageRootChangeID string              `json:"lineage_root_change_id,omitempty"`
		Risk                Risk                `json:"declared_risk"`
	}{
		SchemaVersion:       change.SchemaVersion,
		ContractVersion:     change.ContractVersion,
		ChangeID:            change.ChangeID,
		ActivationID:        change.ActivationID,
		Title:               change.Title,
		Goal:                change.Goal,
		Scope:               change.Scope,
		NonGoals:            change.NonGoals,
		AcceptanceCriteria:  change.AcceptanceCriteria,
		Impact:              change.Impact,
		Requirements:        change.Requirements,
		SupersedesChangeID:  change.SupersedesChangeID,
		LineageRootChangeID: change.LineageRootChangeID,
		Risk:                change.DeclaredRisk,
	})
}

func verdictSubjectDigest(verdict Verdict) (string, error) {
	return digestJSON(struct {
		SchemaVersion        int              `json:"schema_version"`
		ProjectID            string           `json:"project_id"`
		AuthorityID          string           `json:"authority_id"`
		WorkspaceID          string           `json:"workspace_id"`
		ChangeID             string           `json:"change_id"`
		ActivationID         string           `json:"activation_id"`
		ContractDigest       string           `json:"contract_digest"`
		ConfigDigest         string           `json:"config_digest"`
		TruthDigest          string           `json:"truth_digest"`
		SemanticAssessmentID string           `json:"semantic_assessment_id"`
		Source               string           `json:"source_fingerprint"`
		EffectiveRisk        Risk             `json:"effective_risk"`
		TouchedPaths         []string         `json:"touched_paths"`
		InferredImpact       InferredImpact   `json:"inferred_impact"`
		GateAssessments      []GateAssessment `json:"gate_assessments"`
	}{
		SchemaVersion:        verdict.SchemaVersion,
		ProjectID:            verdict.ProjectID,
		AuthorityID:          verdict.AuthorityID,
		WorkspaceID:          verdict.WorkspaceID,
		ChangeID:             verdict.ChangeID,
		ActivationID:         verdict.ActivationID,
		ContractDigest:       verdict.ContractDigest,
		ConfigDigest:         verdict.ConfigDigest,
		TruthDigest:          verdict.TruthDigest,
		SemanticAssessmentID: verdict.SemanticAssessmentID,
		Source:               verdict.Source.Fingerprint,
		EffectiveRisk:        verdict.EffectiveRisk,
		TouchedPaths:         verdict.TouchedPaths,
		InferredImpact:       verdict.InferredImpact,
		GateAssessments:      verdict.GateAssessments,
	})
}

// legacyVerdictSubjectDigest preserves the exact v0.2 subject shape so an
// already-completed legacy Change remains replayable after the v0.3 Core adds
// path-derived impact to new Verdict subjects.
func legacyVerdictSubjectDigest(verdict Verdict) (string, error) {
	return digestJSON(struct {
		SchemaVersion        int              `json:"schema_version"`
		ProjectID            string           `json:"project_id"`
		AuthorityID          string           `json:"authority_id"`
		WorkspaceID          string           `json:"workspace_id"`
		ChangeID             string           `json:"change_id"`
		ActivationID         string           `json:"activation_id"`
		ContractDigest       string           `json:"contract_digest"`
		ConfigDigest         string           `json:"config_digest"`
		TruthDigest          string           `json:"truth_digest"`
		SemanticAssessmentID string           `json:"semantic_assessment_id"`
		Source               string           `json:"source_fingerprint"`
		EffectiveRisk        Risk             `json:"effective_risk"`
		TouchedPaths         []string         `json:"touched_paths"`
		GateAssessments      []GateAssessment `json:"gate_assessments"`
	}{
		SchemaVersion:        verdict.SchemaVersion,
		ProjectID:            verdict.ProjectID,
		AuthorityID:          verdict.AuthorityID,
		WorkspaceID:          verdict.WorkspaceID,
		ChangeID:             verdict.ChangeID,
		ActivationID:         verdict.ActivationID,
		ContractDigest:       verdict.ContractDigest,
		ConfigDigest:         verdict.ConfigDigest,
		TruthDigest:          verdict.TruthDigest,
		SemanticAssessmentID: verdict.SemanticAssessmentID,
		Source:               verdict.Source.Fingerprint,
		EffectiveRisk:        verdict.EffectiveRisk,
		TouchedPaths:         verdict.TouchedPaths,
		GateAssessments:      verdict.GateAssessments,
	})
}

func verdictSubjectDigestForChange(change Change, verdict Verdict) (string, error) {
	if change.ContractVersion == 0 {
		return legacyVerdictSubjectDigest(verdict)
	}
	return verdictSubjectDigest(verdict)
}

func (s Service) InitProject(ctx context.Context, start, name string) (ProjectContext, error) {
	service, err := s.withDefaults()
	if err != nil {
		return ProjectContext{}, err
	}
	if err := requireAuthorityPlatform(); err != nil {
		return ProjectContext{}, err
	}
	root, err := service.Git.DiscoverRoot(ctx, start)
	if err != nil {
		return ProjectContext{}, err
	}
	if err := ensureOutsideRoot(root, service.StateDir); err != nil {
		return ProjectContext{}, err
	}
	workspaceID, err := service.Git.WorkspaceID(ctx, root)
	if err != nil {
		return ProjectContext{}, err
	}
	binding, bindingErr := loadWorkspaceBinding(service.StateDir, workspaceID)
	if bindingErr != nil && !isErrorCodeValue(bindingErr, "WORKSPACE_NOT_REGISTERED") {
		return ProjectContext{}, bindingErr
	}
	if bindingErr == nil && binding.AuthorityID != service.authorityID {
		return ProjectContext{}, newError(KindIntegrity, "AUTHORITY_BINDING_MISMATCH", "Workspace binding belongs to a different authority-state location", nil)
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return ProjectContext{}, newError(KindUsage, "INVALID_PROJECT_NAME", "--name is required and must be at most 200 characters", nil)
	}

	configPath := filepath.Join(root, ".ecp")
	_, statErr := os.Lstat(configPath)
	created := false
	initialConfigDigest := ""
	initialTruthDigest := ""
	initialProjectID := ""
	if os.IsNotExist(statErr) {
		if bindingErr == nil {
			return ProjectContext{}, newError(KindIntegrity, "REGISTERED_CONFIG_MISSING", "this Workspace already has authority history but its .ecp config is missing; initialization will not replace it", nil)
		}
		projectID, err := randomID("prj", 12)
		if err != nil {
			return ProjectContext{}, err
		}
		initialProjectID = projectID
		project, policy, gates := DefaultConfig(name, projectID, service.Clock())
		initialConfigDigest, initialTruthDigest, err = writeInitialConfigWithDigest(root, project, policy, gates)
		if err != nil {
			return ProjectContext{}, err
		}
		created = true
	} else if statErr != nil {
		return ProjectContext{}, newError(KindRuntime, "CONFIG_STAT_FAILED", "could not inspect existing .ecp", statErr)
	}

	config, err := LoadConfig(root)
	if err != nil {
		return ProjectContext{}, err
	}
	if created && (config.Digest != initialConfigDigest || config.TruthDigest != initialTruthDigest || config.Project.ProjectID != initialProjectID) {
		return ProjectContext{}, newError(KindConflict, "INITIAL_CONFIG_CHANGED", "the initial .ecp config changed before bootstrap registration", nil)
	}
	if bindingErr != nil {
		if !created {
			return ProjectContext{}, newError(KindBlocked, "PROJECT_REGISTRATION_REQUIRED", "existing .ecp config must be inspected and explicitly registered with its exact candidate digest; project init will not accept it", nil)
		}
		binding = WorkspaceBinding{
			SchemaVersion:       SchemaVersion,
			AuthorityID:         service.authorityID,
			WorkspaceID:         workspaceID,
			ProjectID:           config.Project.ProjectID,
			InitialConfigDigest: config.Digest,
			InitialTruthDigest:  config.TruthDigest,
			Actor:               "bootstrap",
			Reason:              "Core created this initial .ecp config in the same project init operation",
			Trust:               "local-bootstrap-acknowledgement",
			CoreIdentity:        service.CoreIdentity,
			BoundAt:             service.Clock().UTC(),
		}
		if err := createWorkspaceBinding(ctx, service.StateDir, binding); err != nil {
			return ProjectContext{}, err
		}
	}
	if err := service.bootstrapBoundWorkspace(ctx, config, binding); err != nil {
		return ProjectContext{}, err
	}
	return service.Context(ctx, root)
}

func (s Service) InspectProject(ctx context.Context, start string) (ProjectInspection, error) {
	service, err := s.withDefaults()
	if err != nil {
		return ProjectInspection{}, err
	}
	if err := requireAuthorityPlatform(); err != nil {
		return ProjectInspection{}, err
	}
	root, err := service.Git.DiscoverRoot(ctx, start)
	if err != nil {
		return ProjectInspection{}, err
	}
	if err := ensureOutsideRoot(root, service.StateDir); err != nil {
		return ProjectInspection{}, err
	}
	workspaceID, err := service.Git.WorkspaceID(ctx, root)
	if err != nil {
		return ProjectInspection{}, err
	}
	config, err := LoadConfig(root)
	if err != nil {
		return ProjectInspection{}, err
	}
	inspection := ProjectInspection{
		SchemaVersion:         SchemaVersion,
		Root:                  root,
		AuthorityID:           service.authorityID,
		WorkspaceID:           workspaceID,
		CandidateProjectID:    config.Project.ProjectID,
		CandidateConfigDigest: config.Digest,
		CandidateTruthDigest:  config.TruthDigest,
	}
	binding, err := loadWorkspaceBinding(service.StateDir, workspaceID)
	if err == nil {
		if binding.AuthorityID != service.authorityID {
			return ProjectInspection{}, newError(KindIntegrity, "AUTHORITY_BINDING_MISMATCH", "Workspace binding belongs to a different authority-state location", nil)
		}
		inspection.Registered = true
		inspection.BoundProjectID = binding.ProjectID
		inspection.InitialConfigDigest = binding.InitialConfigDigest
		inspection.InitialTruthDigest = binding.InitialTruthDigest
	} else if !isErrorCodeValue(err, "WORKSPACE_NOT_REGISTERED") {
		return ProjectInspection{}, err
	}
	return inspection, nil
}

func (s Service) RegisterProject(ctx context.Context, start, expectedAuthorityID, expectedWorkspaceID, expectedConfigDigest, expectedTruthDigest, actor, reason string) (ProjectContext, error) {
	expectedAuthorityID, err := normalizeExpectedAuthorityID(expectedAuthorityID)
	if err != nil {
		return ProjectContext{}, err
	}
	expectedWorkspaceID, err = normalizeExpectedWorkspaceID(expectedWorkspaceID)
	if err != nil {
		return ProjectContext{}, err
	}
	expectedConfigDigest, err = normalizeExpectedConfigDigest(expectedConfigDigest)
	if err != nil {
		return ProjectContext{}, err
	}
	expectedTruthDigest, err = normalizeExpectedTruthDigest(expectedTruthDigest)
	if err != nil {
		return ProjectContext{}, err
	}
	service, err := s.withDefaults()
	if err != nil {
		return ProjectContext{}, err
	}
	if err := requireAuthorityPlatform(); err != nil {
		return ProjectContext{}, err
	}
	if service.authorityID != expectedAuthorityID {
		return ProjectContext{}, newError(KindConflict, "AUTHORITY_ID_MISMATCH", "the selected authority state no longer matches --authority", nil)
	}
	root, err := service.Git.DiscoverRoot(ctx, start)
	if err != nil {
		return ProjectContext{}, err
	}
	if err := ensureOutsideRoot(root, service.StateDir); err != nil {
		return ProjectContext{}, err
	}
	actor = strings.TrimSpace(actor)
	reason = strings.TrimSpace(reason)
	if actor == "" || reason == "" {
		return ProjectContext{}, newError(KindUsage, "ACK_FIELDS_REQUIRED", "--actor and --reason are required", nil)
	}
	if len(actor) > 200 || len(reason) > 2000 {
		return ProjectContext{}, newError(KindUsage, "ACK_FIELDS_TOO_LONG", "actor or reason exceeds the supported limit", nil)
	}
	workspaceID, err := service.Git.WorkspaceID(ctx, root)
	if err != nil {
		return ProjectContext{}, err
	}
	if workspaceID != expectedWorkspaceID {
		return ProjectContext{}, newError(KindConflict, "WORKSPACE_ID_MISMATCH", "the target Workspace no longer matches --workspace", nil)
	}
	config, err := LoadConfig(root)
	if err != nil {
		return ProjectContext{}, err
	}
	if config.Digest != expectedConfigDigest {
		return ProjectContext{}, newError(KindConflict, "CONFIG_DIGEST_MISMATCH", "the current candidate config no longer matches --config-digest", nil)
	}
	if config.TruthDigest != expectedTruthDigest {
		return ProjectContext{}, newError(KindConflict, "TRUTH_DIGEST_MISMATCH", "the current candidate Project Truth no longer matches --truth-digest", nil)
	}
	if _, err := loadWorkspaceBinding(service.StateDir, workspaceID); err == nil {
		return ProjectContext{}, newError(KindConflict, "WORKSPACE_ALREADY_REGISTERED", "this Workspace is already registered; use project status", nil)
	} else if !isErrorCodeValue(err, "WORKSPACE_NOT_REGISTERED") {
		return ProjectContext{}, err
	}
	binding := WorkspaceBinding{
		SchemaVersion:       SchemaVersion,
		AuthorityID:         service.authorityID,
		WorkspaceID:         workspaceID,
		ProjectID:           config.Project.ProjectID,
		InitialConfigDigest: config.Digest,
		InitialTruthDigest:  config.TruthDigest,
		Actor:               actor,
		Reason:              reason,
		Trust:               "local-registration-acknowledgement",
		CoreIdentity:        service.CoreIdentity,
		BoundAt:             service.Clock().UTC(),
	}
	if err := createWorkspaceBinding(ctx, service.StateDir, binding); err != nil {
		return ProjectContext{}, err
	}
	if err := service.bootstrapBoundWorkspace(ctx, config, binding); err != nil {
		return ProjectContext{}, err
	}
	return service.Context(ctx, root)
}

func (s Service) bootstrapBoundWorkspace(ctx context.Context, config ConfigBundle, binding WorkspaceBinding) error {
	store, err := NewStore(s.StateDir, binding.ProjectID, binding.WorkspaceID, 5*time.Second)
	if err != nil {
		return err
	}
	if store.Exists() {
		_, err := store.Load(ctx)
		return err
	}
	if binding.AuthorityID != s.authorityID || binding.ProjectID != config.Project.ProjectID || binding.InitialConfigDigest != config.Digest || binding.InitialTruthDigest != config.TruthDigest {
		return newError(KindBlocked, "INCOMPLETE_BOOTSTRAP_CONFIG_DRIFT", "Workspace binding exists but the config changed before registration completed; restore the bound initial digest", nil)
	}
	registration := ProjectRegistration{
		SchemaVersion: SchemaVersion,
		ProjectID:     binding.ProjectID,
		AuthorityID:   binding.AuthorityID,
		WorkspaceID:   binding.WorkspaceID,
		RegisteredAt:  s.Clock().UTC(),
		CoreVersion:   CoreVersion,
		CoreIdentity:  s.CoreIdentity,
	}
	acceptance := ConfigAcceptance{
		SchemaVersion: SchemaVersion,
		ConfigDigest:  binding.InitialConfigDigest,
		Project:       config.Project,
		Policy:        config.Policy,
		Gates:         config.Gates,
		Actor:         binding.Actor,
		Reason:        binding.Reason,
		AcceptedAt:    s.Clock().UTC(),
		Trust:         binding.Trust,
		CoreIdentity:  s.CoreIdentity,
	}
	truthAcceptance := ProjectTruthAcceptance{
		SchemaVersion: SchemaVersion,
		TruthDigest:   config.TruthDigest,
		Truth:         config.Truth,
		Files:         config.TruthFiles,
		Actor:         binding.Actor,
		Reason:        binding.Reason,
		AcceptedAt:    s.Clock().UTC(),
		Trust:         binding.Trust,
		CoreIdentity:  s.CoreIdentity,
	}
	if err := store.WriteTruthBlobs(config.TruthContent); err != nil {
		return err
	}
	_, err = store.Append(ctx, nil,
		PendingEvent{Type: "project_registered", Origin: "cli", Payload: registration},
		PendingEvent{Type: "config_accepted", Origin: "cli", Payload: acceptance},
		PendingEvent{Type: "project_truth_accepted", Origin: "cli", Payload: truthAcceptance},
	)
	return err
}

// EnableProject activates ECP for the exact registered Workspace and accepted
// config observed by the caller. The activation lives in the authority event
// store; repository files cannot enable themselves.
func (s Service) EnableProject(ctx context.Context, start, expectedAuthorityID, expectedWorkspaceID, expectedActivationToken, expectedConfigDigest, expectedTruthDigest, actor, reason string) (ProjectStatus, error) {
	expectedAuthorityID, err := normalizeExpectedAuthorityID(expectedAuthorityID)
	if err != nil {
		return ProjectStatus{}, err
	}
	expectedWorkspaceID, err = normalizeExpectedWorkspaceID(expectedWorkspaceID)
	if err != nil {
		return ProjectStatus{}, err
	}
	expectedActivationToken, err = normalizeExpectedActivationToken(expectedActivationToken)
	if err != nil {
		return ProjectStatus{}, err
	}
	expectedConfigDigest, err = normalizeExpectedConfigDigest(expectedConfigDigest)
	if err != nil {
		return ProjectStatus{}, err
	}
	expectedTruthDigest, err = normalizeExpectedTruthDigest(expectedTruthDigest)
	if err != nil {
		return ProjectStatus{}, err
	}
	actor, reason, err = normalizeProjectActivationFields(actor, reason)
	if err != nil {
		return ProjectStatus{}, err
	}
	service, workspace, err := s.load(ctx, start)
	if err != nil {
		return ProjectStatus{}, err
	}
	if err := requireExpectedAuthorityWorkspace(service, workspace, expectedAuthorityID, expectedWorkspaceID); err != nil {
		return ProjectStatus{}, err
	}
	if err := requireExpectedActivationToken(service, workspace, expectedActivationToken); err != nil {
		return ProjectStatus{}, err
	}
	release, err := workspace.Store.AcquireGateLease(ctx)
	if err != nil {
		return ProjectStatus{}, err
	}
	defer release()
	service, workspace, err = service.load(ctx, workspace.Root)
	if err != nil {
		return ProjectStatus{}, err
	}
	if err := requireExpectedAuthorityWorkspace(service, workspace, expectedAuthorityID, expectedWorkspaceID); err != nil {
		return ProjectStatus{}, err
	}
	if err := requireExpectedActivationToken(service, workspace, expectedActivationToken); err != nil {
		return ProjectStatus{}, err
	}
	workspace, err = recoverInterruptedGateRun(ctx, service, workspace, "project enable acquired the released Gate sequence lease")
	if err != nil {
		return ProjectStatus{}, err
	}
	if err := requireAcceptedConfig(workspace); err != nil {
		return ProjectStatus{}, err
	}
	if err := requireAcceptedTruth(workspace); err != nil {
		return ProjectStatus{}, err
	}
	if workspace.Config.Digest != expectedConfigDigest {
		return ProjectStatus{}, newError(KindConflict, "CONFIG_DIGEST_MISMATCH", "the current accepted config no longer matches --config-digest", nil)
	}
	if workspace.Config.TruthDigest != expectedTruthDigest {
		return ProjectStatus{}, newError(KindConflict, "TRUTH_DIGEST_MISMATCH", "the current accepted Project Truth no longer matches --truth-digest", nil)
	}
	required := requiredGates(workspace.Config.Gates, workspace.Config.Policy.DefaultRisk)
	if len(required) == 0 {
		return ProjectStatus{}, newError(KindBlocked, "NO_REQUIRED_GATES", "the accepted config must define a required Gate for the default risk before ECP can be enabled", nil)
	}
	for _, gate := range required {
		if _, err := resolveGateExecutionContext(workspace.Root, workspace.Config.Policy, gate); err != nil {
			return ProjectStatus{}, newError(KindBlocked, "GATE_EXECUTION_CONTEXT_UNAVAILABLE", fmt.Sprintf("required Gate %q cannot run in the current Workspace", gate.ID), err)
		}
	}
	if workspace.Projection.ECPEnabled() {
		return service.ProjectStatus(ctx, workspace.Root)
	}
	recheck, err := LoadConfig(workspace.Root)
	if err != nil {
		return ProjectStatus{}, err
	}
	if recheck.Digest != expectedConfigDigest || recheck.TruthDigest != expectedTruthDigest {
		return ProjectStatus{}, newError(KindConflict, "CONFIG_DIGEST_MISMATCH", "the candidate config changed while ECP enablement was being prepared", nil)
	}
	activationID, err := randomID("act", 12)
	if err != nil {
		return ProjectStatus{}, err
	}
	previousActivation := ""
	if workspace.Projection.Activation != nil {
		previousActivation = workspace.Projection.Activation.ActivationID
	}
	activation := ProjectActivation{
		SchemaVersion:      SchemaVersion,
		ActivationID:       activationID,
		PreviousActivation: previousActivation,
		Enabled:            true,
		ProjectID:          workspace.ProjectID,
		AuthorityID:        service.authorityID,
		WorkspaceID:        workspace.WorkspaceID,
		ConfigDigest:       workspace.Config.Digest,
		TruthDigest:        workspace.Config.TruthDigest,
		Actor:              actor,
		Reason:             reason,
		Trust:              "local-project-enablement-acknowledgement",
		CoreIdentity:       service.CoreIdentity,
		ChangedAt:          service.Clock().UTC(),
	}
	revision := workspace.Projection.Revision
	if _, err := workspace.Store.Append(ctx, &revision, PendingEvent{Type: "project_enabled", Origin: "cli", Payload: activation}); err != nil {
		return ProjectStatus{}, err
	}
	return service.ProjectStatus(ctx, workspace.Root)
}

// DisableProject is an audited, project-level operation. It never rewrites the
// worktree or deletes history. If a Change is active, cancellation and
// disablement are appended atomically under the terminal lease.
func (s Service) DisableProject(ctx context.Context, start, expectedAuthorityID, expectedWorkspaceID, expectedActivationToken, actor, reason string) (ProjectStatus, error) {
	expectedAuthorityID, err := normalizeExpectedAuthorityID(expectedAuthorityID)
	if err != nil {
		return ProjectStatus{}, err
	}
	expectedWorkspaceID, err = normalizeExpectedWorkspaceID(expectedWorkspaceID)
	if err != nil {
		return ProjectStatus{}, err
	}
	expectedActivationToken, err = normalizeExpectedActivationToken(expectedActivationToken)
	if err != nil {
		return ProjectStatus{}, err
	}
	actor, reason, err = normalizeProjectActivationFields(actor, reason)
	if err != nil {
		return ProjectStatus{}, err
	}
	service, err := s.withDefaults()
	if err != nil {
		return ProjectStatus{}, err
	}
	if err := requireAuthorityPlatform(); err != nil {
		return ProjectStatus{}, err
	}
	if service.authorityID != expectedAuthorityID {
		return ProjectStatus{}, newError(KindConflict, "AUTHORITY_ID_MISMATCH", "the selected authority state no longer matches --authority", nil)
	}
	root, err := service.Git.DiscoverRoot(ctx, start)
	if err != nil {
		return ProjectStatus{}, err
	}
	if err := ensureOutsideRoot(root, service.StateDir); err != nil {
		return ProjectStatus{}, err
	}
	workspaceID, err := service.Git.WorkspaceID(ctx, root)
	if err != nil {
		return ProjectStatus{}, err
	}
	if workspaceID != expectedWorkspaceID {
		return ProjectStatus{}, newError(KindConflict, "WORKSPACE_ID_MISMATCH", "the target Workspace no longer matches --workspace", nil)
	}
	if _, err := loadWorkspaceBinding(service.StateDir, workspaceID); err != nil {
		if isErrorCodeValue(err, "WORKSPACE_NOT_REGISTERED") {
			token, tokenErr := projectActivationToken(service.authorityID, workspaceID, "", false, NewProjection())
			if tokenErr != nil {
				return ProjectStatus{}, tokenErr
			}
			if token != expectedActivationToken {
				return ProjectStatus{}, newError(KindConflict, "ACTIVATION_TOKEN_MISMATCH", "the project activation state no longer matches --activation-token", nil)
			}
			finalStatus, statusErr := service.ProjectStatus(ctx, root)
			if statusErr != nil {
				return ProjectStatus{}, statusErr
			}
			if finalStatus.ActivationToken != expectedActivationToken || finalStatus.Enabled {
				return ProjectStatus{}, newError(KindConflict, "ACTIVATION_TOKEN_MISMATCH", "the project activation state changed while disabled mode was being confirmed", nil)
			}
			return finalStatus, nil
		}
		return ProjectStatus{}, err
	}
	service, workspace, err := service.loadAuthority(ctx, root)
	if err != nil {
		return ProjectStatus{}, err
	}
	if err := requireExpectedAuthorityWorkspace(service, workspace, expectedAuthorityID, expectedWorkspaceID); err != nil {
		return ProjectStatus{}, err
	}
	if err := requireExpectedActivationToken(service, workspace, expectedActivationToken); err != nil {
		return ProjectStatus{}, err
	}
	release, err := workspace.Store.AcquireGateLease(ctx)
	if err != nil {
		return ProjectStatus{}, err
	}
	defer release()
	service, workspace, err = service.loadAuthority(ctx, workspace.Root)
	if err != nil {
		return ProjectStatus{}, err
	}
	if err := requireExpectedAuthorityWorkspace(service, workspace, expectedAuthorityID, expectedWorkspaceID); err != nil {
		return ProjectStatus{}, err
	}
	if err := requireExpectedActivationToken(service, workspace, expectedActivationToken); err != nil {
		return ProjectStatus{}, err
	}
	workspace, err = recoverInterruptedGateRun(ctx, service, workspace, "project disable acquired the released Gate sequence lease")
	if err != nil {
		return ProjectStatus{}, err
	}
	active := workspace.Projection.ActiveChange()
	if !workspace.Projection.ECPEnabled() && active == nil {
		return service.ProjectStatus(ctx, workspace.Root)
	}
	pending := make([]PendingEvent, 0, 2)
	currentActivationID := ""
	if workspace.Projection.Activation != nil {
		currentActivationID = workspace.Projection.Activation.ActivationID
	}
	if active != nil {
		pending = append(pending, PendingEvent{Type: "change_cancelled", Origin: "cli", Payload: ChangeCancellation{
			SchemaVersion: SchemaVersion,
			ChangeID:      active.ChangeID,
			ActivationID:  active.ActivationID,
			Actor:         actor,
			Reason:        reason,
			Trust:         "local-project-disablement-acknowledgement",
			CancelledAt:   service.Clock().UTC(),
		}})
	}
	if workspace.Projection.ECPEnabled() {
		activationID, activationErr := randomID("act", 12)
		if activationErr != nil {
			return ProjectStatus{}, activationErr
		}
		configDigest := ""
		truthDigest := ""
		if workspace.Projection.AcceptedConfig != nil {
			configDigest = workspace.Projection.AcceptedConfig.ConfigDigest
		}
		if workspace.Projection.AcceptedTruth != nil {
			truthDigest = workspace.Projection.AcceptedTruth.TruthDigest
		}
		pending = append(pending, PendingEvent{Type: "project_disabled", Origin: "cli", Payload: ProjectActivation{
			SchemaVersion:      SchemaVersion,
			ActivationID:       activationID,
			PreviousActivation: currentActivationID,
			Enabled:            false,
			ProjectID:          workspace.ProjectID,
			AuthorityID:        service.authorityID,
			WorkspaceID:        workspace.WorkspaceID,
			ConfigDigest:       configDigest,
			TruthDigest:        truthDigest,
			Actor:              actor,
			Reason:             reason,
			Trust:              "local-project-disablement-acknowledgement",
			CoreIdentity:       service.CoreIdentity,
			ChangedAt:          service.Clock().UTC(),
		}})
	}
	revision := workspace.Projection.Revision
	if _, err := workspace.Store.Append(ctx, &revision, pending...); err != nil {
		return ProjectStatus{}, err
	}
	return service.ProjectStatus(ctx, workspace.Root)
}

func normalizeProjectActivationFields(actor, reason string) (string, string, error) {
	actor = strings.TrimSpace(actor)
	reason = strings.TrimSpace(reason)
	if actor == "" || reason == "" {
		return "", "", newError(KindUsage, "PROJECT_ACTIVATION_FIELDS_REQUIRED", "--actor and --reason are required", nil)
	}
	if len(actor) > 200 || len(reason) > 2000 {
		return "", "", newError(KindUsage, "PROJECT_ACTIVATION_FIELDS_TOO_LONG", "actor or reason exceeds the supported limit", nil)
	}
	return actor, reason, nil
}

func (s Service) AcceptPolicy(ctx context.Context, start, expectedAuthorityID, expectedWorkspaceID, expectedConfigDigest, actor, reason string) (ConfigAcceptance, error) {
	expectedAuthorityID, err := normalizeExpectedAuthorityID(expectedAuthorityID)
	if err != nil {
		return ConfigAcceptance{}, err
	}
	expectedWorkspaceID, err = normalizeExpectedWorkspaceID(expectedWorkspaceID)
	if err != nil {
		return ConfigAcceptance{}, err
	}
	expectedConfigDigest, err = normalizeExpectedConfigDigest(expectedConfigDigest)
	if err != nil {
		return ConfigAcceptance{}, err
	}
	service, workspace, err := s.load(ctx, start)
	if err != nil {
		return ConfigAcceptance{}, err
	}
	if err := requireExpectedAuthorityWorkspace(service, workspace, expectedAuthorityID, expectedWorkspaceID); err != nil {
		return ConfigAcceptance{}, err
	}
	actor = strings.TrimSpace(actor)
	reason = strings.TrimSpace(reason)
	if actor == "" || reason == "" {
		return ConfigAcceptance{}, newError(KindUsage, "ACK_FIELDS_REQUIRED", "--actor and --reason are required", nil)
	}
	if len(actor) > 200 || len(reason) > 2000 {
		return ConfigAcceptance{}, newError(KindUsage, "ACK_FIELDS_TOO_LONG", "actor or reason exceeds the supported limit", nil)
	}
	if workspace.Config.Digest != expectedConfigDigest {
		return ConfigAcceptance{}, newError(KindConflict, "CONFIG_DIGEST_MISMATCH", "the current candidate config no longer matches --config-digest", nil)
	}
	if workspace.Projection.ActiveChange() != nil {
		return ConfigAcceptance{}, newError(KindBlocked, "ACTIVE_CHANGE_POLICY_DRIFT", "policy cannot be accepted while a Change is active; revert the draft config or finish the existing Change", nil)
	}
	if workspace.Config.Project.ProjectID != workspace.ProjectID {
		return ConfigAcceptance{}, newError(KindBlocked, "PROJECT_ID_IMMUTABLE", "draft project_id differs from the Workspace authority binding and cannot be accepted", nil)
	}
	if workspace.Projection.AcceptedTruth != nil {
		if err := validateProjectTruth(workspace.Projection.AcceptedTruth.Truth, workspace.Config.Gates); err != nil {
			return ConfigAcceptance{}, newError(KindBlocked, "CONFIG_BREAKS_ACCEPTED_TRUTH", "candidate control config invalidates the previously accepted Project Truth", err)
		}
	}
	acceptance := ConfigAcceptance{
		SchemaVersion: SchemaVersion,
		ConfigDigest:  workspace.Config.Digest,
		Project:       workspace.Config.Project,
		Policy:        workspace.Config.Policy,
		Gates:         workspace.Config.Gates,
		Actor:         actor,
		Reason:        reason,
		AcceptedAt:    service.Clock().UTC(),
		Trust:         "local-acknowledgement",
		CoreIdentity:  service.CoreIdentity,
	}
	revision := workspace.Projection.Revision
	if _, err := workspace.Store.Append(ctx, &revision, PendingEvent{Type: "config_accepted", Origin: "cli", Payload: acceptance}); err != nil {
		return ConfigAcceptance{}, err
	}
	return acceptance, nil
}

// TruthDiff compares the candidate repository Project Truth with the last
// accepted authority epoch. It is read-only and never treats the candidate as
// authoritative merely because it parses successfully.
func (s Service) TruthDiff(ctx context.Context, start string) (TruthDiffReport, error) {
	service, workspace, err := s.load(ctx, start)
	if err != nil {
		return TruthDiffReport{}, err
	}
	if err := requireECPEnabled(workspace); err != nil {
		return TruthDiffReport{}, err
	}
	if err := requireAcceptedConfig(workspace); err != nil {
		return TruthDiffReport{}, err
	}
	change := workspace.Projection.ActiveChange()
	if change == nil {
		return TruthDiffReport{}, newError(KindBlocked, "NO_ACTIVE_CHANGE", "Project Truth can only evolve inside an active Change", nil)
	}
	if workspace.Projection.AcceptedTruth == nil {
		return TruthDiffReport{}, newError(KindIntegrity, "PROJECT_TRUTH_NOT_ACCEPTED", "Workspace has no accepted Project Truth epoch", nil)
	}
	delta, err := computeTruthDelta(*workspace.Projection.AcceptedTruth, workspace.Config)
	if err != nil {
		return TruthDiffReport{}, err
	}
	protected := false
	for _, item := range delta {
		protected = protected || item.Protected
	}
	return TruthDiffReport{
		SchemaVersion:        SchemaVersion,
		ProjectID:            workspace.ProjectID,
		AuthorityID:          service.authorityID,
		WorkspaceID:          workspace.WorkspaceID,
		ChangeID:             change.ChangeID,
		PreviousTruthDigest:  workspace.Projection.AcceptedTruth.TruthDigest,
		CandidateTruthDigest: workspace.Config.TruthDigest,
		Changed:              workspace.Projection.AcceptedTruth.TruthDigest != workspace.Config.TruthDigest,
		ProtectedChange:      protected,
		Delta:                delta,
	}, nil
}

// AssessSemantic records the required semantic reconciliation for the current
// source fingerprint. When behavior intentionally changes, acceptance of the
// exact candidate Project Truth is appended atomically after the assessment.
func (s Service) AssessSemantic(ctx context.Context, start string, input SemanticAssessmentInput) (SemanticAssessment, error) {
	expectedAuthority, err := normalizeExpectedAuthorityID(input.ExpectedAuthority)
	if err != nil {
		return SemanticAssessment{}, err
	}
	expectedWorkspace, err := normalizeExpectedWorkspaceID(input.ExpectedWorkspace)
	if err != nil {
		return SemanticAssessment{}, err
	}
	expectedActivation, err := normalizeExpectedActivationToken(input.ExpectedActivation)
	if err != nil {
		return SemanticAssessment{}, err
	}
	expectedChangeID, err := normalizeExpectedChangeID(input.ExpectedChangeID)
	if err != nil {
		return SemanticAssessment{}, err
	}
	expectedSource, err := normalizeExpectedSourceFingerprint(input.ExpectedSource)
	if err != nil {
		return SemanticAssessment{}, err
	}
	expectedPreviousTruth, err := normalizeExpectedTruthDigest(input.ExpectedPreviousTruth)
	if err != nil {
		return SemanticAssessment{}, err
	}
	expectedCandidateTruth, err := normalizeExpectedTruthDigest(input.ExpectedCandidateTruth)
	if err != nil {
		return SemanticAssessment{}, err
	}
	input.Summary = strings.TrimSpace(input.Summary)
	input.Actor = strings.TrimSpace(input.Actor)
	input.Reason = strings.TrimSpace(input.Reason)
	input.Categories, err = normalizeTextList(input.Categories, true)
	if err != nil {
		return SemanticAssessment{}, err
	}
	if input.Summary == "" || input.Actor == "" || input.Reason == "" {
		return SemanticAssessment{}, newError(KindUsage, "SEMANTIC_FIELDS_REQUIRED", "--summary, --category, --actor, and --reason are required", nil)
	}
	if len(input.Summary) > 4000 || len(input.Actor) > 200 || len(input.Reason) > 4000 {
		return SemanticAssessment{}, newError(KindUsage, "SEMANTIC_FIELDS_TOO_LONG", "semantic assessment fields exceed supported limits", nil)
	}

	service, workspace, err := s.load(ctx, start)
	if err != nil {
		return SemanticAssessment{}, err
	}
	if err := requireExpectedAuthorityWorkspace(service, workspace, expectedAuthority, expectedWorkspace); err != nil {
		return SemanticAssessment{}, err
	}
	if err := requireExpectedActivationToken(service, workspace, expectedActivation); err != nil {
		return SemanticAssessment{}, err
	}
	release, err := workspace.Store.AcquireGateLease(ctx)
	if err != nil {
		return SemanticAssessment{}, err
	}
	defer release()
	service, workspace, err = service.load(ctx, workspace.Root)
	if err != nil {
		return SemanticAssessment{}, err
	}
	if err := requireExpectedAuthorityWorkspace(service, workspace, expectedAuthority, expectedWorkspace); err != nil {
		return SemanticAssessment{}, err
	}
	if err := requireExpectedActivationToken(service, workspace, expectedActivation); err != nil {
		return SemanticAssessment{}, err
	}
	if err := requireECPEnabled(workspace); err != nil {
		return SemanticAssessment{}, err
	}
	workspace, err = recoverInterruptedGateRun(ctx, service, workspace, "semantic reconciliation acquired the released Gate sequence lease")
	if err != nil {
		return SemanticAssessment{}, err
	}
	if err := requireAcceptedConfig(workspace); err != nil {
		return SemanticAssessment{}, err
	}
	change, err := requireExpectedActiveChange(workspace.Projection, expectedChangeID)
	if err != nil {
		return SemanticAssessment{}, err
	}
	if err := validateRequirementAssessments(*change, input.RequirementAssessments); err != nil {
		return SemanticAssessment{}, err
	}
	accepted := workspace.Projection.AcceptedTruth
	if accepted == nil || accepted.TruthDigest != expectedPreviousTruth {
		return SemanticAssessment{}, newError(KindConflict, "PREVIOUS_TRUTH_DIGEST_MISMATCH", "the accepted Project Truth no longer matches --previous-truth-digest", nil)
	}
	if workspace.Config.TruthDigest != expectedCandidateTruth {
		return SemanticAssessment{}, newError(KindConflict, "TRUTH_DIGEST_MISMATCH", "the candidate Project Truth no longer matches --candidate-truth-digest", nil)
	}
	current, err := service.Git.Snapshot(ctx, workspace.Root, workspace.effectiveConfig().Policy)
	if err != nil {
		return SemanticAssessment{}, err
	}
	if current.Fingerprint != expectedSource {
		return SemanticAssessment{}, newError(KindConflict, "SOURCE_FINGERPRINT_MISMATCH", "the current source no longer matches --source-fingerprint", nil)
	}
	delta, err := computeTruthDelta(*accepted, workspace.Config)
	if err != nil {
		return SemanticAssessment{}, err
	}
	if err := validateTruthDeltaAgainstImpact(delta, change.Impact, accepted.Truth, workspace.Config.Truth); err != nil {
		return SemanticAssessment{}, err
	}
	if input.Behavior == SemanticBehaviorPreserved && len(change.Impact.ExpectedChanges) > 0 {
		return SemanticAssessment{}, newError(KindBlocked, "EXPECTED_SEMANTIC_CHANGE_NOT_RECONCILED", "the Change declared expected semantic changes and cannot be reconciled as PRESERVED", nil)
	}
	if input.Behavior == SemanticBehaviorChanged && len(change.Impact.ExpectedChanges) == 0 && len(change.Impact.Unknowns) == 0 {
		return SemanticAssessment{}, newError(KindBlocked, "UNEXPECTED_SEMANTIC_CHANGE", "CHANGED must be covered by an expected change or a concrete startup uncertainty in the Change impact", nil)
	}
	protected := false
	for _, item := range delta {
		protected = protected || item.Protected
	}
	if protected && input.Behavior == SemanticBehaviorChanged && !input.ConfirmProtected {
		return SemanticAssessment{}, newError(KindBlocked, "PROTECTED_TRUTH_CONFIRMATION_REQUIRED", "protected Project Truth changed; explicit product-language confirmation is required before accepting the candidate truth", nil)
	}
	assessmentID, err := randomID("sem", 12)
	if err != nil {
		return SemanticAssessment{}, err
	}
	assessment := SemanticAssessment{
		SchemaVersion:          SchemaVersion,
		AssessmentID:           assessmentID,
		ChangeID:               change.ChangeID,
		ActivationID:           change.ActivationID,
		PreviousTruthDigest:    accepted.TruthDigest,
		CurrentTruthDigest:     workspace.Config.TruthDigest,
		SourceFingerprint:      current.Fingerprint,
		Behavior:               input.Behavior,
		Summary:                input.Summary,
		Categories:             input.Categories,
		RequirementAssessments: input.RequirementAssessments,
		TruthDelta:             delta,
		ProtectedChange:        protected,
		ProtectedConfirmed:     protected && input.Behavior == SemanticBehaviorChanged && input.ConfirmProtected,
		Actor:                  input.Actor,
		Reason:                 input.Reason,
		RecordedAt:             service.Clock().UTC(),
		Trust:                  "local-semantic-reconciliation",
	}
	if err := validateSemanticAssessment(assessment, accepted.TruthDigest); err != nil {
		return SemanticAssessment{}, err
	}
	if err := validateRequirementAssessments(*change, assessment.RequirementAssessments); err != nil {
		return SemanticAssessment{}, err
	}
	pending := []PendingEvent{{Type: "semantic_assessed", Origin: "cli", Payload: assessment}}
	if assessment.Behavior == SemanticBehaviorChanged && assessment.CurrentTruthDigest != accepted.TruthDigest {
		if err := workspace.Store.WriteTruthBlobs(workspace.Config.TruthContent); err != nil {
			return SemanticAssessment{}, err
		}
		pending = append(pending, PendingEvent{Type: "project_truth_accepted", Origin: "cli", Payload: ProjectTruthAcceptance{
			SchemaVersion:       SchemaVersion,
			TruthDigest:         workspace.Config.TruthDigest,
			PreviousTruthDigest: accepted.TruthDigest,
			ChangeID:            change.ChangeID,
			Truth:               workspace.Config.Truth,
			Files:               workspace.Config.TruthFiles,
			Actor:               input.Actor,
			Reason:              input.Reason,
			AcceptedAt:          service.Clock().UTC(),
			Trust:               "local-protected-truth-acknowledgement",
			CoreIdentity:        service.CoreIdentity,
		}})
	}
	revision := workspace.Projection.Revision
	if _, err := workspace.Store.Append(ctx, &revision, pending...); err != nil {
		return SemanticAssessment{}, err
	}
	return assessment, nil
}

// AcceptedProjectTruth returns the exact authority-backed truth epoch and raw
// contract contents. It uses the authority-only path so a future maintainer can
// recover the last accepted facts even when the repository candidate is
// malformed or has drifted.
func (s Service) AcceptedProjectTruth(ctx context.Context, start string) (ProjectTruthView, error) {
	return s.AcceptedProjectTruthRevision(ctx, start, "")
}

// AcceptedControlConfigRevision returns the latest or one exact historical
// parsed control epoch through the authority-only path. This gives an adapter a
// trustworthy comparison/recovery source when candidate policy is drifted or
// malformed without letting the accepted payload rewrite itself.
func (s Service) AcceptedControlConfigRevision(ctx context.Context, start, digest string) (ControlConfigView, error) {
	service, workspace, err := s.loadAuthority(ctx, start)
	if err != nil {
		return ControlConfigView{}, err
	}
	accepted := workspace.Projection.AcceptedConfig
	digest = strings.TrimSpace(digest)
	if digest != "" {
		if !isSHA256Digest(digest) {
			return ControlConfigView{}, newError(KindUsage, "INVALID_CONFIG_DIGEST", "--digest must be a complete accepted sha256 config digest", nil)
		}
		accepted = nil
		for i := len(workspace.Projection.ConfigHistory) - 1; i >= 0; i-- {
			if workspace.Projection.ConfigHistory[i].ConfigDigest == digest {
				copyOfAcceptance := workspace.Projection.ConfigHistory[i]
				accepted = &copyOfAcceptance
				break
			}
		}
	}
	if accepted == nil {
		return ControlConfigView{}, newError(KindNotFound, "CONFIG_NOT_ACCEPTED", "Workspace has no matching accepted Control Config epoch", nil)
	}
	return ControlConfigView{
		SchemaVersion: SchemaVersion,
		ProjectID:     workspace.ProjectID,
		AuthorityID:   service.authorityID,
		WorkspaceID:   workspace.WorkspaceID,
		ConfigDigest:  accepted.ConfigDigest,
		Project:       accepted.Project,
		Policy:        accepted.Policy,
		Gates:         accepted.Gates,
		Actor:         accepted.Actor,
		Reason:        accepted.Reason,
		AcceptedAt:    accepted.AcceptedAt,
		Trust:         accepted.Trust,
	}, nil
}

// AcceptedProjectTruthRevision returns the latest accepted truth when digest is
// empty, or one exact historical accepted epoch when a complete digest is
// supplied. Candidate repository content is never consulted.
func (s Service) AcceptedProjectTruthRevision(ctx context.Context, start, digest string) (ProjectTruthView, error) {
	service, workspace, err := s.loadAuthority(ctx, start)
	if err != nil {
		return ProjectTruthView{}, err
	}
	accepted := workspace.Projection.AcceptedTruth
	digest = strings.TrimSpace(digest)
	if digest != "" {
		if !isSHA256Digest(digest) {
			return ProjectTruthView{}, newError(KindUsage, "INVALID_TRUTH_DIGEST", "--digest must be a complete accepted sha256 truth digest", nil)
		}
		accepted = nil
		for i := len(workspace.Projection.TruthHistory) - 1; i >= 0; i-- {
			if workspace.Projection.TruthHistory[i].TruthDigest == digest {
				copyOfAcceptance := workspace.Projection.TruthHistory[i]
				accepted = &copyOfAcceptance
				break
			}
		}
	}
	if accepted == nil {
		return ProjectTruthView{}, newError(KindNotFound, "PROJECT_TRUTH_NOT_ACCEPTED", "Workspace has no matching accepted Project Truth epoch", nil)
	}
	files, err := workspace.Store.ReadTruthFiles(accepted.Files)
	if err != nil {
		return ProjectTruthView{}, err
	}
	return ProjectTruthView{
		SchemaVersion: SchemaVersion,
		ProjectID:     workspace.ProjectID,
		AuthorityID:   service.authorityID,
		WorkspaceID:   workspace.WorkspaceID,
		TruthDigest:   accepted.TruthDigest,
		Truth:         accepted.Truth,
		Files:         files,
		Actor:         accepted.Actor,
		Reason:        accepted.Reason,
		AcceptedAt:    accepted.AcceptedAt,
		ChangeID:      accepted.ChangeID,
		Trust:         accepted.Trust,
	}, nil
}

func (s Service) StartChange(ctx context.Context, start string, input StartChangeInput) (Change, error) {
	expectedAuthority, err := normalizeExpectedAuthorityID(input.ExpectedAuthority)
	if err != nil {
		return Change{}, err
	}
	expectedWorkspace, err := normalizeExpectedWorkspaceID(input.ExpectedWorkspace)
	if err != nil {
		return Change{}, err
	}
	expectedActivation, err := normalizeExpectedActivationToken(input.ExpectedActivation)
	if err != nil {
		return Change{}, err
	}
	expectedConfig, err := normalizeExpectedConfigDigest(input.ExpectedConfig)
	if err != nil {
		return Change{}, err
	}
	expectedTruth, err := normalizeExpectedTruthDigest(input.ExpectedTruth)
	if err != nil {
		return Change{}, err
	}
	expectedSource, err := normalizeExpectedSourceFingerprint(input.ExpectedSource)
	if err != nil {
		return Change{}, err
	}
	service, workspace, err := s.load(ctx, start)
	if err != nil {
		return Change{}, err
	}
	if service.authorityID != expectedAuthority {
		return Change{}, newError(KindConflict, "AUTHORITY_ID_MISMATCH", "the selected authority state no longer matches --authority", nil)
	}
	if workspace.WorkspaceID != expectedWorkspace {
		return Change{}, newError(KindConflict, "WORKSPACE_ID_MISMATCH", "the target Workspace no longer matches --workspace", nil)
	}
	if err := requireECPEnabled(workspace); err != nil {
		return Change{}, err
	}
	if err := requireExpectedActivationToken(service, workspace, expectedActivation); err != nil {
		return Change{}, err
	}
	if err := requireAcceptedConfig(workspace); err != nil {
		return Change{}, err
	}
	if workspace.Config.Digest != expectedConfig {
		return Change{}, newError(KindConflict, "CONFIG_DIGEST_MISMATCH", "the current accepted config no longer matches --config-digest", nil)
	}
	if workspace.Projection.AcceptedTruth == nil || workspace.Projection.AcceptedTruth.TruthDigest != expectedTruth {
		return Change{}, newError(KindConflict, "TRUTH_DIGEST_MISMATCH", "the accepted Project Truth no longer matches --truth-digest", nil)
	}
	if workspace.Projection.ActiveChange() != nil {
		return Change{}, newError(KindConflict, "ACTIVE_CHANGE_EXISTS", "this Workspace already has an active Change", nil)
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Goal = strings.TrimSpace(input.Goal)
	input.SupersedesChangeID = strings.TrimSpace(input.SupersedesChangeID)
	if input.Title == "" || input.Goal == "" {
		return Change{}, newError(KindUsage, "CHANGE_CONTRACT_INCOMPLETE", "--title and --goal are required", nil)
	}
	if len(input.Title) > 300 || len(input.Goal) > 4000 {
		return Change{}, newError(KindUsage, "CHANGE_CONTRACT_TOO_LONG", "title or goal exceeds the supported limit", nil)
	}
	if len(input.Scope) == 0 {
		return Change{}, newError(KindUsage, "CHANGE_SCOPE_REQUIRED", "at least one --scope path root is required", nil)
	}
	if len(input.AcceptanceCriteria) == 0 {
		return Change{}, newError(KindUsage, "ACCEPTANCE_REQUIRED", "at least one --acceptance criterion is required", nil)
	}
	if input.Risk == "" {
		input.Risk = workspace.Config.Policy.DefaultRisk
	}
	if input.Risk.Rank() == 0 {
		return Change{}, newError(KindUsage, "INVALID_RISK", "risk must be low, moderate, high, or critical", nil)
	}
	scope, err := normalizeUniquePathRoots(input.Scope)
	if err != nil {
		return Change{}, err
	}
	input.NonGoals, err = normalizeTextList(input.NonGoals, false)
	if err != nil {
		return Change{}, err
	}
	input.AcceptanceCriteria, err = normalizeTextList(input.AcceptanceCriteria, true)
	if err != nil {
		return Change{}, err
	}
	acceptedTruth := workspace.Projection.AcceptedTruth.Truth
	if err := validateChangeImpact(input.Impact, acceptedTruth); err != nil {
		return Change{}, err
	}
	if err := validateChangeRequirements(input.Requirements, input.AcceptanceCriteria, input.Impact, workspace.Config.Gates, true); err != nil {
		return Change{}, err
	}
	if input.SupersedesChangeID != "" {
		if err := validateIdentifier(input.SupersedesChangeID, "supersedes_change_id"); err != nil {
			return Change{}, err
		}
	}
	scopeInference := inferImpact(scope, acceptedTruth)
	if missing := undeclaredInferredImpact(input.Impact, scopeInference); !impactInferenceEmpty(missing) {
		return Change{}, newError(KindBlocked, "DECLARED_IMPACT_INCOMPLETE", fmt.Sprintf("Change scope implies undeclared Project Truth impact: components=%v capabilities=%v invariants=%v unmapped_paths=%v", missing.ComponentIDs, missing.CapabilityIDs, missing.InvariantIDs, missing.UnmappedPaths), nil)
	}
	preflightRisk := MaxRisk(input.Risk, impactTruthRisk(input.Impact, acceptedTruth, false))
	preflightedGates := make(map[string]struct{})
	preflightChange := Change{DeclaredRisk: input.Risk, Impact: input.Impact, Requirements: input.Requirements}
	for _, reachableRisk := range reachableRisks(preflightRisk, scope, workspace.Config.Policy) {
		required := requiredGatesForChange(workspace.Config.Gates, acceptedTruth, acceptedTruth, preflightChange, reachableRisk, false, scope)
		if len(required) == 0 {
			return Change{}, newError(KindBlocked, "NO_REQUIRED_GATES", fmt.Sprintf("Change scope can resolve to risk %q, but the accepted config has no required Gate for that risk", reachableRisk), nil)
		}
		for _, gate := range required {
			if _, exists := preflightedGates[gate.ID]; exists {
				continue
			}
			if _, err := resolveGateExecutionContext(workspace.Root, workspace.Config.Policy, gate); err != nil {
				return Change{}, newError(KindBlocked, "GATE_EXECUTION_CONTEXT_UNAVAILABLE", fmt.Sprintf("required Gate %q cannot run in the current Workspace", gate.ID), err)
			}
			preflightedGates[gate.ID] = struct{}{}
		}
	}
	current, err := service.Git.Snapshot(ctx, workspace.Root, workspace.Config.Policy)
	if err != nil {
		return Change{}, err
	}
	if current.Fingerprint != expectedSource {
		return Change{}, newError(KindConflict, "SOURCE_FINGERPRINT_MISMATCH", "the current source no longer matches --source-fingerprint", nil)
	}
	baseline := current
	lineageRoot := ""
	latest := latestChange(workspace.Projection)
	if input.SupersedesChangeID != "" {
		if latest == nil || latest.ChangeID != input.SupersedesChangeID || latest.State != ChangeCancelled || latest.ActivationID != workspace.Projection.Activation.ActivationID {
			return Change{}, newError(KindConflict, "INVALID_SUPERSEDED_CHANGE", "--supersedes-change must name the latest cancelled Change in the current activation epoch", nil)
		}
		inheritedTouched := TouchedPaths(latest.Baseline, current)
		for _, path := range inheritedTouched {
			if !pathInAnyScope(scope, path) {
				return Change{}, newError(KindBlocked, "SUPERSEDED_CHANGE_OUTSIDE_SCOPE", "the new Change scope must include every path still changed from the superseded Change baseline", nil)
			}
		}
		baseline = latest.Baseline
		lineageRoot = latest.LineageRootChangeID
		if lineageRoot == "" {
			lineageRoot = latest.ChangeID
		}
	} else if latest != nil && latest.State == ChangeCancelled && latest.ActivationID == workspace.Projection.Activation.ActivationID && len(TouchedPaths(latest.Baseline, current)) > 0 {
		return Change{}, newError(KindBlocked, "CHANGE_BASELINE_CARRY_REQUIRED", "source still differs from the latest cancelled Change baseline; explicitly supersede that Change so its edits cannot be laundered into a new baseline", nil)
	}
	changeID, err := randomID("chg", 12)
	if err != nil {
		return Change{}, err
	}
	change := Change{
		SchemaVersion:       SchemaVersion,
		ContractVersion:     2,
		ChangeID:            changeID,
		ActivationID:        workspace.Projection.Activation.ActivationID,
		Title:               input.Title,
		Goal:                input.Goal,
		Scope:               scope,
		NonGoals:            input.NonGoals,
		AcceptanceCriteria:  input.AcceptanceCriteria,
		Impact:              input.Impact,
		Requirements:        input.Requirements,
		SupersedesChangeID:  input.SupersedesChangeID,
		LineageRootChangeID: lineageRoot,
		DeclaredRisk:        input.Risk,
		ConfigDigest:        workspace.Config.Digest,
		TruthDigest:         workspace.Projection.AcceptedTruth.TruthDigest,
		Baseline:            baseline,
		State:               ChangeActive,
		CreatedAt:           service.Clock().UTC(),
	}
	change.ContractDigest, err = changeContractDigest(change)
	if err != nil {
		return Change{}, newError(KindRuntime, "CONTRACT_DIGEST_FAILED", "could not digest Change contract", err)
	}
	revision := workspace.Projection.Revision
	if _, err := workspace.Store.Append(ctx, &revision, PendingEvent{Type: "change_started", Origin: "cli", Payload: change}); err != nil {
		return Change{}, err
	}
	return change, nil
}

func (s Service) Context(ctx context.Context, start string) (ProjectContext, error) {
	service, workspace, err := s.load(ctx, start)
	if err != nil {
		return ProjectContext{}, err
	}
	activationToken, err := activationTokenForWorkspace(service, workspace)
	if err != nil {
		return ProjectContext{}, err
	}
	effective := workspace.effectiveConfig()
	snapshot, err := service.Git.Snapshot(ctx, workspace.Root, effective.Policy)
	if err != nil {
		return ProjectContext{}, err
	}
	contextResult := ProjectContext{
		SchemaVersion:      SchemaVersion,
		ECPEnabled:         workspace.Projection.ECPEnabled(),
		ProjectID:          workspace.ProjectID,
		AuthorityID:        service.authorityID,
		CandidateProjectID: workspace.Config.Project.ProjectID,
		WorkspaceID:        workspace.WorkspaceID,
		ActivationToken:    activationToken,
		Root:               workspace.Root,
		CandidateConfig:    workspace.Config.Digest,
		CandidateTruth:     workspace.Config.TruthDigest,
		Source:             snapshot.Ref(),
		NextActions:        []string{},
		Diagnostics:        []VerdictReason{},
	}
	if workspace.Projection.Activation != nil {
		contextResult.ActivationID = workspace.Projection.Activation.ActivationID
	}
	if workspace.Projection.AcceptedConfig != nil {
		contextResult.AcceptedConfig = workspace.Projection.AcceptedConfig.ConfigDigest
		contextResult.ConfigAccepted = workspace.Projection.AcceptedConfig.ConfigDigest == workspace.Config.Digest && workspace.Config.Project.ProjectID == workspace.ProjectID
	}
	if workspace.Projection.AcceptedTruth != nil {
		contextResult.AcceptedTruth = workspace.Projection.AcceptedTruth.TruthDigest
		contextResult.TruthAccepted = workspace.Projection.AcceptedTruth.TruthDigest == workspace.Config.TruthDigest
	}
	if active := workspace.Projection.ActiveChange(); active != nil {
		contextResult.ActiveChange = summarizeChange(active)
	}
	contextResult.ActiveGateRun = workspace.Projection.ActiveGateRun()
	if !contextResult.ECPEnabled {
		contextResult.Assurance = string(ProjectAssuranceDisabled)
		contextResult.Diagnostics = append(contextResult.Diagnostics, VerdictReason{Code: "ECP_DISABLED", Message: "ECP is not enabled for this Workspace"})
		contextResult.NextActions = append(contextResult.NextActions, "enable ECP for this Workspace before starting a governed Change")
		return contextResult, nil
	}
	if !contextResult.ConfigAccepted {
		contextResult.Assurance = "BLOCKED"
		contextResult.Diagnostics = append(contextResult.Diagnostics, VerdictReason{Code: "CONFIG_NOT_ACCEPTED", Message: "candidate config does not match the accepted config epoch"})
	}
	if !contextResult.TruthAccepted {
		contextResult.Assurance = "BLOCKED"
		contextResult.Diagnostics = append(contextResult.Diagnostics, VerdictReason{Code: "PROJECT_TRUTH_NOT_ACCEPTED", Message: "candidate Project Truth does not match the accepted truth epoch"})
	}
	if active := workspace.Projection.ActiveChange(); active != nil {
		if !contextResult.ConfigAccepted {
			contextResult.NextActions = append(contextResult.NextActions, "restore the exact accepted Draft Config or explicitly cancel the active Change")
		} else if !contextResult.TruthAccepted {
			contextResult.NextActions = append(contextResult.NextActions, "inspect and reconcile the Project Truth delta for the active Change")
		} else {
			verdict, verdictErr := service.evaluate(ctx, workspace, active, snapshot)
			if verdictErr != nil {
				contextResult.Assurance = "INDETERMINATE"
				contextResult.Diagnostics = append(contextResult.Diagnostics, VerdictReason{Code: "VERDICT_ERROR", Message: verdictErr.Error()})
			} else {
				contextResult.Assurance = string(verdict.Status)
				contextResult.Diagnostics = append(contextResult.Diagnostics, verdict.Reasons...)
			}
			contextResult.NextActions = append(contextResult.NextActions, "inspect gate plan", "run required gates", "request current verdict")
		}
	} else {
		if !contextResult.ConfigAccepted {
			contextResult.NextActions = append(contextResult.NextActions, "review Draft Config and accept its exact candidate_config_digest explicitly")
		} else if !contextResult.TruthAccepted {
			contextResult.NextActions = append(contextResult.NextActions, "start a governed Change from the accepted truth or restore the accepted Project Truth")
		} else if len(requiredGates(effective.Gates, effective.Policy.DefaultRisk)) == 0 {
			contextResult.Assurance = "BLOCKED"
			contextResult.Diagnostics = append(contextResult.Diagnostics, VerdictReason{Code: "NO_REQUIRED_GATES", Message: "the default risk has no required Gate; configure and accept a Gate before starting a Change"})
			contextResult.NextActions = append(contextResult.NextActions, "configure at least one Gate for the default risk, review it, then accept the new config epoch")
		} else {
			contextResult.Assurance = "UNASSESSED"
			if effective.Truth.Maturity == TruthMaturitySeed {
				contextResult.Diagnostics = append(contextResult.Diagnostics, VerdictReason{Code: "PROJECT_TRUTH_SEED", Message: "Project Truth is still a bootstrap seed"})
				contextResult.NextActions = append(contextResult.NextActions, "start one bounded onboarding Change to establish Project Truth from repository evidence and product intent")
			} else {
				contextResult.NextActions = append(contextResult.NextActions, "start one bounded Change before modifying managed artifacts")
			}
		}
	}
	return contextResult, nil
}

func (s Service) PlanGates(ctx context.Context, start string) (GatePlan, error) {
	service, workspace, err := s.load(ctx, start)
	if err != nil {
		return GatePlan{}, err
	}
	if err := requireECPEnabled(workspace); err != nil {
		return GatePlan{}, err
	}
	if err := requireAcceptedConfig(workspace); err != nil {
		return GatePlan{}, err
	}
	if err := requireAcceptedTruth(workspace); err != nil {
		return GatePlan{}, err
	}
	change := workspace.Projection.ActiveChange()
	if change == nil {
		return GatePlan{}, newError(KindBlocked, "NO_ACTIVE_CHANGE", "no active Change exists", nil)
	}
	current, err := service.Git.Snapshot(ctx, workspace.Root, workspace.Config.Policy)
	if err != nil {
		return GatePlan{}, err
	}
	plan, _, err := service.buildGatePlan(workspace, change, current)
	return plan, err
}

func (s Service) buildGatePlan(workspace loadedWorkspace, change *Change, current SourceSnapshot) (GatePlan, map[string]gateExecutionContext, error) {
	touched := TouchedPaths(change.Baseline, current)
	startingTruth, err := acceptedTruthAtDigest(workspace.Projection, change.TruthDigest)
	if err != nil {
		return GatePlan{}, nil, err
	}
	truthChanged := change.TruthDigest != workspace.Config.TruthDigest
	inferred := InferredImpact{}
	if change.ContractVersion >= 2 {
		inferred = inferImpact(touched, workspace.Config.Truth)
	}
	risk := effectiveRiskForContract(*change, touched, workspace.Config.Policy, startingTruth, workspace.Config.Truth, truthChanged)
	required := requiredGatesForChange(workspace.Config.Gates, startingTruth, workspace.Config.Truth, *change, risk, truthChanged, touched)
	planned := make([]PlannedGate, 0, len(required))
	contexts := make(map[string]gateExecutionContext, len(required))
	for _, gate := range required {
		executionContext, err := resolveGateExecutionContext(workspace.Root, workspace.Config.Policy, gate)
		if err != nil {
			return GatePlan{}, nil, newError(KindBlocked, "GATE_EXECUTION_CONTEXT_UNAVAILABLE", fmt.Sprintf("required Gate %q cannot be planned in the current Workspace", gate.ID), err)
		}
		contexts[gate.ID] = executionContext
		planned = append(planned, PlannedGate{
			ID:                       gate.ID,
			Description:              gate.Description,
			Tier:                     effectiveGateTier(gate.Tier),
			Command:                  append([]string(nil), gate.Command...),
			WorkingDirectory:         gate.WorkingDirectory,
			ResolvedWorkingDirectory: executionContext.WorkingDirectory,
			ResolvedExecutable:       executionContext.ResolvedExecutable,
			ResolvedExecutableDigest: executionContext.ExecutableDigest,
			EnvironmentNames:         append([]string(nil), executionContext.EnvironmentNames...),
			EnvironmentDigest:        executionContext.EnvironmentDigest,
			TimeoutSeconds:           gate.TimeoutSeconds,
			RequiresNetwork:          gate.RequiresNetwork,
			ProducesSideEffects:      gate.ProducesSideEffects,
		})
	}
	plan := GatePlan{
		SchemaVersion:  SchemaVersion,
		ProjectID:      workspace.ProjectID,
		AuthorityID:    s.authorityID,
		WorkspaceID:    workspace.WorkspaceID,
		ChangeID:       change.ChangeID,
		ActivationID:   change.ActivationID,
		ContractDigest: change.ContractDigest,
		ConfigDigest:   workspace.Config.Digest,
		TruthDigest:    workspace.Config.TruthDigest,
		Source:         current.Ref(),
		EffectiveRisk:  risk,
		TouchedPaths:   touched,
		InferredImpact: inferred,
		Gates:          planned,
		CoreIdentity:   s.CoreIdentity,
	}
	digest, err := digestJSON(plan)
	if err != nil {
		return GatePlan{}, nil, newError(KindRuntime, "GATE_PLAN_DIGEST_FAILED", "could not digest the Gate plan", err)
	}
	plan.PlanDigest = digest
	return plan, contexts, nil
}

func (s Service) RunGates(ctx context.Context, start, expectedChangeID, expectedPlanDigest string, selected []string) (result GateRunResult, returnErr error) {
	expectedChangeID, err := normalizeExpectedChangeID(expectedChangeID)
	if err != nil {
		return GateRunResult{}, err
	}
	expectedPlanDigest, err = normalizeExpectedPlanDigest(expectedPlanDigest)
	if err != nil {
		return GateRunResult{}, err
	}
	service, workspace, err := s.load(ctx, start)
	if err != nil {
		return GateRunResult{}, err
	}
	if err := requireECPEnabled(workspace); err != nil {
		return GateRunResult{}, err
	}
	release, err := workspace.Store.AcquireGateLease(ctx)
	if err != nil {
		return GateRunResult{}, err
	}
	defer release()
	service, workspace, err = service.load(ctx, workspace.Root)
	if err != nil {
		return GateRunResult{}, err
	}
	if err := requireECPEnabled(workspace); err != nil {
		return GateRunResult{}, err
	}
	workspace, err = recoverInterruptedGateRun(ctx, service, workspace, "a new Gate sequence acquired the released Workspace lease")
	if err != nil {
		return GateRunResult{}, err
	}
	if err := requireAcceptedConfig(workspace); err != nil {
		return GateRunResult{}, err
	}
	if err := requireAcceptedTruth(workspace); err != nil {
		return GateRunResult{}, err
	}
	change, err := requireExpectedActiveChange(workspace.Projection, expectedChangeID)
	if err != nil {
		return GateRunResult{}, err
	}
	current, err := service.Git.Snapshot(ctx, workspace.Root, workspace.Config.Policy)
	if err != nil {
		return GateRunResult{}, err
	}
	plan, _, err := service.buildGatePlan(workspace, change, current)
	if err != nil {
		return GateRunResult{}, err
	}
	if plan.PlanDigest != expectedPlanDigest {
		return GateRunResult{}, newError(KindConflict, "GATE_PLAN_MISMATCH", "the current Gate plan no longer matches --plan-digest", nil)
	}
	startingTruth, err := acceptedTruthAtDigest(workspace.Projection, change.TruthDigest)
	if err != nil {
		return GateRunResult{}, err
	}
	required := requiredGatesForChange(
		workspace.Config.Gates,
		startingTruth,
		workspace.Config.Truth,
		*change,
		plan.EffectiveRisk,
		change.TruthDigest != workspace.Config.TruthDigest,
		plan.TouchedPaths,
	)
	gates, err := selectGates(required, selected)
	if err != nil {
		return GateRunResult{}, err
	}
	if len(gates) == 0 {
		return GateRunResult{}, newError(KindBlocked, "NO_REQUIRED_GATES", "current policy resolves to no required Gates", nil)
	}
	runID, err := randomID("run", 12)
	if err != nil {
		return GateRunResult{}, err
	}
	gateIDs := make([]string, 0, len(gates))
	for _, gate := range gates {
		gateIDs = append(gateIDs, gate.ID)
	}
	run := GateRun{
		SchemaVersion: SchemaVersion,
		RunID:         runID,
		ChangeID:      change.ChangeID,
		ActivationID:  change.ActivationID,
		PlanDigest:    expectedPlanDigest,
		GateIDs:       gateIDs,
		State:         GateRunInProgress,
		StartedAt:     service.Clock().UTC(),
		EvidenceIDs:   []string{},
	}
	projection := workspace.Projection
	revision := projection.Revision
	projection, err = workspace.Store.Append(ctx, &revision, PendingEvent{Type: "gate_run_started", Origin: "cli", Payload: run})
	if err != nil {
		return GateRunResult{}, err
	}
	result = GateRunResult{SchemaVersion: SchemaVersion, RunID: runID, ChangeID: change.ChangeID, State: GateRunInProgress, Evidence: []Evidence{}}
	defer func() {
		state, outcomeCode, reason := terminalGateRunOutcome(returnErr)
		finalizeContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		terminalErr := finalizeGateRun(finalizeContext, service, workspace.Store, &projection, runID, state, outcomeCode, reason)
		if terminalErr == nil {
			result.State = state
		} else if returnErr == nil {
			returnErr = terminalErr
		}
	}()
	for _, gate := range gates {
		currentConfig, err := LoadConfig(workspace.Root)
		if err != nil {
			return result, err
		}
		if currentConfig.Digest != workspace.Config.Digest || currentConfig.TruthDigest != workspace.Config.TruthDigest {
			return result, newError(KindBlocked, "CONFIG_CHANGED_DURING_GATE_SEQUENCE", "candidate control config or Project Truth changed during the Gate sequence", nil)
		}
		if gate.RequiresNetwork {
			return result, newError(KindBlocked, "NETWORK_GATE_UNSUPPORTED", fmt.Sprintf("gate %q declares network access; v0.3 has no trusted network authorization boundary", gate.ID), nil)
		}
		pre, err := service.Git.Snapshot(ctx, workspace.Root, workspace.Config.Policy)
		if err != nil {
			return result, err
		}
		currentPlan, contexts, err := service.buildGatePlan(workspace, change, pre)
		if err != nil {
			return result, err
		}
		if currentPlan.PlanDigest != expectedPlanDigest {
			return result, newError(KindConflict, "GATE_PLAN_MISMATCH", "the Gate plan changed before execution; run gate plan again", nil)
		}
		executionContext, ok := contexts[gate.ID]
		if !ok {
			return result, newError(KindConflict, "GATE_NOT_IN_PLAN", fmt.Sprintf("gate %q is not in the exact current plan", gate.ID), nil)
		}
		execution, err := service.Runner.runWithContext(ctx, workspace.Config.Policy, gate, executionContext)
		if err != nil {
			return result, err
		}
		post, err := service.Git.Snapshot(ctx, workspace.Root, workspace.Config.Policy)
		if err != nil {
			return result, newError(KindIntegrity, "GATE_POST_SNAPSHOT_FAILED", "Gate finished but Core could not establish a trustworthy post-run source fingerprint", err)
		}
		evidenceID, err := randomID("evd", 12)
		if err != nil {
			return result, err
		}
		artifact, err := workspace.Store.WriteArtifacts(change.ChangeID, evidenceID, execution.Stdout, execution.Stderr)
		if err != nil {
			return result, err
		}
		gateDigest, err := digestJSON(gate)
		if err != nil {
			return result, newError(KindRuntime, "GATE_DIGEST_FAILED", "could not digest gate definition", err)
		}
		evidence := Evidence{
			SchemaVersion:            SchemaVersion,
			EvidenceID:               evidenceID,
			ChangeID:                 change.ChangeID,
			GateRunID:                runID,
			ActivationID:             change.ActivationID,
			GateID:                   gate.ID,
			PlanDigest:               expectedPlanDigest,
			GateDigest:               gateDigest,
			ContractDigest:           change.ContractDigest,
			ConfigDigest:             workspace.Config.Digest,
			TruthDigest:              workspace.Config.TruthDigest,
			ProjectID:                workspace.ProjectID,
			AuthorityID:              service.authorityID,
			WorkspaceID:              workspace.WorkspaceID,
			PreSnapshot:              pre.Ref(),
			PostSnapshot:             post.Ref(),
			Command:                  execution.Command,
			WorkingDirectory:         execution.WorkingDirectory,
			ResolvedExecutable:       execution.ResolvedExecutable,
			ResolvedExecutableDigest: execution.ExecutableDigest,
			EnvironmentNames:         execution.EnvironmentNames,
			EnvironmentDigest:        execution.EnvironmentDigest,
			StartedAt:                execution.StartedAt,
			FinishedAt:               execution.FinishedAt,
			Duration:                 execution.Duration,
			ExitCode:                 execution.ExitCode,
			ProcessError:             execution.ProcessError,
			TimedOut:                 execution.TimedOut,
			SourceMutated:            pre.Fingerprint != post.Fingerprint,
			ExitCodeAllowed:          execution.ExitCodeAllowed,
			StdoutDigest:             execution.StdoutDigest,
			StderrDigest:             execution.StderrDigest,
			StdoutStoredDigest:       artifact.StdoutDigest,
			StderrStoredDigest:       artifact.StderrDigest,
			StdoutBytes:              execution.StdoutBytes,
			StderrBytes:              execution.StderrBytes,
			StdoutTruncated:          execution.StdoutTruncated,
			StderrTruncated:          execution.StderrTruncated,
			StdoutArtifact:           artifact.StdoutPath,
			StderrArtifact:           artifact.StderrPath,
			RunnerVersion:            service.CoreIdentity,
			Scope:                    "local",
		}
		revision := projection.Revision
		projection, err = workspace.Store.Append(ctx, &revision, PendingEvent{Type: "evidence_recorded", Origin: "cli", Payload: evidence})
		if err != nil {
			return result, err
		}
		result.Evidence = append(result.Evidence, evidence)
	}
	return result, nil
}

func recoverInterruptedGateRun(ctx context.Context, service Service, workspace loadedWorkspace, reason string) (loadedWorkspace, error) {
	active := workspace.Projection.ActiveGateRun()
	if active == nil {
		return workspace, nil
	}
	if strings.TrimSpace(reason) == "" {
		reason = "a later operation acquired the released Gate sequence lease"
	}
	projection := workspace.Projection
	if err := finalizeGateRun(ctx, service, workspace.Store, &projection, active.RunID, GateRunInterrupted, "PREVIOUS_PROCESS_EXITED", reason); err != nil {
		return loadedWorkspace{}, err
	}
	workspace.Projection = projection
	return workspace, nil
}

func finalizeGateRun(ctx context.Context, service Service, store *Store, projection *Projection, runID string, state GateRunState, outcomeCode, reason string) error {
	active := projection.ActiveGateRun()
	if active == nil || active.RunID != runID {
		return newError(KindIntegrity, "GATE_RUN_FINALIZE_MISMATCH", "the GateRun being finalized is no longer active", nil)
	}
	reason = strings.ReplaceAll(reason, "\x00", "�")
	reason = truncateUTF8Bytes(strings.TrimSpace(reason), 2000)
	finishedAt := service.Clock().UTC()
	if finishedAt.Before(active.StartedAt) {
		finishedAt = active.StartedAt
	}
	terminal := GateRunTerminal{
		SchemaVersion: SchemaVersion,
		RunID:         active.RunID,
		ChangeID:      active.ChangeID,
		ActivationID:  active.ActivationID,
		State:         state,
		FinishedAt:    finishedAt,
		EvidenceIDs:   append([]string(nil), active.EvidenceIDs...),
		OutcomeCode:   outcomeCode,
		Reason:        reason,
	}
	revision := projection.Revision
	updated, err := store.Append(ctx, &revision, PendingEvent{Type: "gate_run_finished", Origin: "cli", Payload: terminal})
	if err != nil {
		return err
	}
	*projection = updated
	return nil
}

func truncateUTF8Bytes(value string, limit int) string {
	if limit < 0 || len(value) <= limit {
		return value
	}
	value = value[:limit]
	for len(value) > 0 && !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func terminalGateRunOutcome(err error) (GateRunState, string, string) {
	if err == nil {
		return GateRunCompleted, "SEQUENCE_COMPLETED", "the selected Gate sequence finished and all produced Evidence was recorded"
	}
	if errors.Is(err, context.Canceled) {
		return GateRunCancelled, "CONTEXT_CANCELLED", "the Gate sequence was cancelled before it completed"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return GateRunCancelled, "CONTEXT_DEADLINE_EXCEEDED", "the Gate sequence context deadline elapsed before it completed"
	}
	code := "GATE_SEQUENCE_FAILED"
	var typed *ECPError
	if errors.As(err, &typed) && strings.TrimSpace(typed.Code) != "" {
		code = typed.Code
	}
	return GateRunFailed, code, err.Error()
}

func (s Service) Verdict(ctx context.Context, start string) (Verdict, error) {
	service, workspace, err := s.load(ctx, start)
	if err != nil {
		return Verdict{}, err
	}
	if err := requireECPEnabled(workspace); err != nil {
		return Verdict{}, err
	}
	change := workspace.Projection.ActiveChange()
	if change == nil {
		return Verdict{}, newError(KindBlocked, "NO_ACTIVE_CHANGE", "no active Change exists", nil)
	}
	current, err := service.Git.Snapshot(ctx, workspace.Root, workspace.effectiveConfig().Policy)
	if err != nil {
		return Verdict{}, err
	}
	return service.evaluate(ctx, workspace, change, current)
}

func (s Service) ListChanges(ctx context.Context, start string) ([]ChangeHistoryItem, error) {
	service, workspace, err := s.loadAuthority(ctx, start)
	if err != nil {
		return nil, err
	}
	result := make([]ChangeHistoryItem, 0, len(workspace.Projection.ChangeOrder))
	for _, changeID := range workspace.Projection.ChangeOrder {
		change := workspace.Projection.Changes[changeID]
		if change == nil {
			return nil, newError(KindIntegrity, "CHANGE_HISTORY_MISSING", "Change order references a missing Change", nil)
		}
		item := ChangeHistoryItem{
			SchemaVersion:       SchemaVersion,
			ContractVersion:     change.ContractVersion,
			ProjectID:           workspace.ProjectID,
			AuthorityID:         service.authorityID,
			WorkspaceID:         workspace.WorkspaceID,
			ChangeID:            change.ChangeID,
			ActivationID:        change.ActivationID,
			Title:               change.Title,
			Goal:                change.Goal,
			State:               change.State,
			DeclaredRisk:        change.DeclaredRisk,
			Scope:               append([]string(nil), change.Scope...),
			NonGoals:            append([]string(nil), change.NonGoals...),
			AcceptanceCriteria:  append([]string(nil), change.AcceptanceCriteria...),
			Impact:              change.Impact,
			Requirements:        append([]ChangeRequirement(nil), change.Requirements...),
			SupersedesChangeID:  change.SupersedesChangeID,
			LineageRootChangeID: change.LineageRootChangeID,
			ConfigDigest:        change.ConfigDigest,
			TruthDigest:         change.TruthDigest,
			ContractDigest:      change.ContractDigest,
			CreatedAt:           change.CreatedAt,
			SemanticAssessments: append([]SemanticAssessment(nil), workspace.Projection.SemanticAssessments[changeID]...),
		}
		if completion, ok := workspace.Projection.Completions[changeID]; ok {
			copyOfCompletion := completion
			item.Completion = &copyOfCompletion
		}
		if cancellation, ok := workspace.Projection.Cancellations[changeID]; ok {
			copyOfCancellation := cancellation
			item.Cancellation = &copyOfCancellation
		}
		result = append(result, item)
	}
	return result, nil
}

func (s Service) ListEvidence(ctx context.Context, start, changeID string) (EvidenceReport, error) {
	service, workspace, err := s.loadAuthority(ctx, start)
	if err != nil {
		return EvidenceReport{}, err
	}
	changeID = strings.TrimSpace(changeID)
	if changeID == "" {
		if active := workspace.Projection.ActiveChange(); active != nil {
			changeID = active.ChangeID
		} else if count := len(workspace.Projection.ChangeOrder); count > 0 {
			changeID = workspace.Projection.ChangeOrder[count-1]
		}
	}
	if changeID == "" {
		return EvidenceReport{}, newError(KindNotFound, "NO_CHANGES", "this Workspace has no Change history", nil)
	}
	if err := validateIdentifier(changeID, "change_id"); err != nil {
		return EvidenceReport{}, err
	}
	change := workspace.Projection.Changes[changeID]
	if change == nil {
		return EvidenceReport{}, newError(KindNotFound, "CHANGE_NOT_FOUND", "the requested Change does not exist in this Workspace", nil)
	}
	return EvidenceReport{
		SchemaVersion: SchemaVersion,
		ProjectID:     workspace.ProjectID,
		AuthorityID:   service.authorityID,
		WorkspaceID:   workspace.WorkspaceID,
		ChangeID:      changeID,
		ChangeState:   change.State,
		Evidence:      sortedEvidenceByTime(workspace.Projection.Evidence[changeID]),
	}, nil
}

func (s Service) ListGateRuns(ctx context.Context, start, changeID string) (GateRunReport, error) {
	service, workspace, err := s.loadAuthority(ctx, start)
	if err != nil {
		return GateRunReport{}, err
	}
	changeID = strings.TrimSpace(changeID)
	if changeID != "" {
		if err := validateIdentifier(changeID, "change_id"); err != nil {
			return GateRunReport{}, err
		}
		if workspace.Projection.Changes[changeID] == nil {
			return GateRunReport{}, newError(KindNotFound, "CHANGE_NOT_FOUND", "the requested Change does not exist in this Workspace", nil)
		}
	}
	runs := make([]GateRun, 0, len(workspace.Projection.GateRunOrder))
	for _, runID := range workspace.Projection.GateRunOrder {
		run := workspace.Projection.GateRuns[runID]
		if run == nil {
			return GateRunReport{}, newError(KindIntegrity, "GATE_RUN_HISTORY_MISSING", "GateRun order references a missing run", nil)
		}
		if changeID != "" && run.ChangeID != changeID {
			continue
		}
		copyOfRun := *run
		copyOfRun.GateIDs = append([]string(nil), run.GateIDs...)
		copyOfRun.EvidenceIDs = append([]string(nil), run.EvidenceIDs...)
		runs = append(runs, copyOfRun)
	}
	return GateRunReport{
		SchemaVersion: SchemaVersion,
		ProjectID:     workspace.ProjectID,
		AuthorityID:   service.authorityID,
		WorkspaceID:   workspace.WorkspaceID,
		ChangeID:      changeID,
		Runs:          runs,
	}, nil
}

func (s Service) RecordAcknowledgement(ctx context.Context, start, expectedChangeID, expectedSubjectDigest, actor, reason string) (Acknowledgement, error) {
	expectedChangeID, err := normalizeExpectedChangeID(expectedChangeID)
	if err != nil {
		return Acknowledgement{}, err
	}
	expectedSubjectDigest, err = normalizeExpectedSubjectDigest(expectedSubjectDigest)
	if err != nil {
		return Acknowledgement{}, err
	}
	service, workspace, err := s.load(ctx, start)
	if err != nil {
		return Acknowledgement{}, err
	}
	if err := requireECPEnabled(workspace); err != nil {
		return Acknowledgement{}, err
	}
	actor = strings.TrimSpace(actor)
	reason = strings.TrimSpace(reason)
	if actor == "" || reason == "" {
		return Acknowledgement{}, newError(KindUsage, "ACK_FIELDS_REQUIRED", "--actor and --reason are required", nil)
	}
	if len(actor) > 200 || len(reason) > 2000 {
		return Acknowledgement{}, newError(KindUsage, "ACK_FIELDS_TOO_LONG", "actor or reason exceeds the supported limit", nil)
	}
	release, err := workspace.Store.AcquireGateLease(ctx)
	if err != nil {
		return Acknowledgement{}, err
	}
	defer release()
	service, workspace, err = service.load(ctx, workspace.Root)
	if err != nil {
		return Acknowledgement{}, err
	}
	if err := requireECPEnabled(workspace); err != nil {
		return Acknowledgement{}, err
	}
	workspace, err = recoverInterruptedGateRun(ctx, service, workspace, "risk acknowledgement acquired the released Gate sequence lease")
	if err != nil {
		return Acknowledgement{}, err
	}
	change, err := requireExpectedActiveChange(workspace.Projection, expectedChangeID)
	if err != nil {
		return Acknowledgement{}, err
	}
	current, err := service.Git.Snapshot(ctx, workspace.Root, workspace.effectiveConfig().Policy)
	if err != nil {
		return Acknowledgement{}, err
	}
	verdict, err := service.evaluate(ctx, workspace, change, current)
	if err != nil {
		return Acknowledgement{}, err
	}
	if verdict.SubjectDigest != expectedSubjectDigest {
		return Acknowledgement{}, newError(KindConflict, "SUBJECT_DIGEST_MISMATCH", "the current subject no longer matches --subject-digest", nil)
	}
	if !onlyAcknowledgementBlocks(verdict.Reasons) {
		return Acknowledgement{}, newError(KindBlocked, "SUBJECT_NOT_ACKNOWLEDGEABLE", "all non-acknowledgement requirements must be satisfied before recording acknowledgement", nil)
	}
	id, err := randomID("ack", 12)
	if err != nil {
		return Acknowledgement{}, err
	}
	acknowledgement := Acknowledgement{
		SchemaVersion: SchemaVersion,
		ID:            id,
		ChangeID:      change.ChangeID,
		ActivationID:  change.ActivationID,
		SubjectDigest: verdict.SubjectDigest,
		Actor:         actor,
		Reason:        reason,
		RecordedAt:    service.Clock().UTC(),
		Trust:         "local-acknowledgement",
	}
	revision := workspace.Projection.Revision
	if _, err := workspace.Store.Append(ctx, &revision, PendingEvent{Type: "acknowledgement_recorded", Origin: "cli", Payload: acknowledgement}); err != nil {
		return Acknowledgement{}, err
	}
	return acknowledgement, nil
}

func (s Service) CompleteChange(ctx context.Context, start, expectedChangeID, expectedSubjectDigest string) (ChangeCompletion, error) {
	expectedChangeID, err := normalizeExpectedChangeID(expectedChangeID)
	if err != nil {
		return ChangeCompletion{}, err
	}
	expectedSubjectDigest, err = normalizeExpectedSubjectDigest(expectedSubjectDigest)
	if err != nil {
		return ChangeCompletion{}, err
	}
	service, workspace, err := s.load(ctx, start)
	if err != nil {
		return ChangeCompletion{}, err
	}
	if err := requireECPEnabled(workspace); err != nil {
		return ChangeCompletion{}, err
	}
	release, err := workspace.Store.AcquireGateLease(ctx)
	if err != nil {
		return ChangeCompletion{}, err
	}
	defer release()
	service, workspace, err = service.load(ctx, workspace.Root)
	if err != nil {
		return ChangeCompletion{}, err
	}
	if err := requireECPEnabled(workspace); err != nil {
		return ChangeCompletion{}, err
	}
	workspace, err = recoverInterruptedGateRun(ctx, service, workspace, "Change completion acquired the released Gate sequence lease")
	if err != nil {
		return ChangeCompletion{}, err
	}
	change, err := requireExpectedActiveChange(workspace.Projection, expectedChangeID)
	if err != nil {
		return ChangeCompletion{}, err
	}
	current, err := service.Git.Snapshot(ctx, workspace.Root, workspace.effectiveConfig().Policy)
	if err != nil {
		return ChangeCompletion{}, err
	}
	verdict, err := service.evaluate(ctx, workspace, change, current)
	if err != nil {
		return ChangeCompletion{}, err
	}
	if verdict.SubjectDigest != expectedSubjectDigest {
		return ChangeCompletion{}, newError(KindConflict, "SUBJECT_DIGEST_MISMATCH", "the current subject no longer matches --subject-digest", nil)
	}
	if verdict.Status != VerdictPass {
		return ChangeCompletion{}, newError(KindBlocked, "CURRENT_VERDICT_NOT_PASS", "Change can complete only with a current PASS Verdict", nil)
	}
	recheck, err := service.Git.Snapshot(ctx, workspace.Root, workspace.effectiveConfig().Policy)
	if err != nil {
		return ChangeCompletion{}, err
	}
	if recheck.Fingerprint != current.Fingerprint {
		return ChangeCompletion{}, newError(KindConflict, "SOURCE_CHANGED_DURING_COMPLETE", "source changed between Verdict and completion", nil)
	}
	verdictDigest, err := digestJSON(verdict)
	if err != nil {
		return ChangeCompletion{}, newError(KindRuntime, "VERDICT_DIGEST_FAILED", "could not digest final Verdict", err)
	}
	completion := ChangeCompletion{
		SchemaVersion: SchemaVersion,
		ChangeID:      change.ChangeID,
		ActivationID:  change.ActivationID,
		SubjectDigest: verdict.SubjectDigest,
		VerdictDigest: verdictDigest,
		FinalVerdict:  verdict,
		CompletedAt:   service.Clock().UTC(),
	}
	revision := workspace.Projection.Revision
	if _, err := workspace.Store.Append(ctx, &revision, PendingEvent{Type: "change_completed", Origin: "cli", Payload: completion}); err != nil {
		return ChangeCompletion{}, err
	}
	return completion, nil
}

func (s Service) CancelChange(ctx context.Context, start, expectedAuthorityID, expectedWorkspaceID, expectedChangeID, actor, reason string) (ChangeCancellation, error) {
	expectedAuthorityID, err := normalizeExpectedAuthorityID(expectedAuthorityID)
	if err != nil {
		return ChangeCancellation{}, err
	}
	expectedWorkspaceID, err = normalizeExpectedWorkspaceID(expectedWorkspaceID)
	if err != nil {
		return ChangeCancellation{}, err
	}
	expectedChangeID, err = normalizeExpectedChangeID(expectedChangeID)
	if err != nil {
		return ChangeCancellation{}, err
	}
	actor = strings.TrimSpace(actor)
	reason = strings.TrimSpace(reason)
	if actor == "" || reason == "" {
		return ChangeCancellation{}, newError(KindUsage, "CANCEL_FIELDS_REQUIRED", "--actor and --reason are required", nil)
	}
	if len(actor) > 200 || len(reason) > 2000 {
		return ChangeCancellation{}, newError(KindUsage, "CANCEL_FIELDS_TOO_LONG", "actor or reason exceeds the supported limit", nil)
	}
	service, workspace, err := s.loadAuthority(ctx, start)
	if err != nil {
		return ChangeCancellation{}, err
	}
	if err := requireExpectedAuthorityWorkspace(service, workspace, expectedAuthorityID, expectedWorkspaceID); err != nil {
		return ChangeCancellation{}, err
	}
	release, err := workspace.Store.AcquireGateLease(ctx)
	if err != nil {
		return ChangeCancellation{}, err
	}
	defer release()
	service, workspace, err = service.loadAuthority(ctx, workspace.Root)
	if err != nil {
		return ChangeCancellation{}, err
	}
	if err := requireExpectedAuthorityWorkspace(service, workspace, expectedAuthorityID, expectedWorkspaceID); err != nil {
		return ChangeCancellation{}, err
	}
	workspace, err = recoverInterruptedGateRun(ctx, service, workspace, "Change cancellation acquired the released Gate sequence lease")
	if err != nil {
		return ChangeCancellation{}, err
	}
	change, err := requireExpectedActiveChange(workspace.Projection, expectedChangeID)
	if err != nil {
		return ChangeCancellation{}, err
	}
	cancellation := ChangeCancellation{
		SchemaVersion: SchemaVersion,
		ChangeID:      change.ChangeID,
		ActivationID:  change.ActivationID,
		Actor:         actor,
		Reason:        reason,
		Trust:         "local-cancellation-acknowledgement",
		CancelledAt:   service.Clock().UTC(),
	}
	revision := workspace.Projection.Revision
	if _, err := workspace.Store.Append(ctx, &revision, PendingEvent{Type: "change_cancelled", Origin: "cli", Payload: cancellation}); err != nil {
		return ChangeCancellation{}, err
	}
	return cancellation, nil
}

func (s Service) evaluate(ctx context.Context, workspace loadedWorkspace, change *Change, current SourceSnapshot) (Verdict, error) {
	service, err := s.withDefaults()
	if err != nil {
		return Verdict{}, err
	}
	if workspace.Projection.Activation == nil || !workspace.Projection.Activation.Enabled || change.ActivationID != workspace.Projection.Activation.ActivationID {
		return Verdict{}, newError(KindIntegrity, "CHANGE_ACTIVATION_MISMATCH", "active Change is not bound to the current enabled project epoch", nil)
	}
	effective := workspace.effectiveConfig()
	touched := TouchedPaths(change.Baseline, current)
	startingTruth, err := acceptedTruthAtDigest(workspace.Projection, change.TruthDigest)
	if err != nil {
		return Verdict{}, err
	}
	truthChanged := change.TruthDigest != effective.TruthDigest
	inferred := InferredImpact{}
	risk := effectiveRiskForChangeLegacy(*change, touched, effective.Policy, startingTruth, effective.Truth, truthChanged)
	if change.ContractVersion >= 2 {
		inferred = inferImpact(touched, effective.Truth)
		risk = effectiveRiskForChange(*change, touched, effective.Policy, startingTruth, effective.Truth, truthChanged)
	}
	reasons := make([]VerdictReason, 0)
	indeterminate := false
	if workspace.Projection.AcceptedConfig == nil || workspace.Projection.AcceptedConfig.ConfigDigest != workspace.Config.Digest || workspace.Config.Project.ProjectID != workspace.ProjectID {
		reasons = append(reasons, VerdictReason{Code: "CONFIG_NOT_ACCEPTED", Message: "candidate .ecp config does not match the accepted config epoch"})
	}
	if workspace.Projection.AcceptedTruth == nil || workspace.Projection.AcceptedTruth.TruthDigest != workspace.Config.TruthDigest {
		reasons = append(reasons, VerdictReason{Code: "PROJECT_TRUTH_NOT_ACCEPTED", Message: "candidate Project Truth does not match the accepted truth epoch"})
	}
	if change.ConfigDigest != effective.Digest {
		reasons = append(reasons, VerdictReason{Code: "CHANGE_CONFIG_EPOCH_CHANGED", Message: "active Change is bound to a different config digest"})
	}
	if change.ContractVersion >= 2 {
		if missing := undeclaredInferredImpactForChange(*change, inferred, startingTruth, truthChanged); !impactInferenceEmpty(missing) {
			reasons = append(reasons, VerdictReason{Code: "DECLARED_IMPACT_INCOMPLETE", Message: fmt.Sprintf("final touched paths imply undeclared Project Truth impact: components=%v capabilities=%v invariants=%v unmapped_paths=%v", missing.ComponentIDs, missing.CapabilityIDs, missing.InvariantIDs, missing.UnmappedPaths)})
		}
	}
	if activeRun := workspace.Projection.ActiveGateRun(); activeRun != nil {
		reasons = append(reasons, VerdictReason{Code: "GATE_RUN_IN_PROGRESS_OR_INTERRUPTED", Message: "a durable GateRun is still IN_PROGRESS; the current holder must finish it or a later lease holder must record interruption before Verdict can be trusted"})
		indeterminate = true
	}
	semanticID := ""
	semantic := latestSemanticAssessment(workspace.Projection, change.ChangeID)
	if semantic == nil {
		reasons = append(reasons, VerdictReason{Code: "SEMANTIC_ASSESSMENT_REQUIRED", Message: "the current Change has no semantic reconciliation for the final source"})
	} else if semantic.Behavior == SemanticBehaviorUnknown {
		reasons = append(reasons, VerdictReason{Code: "SEMANTIC_OUTCOME_UNKNOWN", Message: "semantic behavior remains UNKNOWN and cannot receive PASS"})
	} else if semantic.CurrentTruthDigest != effective.TruthDigest || semantic.SourceFingerprint != current.Fingerprint {
		reasons = append(reasons, VerdictReason{Code: "SEMANTIC_ASSESSMENT_STALE", Message: "semantic reconciliation is bound to a different source or Project Truth epoch"})
	} else {
		semanticID = semantic.AssessmentID
	}
	for _, path := range touched {
		for _, denied := range effective.Policy.DeniedPathRoots {
			// Recursive submodule changes collapse to the mount path. A denied
			// descendant must therefore conservatively overlap that mount just as
			// a path-risk rule does; Core cannot prove the descendant was untouched.
			if containsPath(denied, path) || containsPath(path, denied) {
				reasons = append(reasons, VerdictReason{Code: "DENIED_PATH_TOUCHED", Message: "Change touched a control-plane path denied by policy", Path: path})
				break
			}
		}
		if !pathInAnyScope(change.Scope, path) {
			reasons = append(reasons, VerdictReason{Code: "OUT_OF_SCOPE", Message: "Change touched a path outside its contract", Path: path})
		}
	}

	required := requiredGatesForChange(effective.Gates, startingTruth, effective.Truth, *change, risk, truthChanged, touched)
	currentPlanDigest := ""
	if currentPlan, _, planErr := service.buildGatePlan(workspace, change, current); planErr == nil {
		currentPlanDigest = currentPlan.PlanDigest
	}
	assessments := make([]GateAssessment, 0, len(required))
	if len(required) == 0 {
		reasons = append(reasons, VerdictReason{Code: "NO_REQUIRED_GATES", Message: "effective policy resolved to no required Gates; vacuous PASS is forbidden"})
	}
	for _, gate := range required {
		assessment := GateAssessment{GateID: gate.ID, Required: true, State: "MISSING"}
		gateDigest, err := digestJSON(gate)
		if err != nil {
			return Verdict{}, err
		}
		executionContext, err := resolveGateExecutionContext(workspace.Root, effective.Policy, gate)
		if err != nil {
			assessment.State = "INDETERMINATE"
			reasons = append(reasons, VerdictReason{Code: "GATE_EXECUTION_CONTEXT_UNAVAILABLE", Message: err.Error(), GateID: gate.ID})
			indeterminate = true
			assessments = append(assessments, assessment)
			continue
		}
		candidates := workspace.Projection.Evidence[change.ChangeID]
		matchedSubject := false
		for i := len(candidates) - 1; i >= 0; i-- {
			evidence := candidates[i]
			if evidence.GateID != gate.ID || evidence.ProjectID != workspace.ProjectID || evidence.AuthorityID != service.authorityID || evidence.WorkspaceID != workspace.WorkspaceID || evidence.ChangeID != change.ChangeID || evidence.ActivationID != change.ActivationID {
				continue
			}
			if evidence.ConfigDigest != effective.Digest || evidence.TruthDigest != effective.TruthDigest || evidence.ContractDigest != change.ContractDigest || (currentPlanDigest != "" && evidence.PlanDigest != currentPlanDigest) || evidence.GateDigest != gateDigest || evidence.PreSnapshot.Fingerprint != current.Fingerprint ||
				!slices.Equal(evidence.Command, gate.Command) || evidence.WorkingDirectory != executionContext.WorkingDirectory || evidence.ResolvedExecutable != executionContext.ResolvedExecutable ||
				evidence.ResolvedExecutableDigest != executionContext.ExecutableDigest || evidence.EnvironmentDigest != executionContext.EnvironmentDigest ||
				!slices.Equal(evidence.EnvironmentNames, executionContext.EnvironmentNames) {
				continue
			}
			matchedSubject = true
			assessment.EvidenceID = evidence.EvidenceID
			if err := workspace.Store.VerifyArtifact(evidence.StdoutArtifact, evidence.StdoutStoredDigest); err != nil {
				assessment.State = "CORRUPT"
				reasons = append(reasons, VerdictReason{Code: "EVIDENCE_ARTIFACT_INVALID", Message: err.Error(), GateID: gate.ID})
				indeterminate = true
				break
			}
			if err := workspace.Store.VerifyArtifact(evidence.StderrArtifact, evidence.StderrStoredDigest); err != nil {
				assessment.State = "CORRUPT"
				reasons = append(reasons, VerdictReason{Code: "EVIDENCE_ARTIFACT_INVALID", Message: err.Error(), GateID: gate.ID})
				indeterminate = true
				break
			}
			if evidence.RunnerVersion != service.CoreIdentity || evidence.Scope != "local" {
				assessment.State = "INCOMPATIBLE"
				reasons = append(reasons, VerdictReason{Code: "EVIDENCE_CONTEXT_INCOMPATIBLE", Message: "evidence runner or scope is incompatible with this evaluator", GateID: gate.ID})
				break
			}
			if evidence.PostSnapshot.Fingerprint != evidence.PreSnapshot.Fingerprint || evidence.SourceMutated {
				assessment.State = "INVALIDATED"
				reasons = append(reasons, VerdictReason{Code: "GATE_MUTATED_SOURCE", Message: "source changed while the Gate was running", GateID: gate.ID})
				break
			}
			if !evidence.Passed() {
				assessment.State = "FAILED"
				reasons = append(reasons, VerdictReason{Code: "GATE_FAILED", Message: "latest applicable Gate Evidence did not pass", GateID: gate.ID})
				break
			}
			assessment.State = "PASS"
			break
		}
		if assessment.State == "MISSING" {
			code := "GATE_EVIDENCE_MISSING"
			message := "required Gate has no Evidence for the current source fingerprint"
			if !matchedSubject && hasEvidenceForGate(candidates, gate.ID) {
				code = "GATE_EVIDENCE_STALE"
				message = "required Gate Evidence is bound to a different source/config/contract"
				assessment.State = "STALE"
			}
			reasons = append(reasons, VerdictReason{Code: code, Message: message, GateID: gate.ID})
		}
		assessments = append(assessments, assessment)
	}
	if semantic != nil && semantic.CurrentTruthDigest == effective.TruthDigest && semantic.SourceFingerprint == current.Fingerprint {
		gateStates := make(map[string]string, len(assessments))
		for _, assessment := range assessments {
			gateStates[assessment.GateID] = assessment.State
		}
		for _, requirement := range change.Requirements {
			if requirement.Verification == RequirementVerificationExternal && requirement.Status == RequirementDecided {
				reasons = append(reasons, VerdictReason{Code: "EXTERNAL_REQUIREMENT_PENDING", Message: fmt.Sprintf("requirement %q needs external Evidence, which the local v0.3 Core cannot import or attest", requirement.ID)})
				continue
			}
			if requirement.Verification != RequirementVerificationAutomated {
				continue
			}
			for _, gateID := range requirement.RequiredGateIDs {
				if gateStates[gateID] != "PASS" {
					reasons = append(reasons, VerdictReason{Code: "REQUIREMENT_EVIDENCE_UNSATISFIED", Message: fmt.Sprintf("requirement %q does not have PASS Evidence from its mapped Gate", requirement.ID), GateID: gateID})
				}
			}
		}
	}

	subjectDigest, err := verdictSubjectDigestForChange(*change, Verdict{
		SchemaVersion:        SchemaVersion,
		ProjectID:            workspace.ProjectID,
		AuthorityID:          service.authorityID,
		WorkspaceID:          workspace.WorkspaceID,
		ChangeID:             change.ChangeID,
		ActivationID:         change.ActivationID,
		ContractDigest:       change.ContractDigest,
		ConfigDigest:         effective.Digest,
		TruthDigest:          effective.TruthDigest,
		SemanticAssessmentID: semanticID,
		Source:               current.Ref(),
		EffectiveRisk:        risk,
		TouchedPaths:         touched,
		InferredImpact:       inferred,
		GateAssessments:      assessments,
	})
	if err != nil {
		return Verdict{}, err
	}
	ackID := ""
	if acknowledgementRequired(effective.Policy, risk) {
		for i := len(workspace.Projection.Acknowledgements[change.ChangeID]) - 1; i >= 0; i-- {
			ack := workspace.Projection.Acknowledgements[change.ChangeID][i]
			if ack.ActivationID == change.ActivationID && ack.SubjectDigest == subjectDigest {
				ackID = ack.ID
				break
			}
		}
		if ackID == "" {
			reasons = append(reasons, VerdictReason{Code: "ACKNOWLEDGEMENT_REQUIRED", Message: "effective risk requires a local acknowledgement bound to this exact subject"})
		}
	}

	status := VerdictPass
	if len(reasons) > 0 {
		status = VerdictBlocked
	}
	if indeterminate {
		status = VerdictIndeterminate
	}
	return Verdict{
		SchemaVersion:        SchemaVersion,
		Status:               status,
		ProjectID:            workspace.ProjectID,
		AuthorityID:          service.authorityID,
		WorkspaceID:          workspace.WorkspaceID,
		ChangeID:             change.ChangeID,
		ActivationID:         change.ActivationID,
		ChangeState:          change.State,
		Source:               current.Ref(),
		ConfigDigest:         effective.Digest,
		TruthDigest:          effective.TruthDigest,
		ContractDigest:       change.ContractDigest,
		SemanticAssessmentID: semanticID,
		DeclaredRisk:         change.DeclaredRisk,
		EffectiveRisk:        risk,
		TouchedPaths:         touched,
		InferredImpact:       inferred,
		GateAssessments:      assessments,
		Acknowledgement:      ackID,
		Reasons:              reasons,
		SubjectDigest:        subjectDigest,
		EvaluatorVersion:     service.CoreIdentity,
		EvaluatedAt:          service.Clock().UTC(),
	}, nil
}

func (s Service) load(ctx context.Context, start string) (Service, loadedWorkspace, error) {
	service, workspace, err := s.loadAuthority(ctx, start)
	if err != nil {
		return Service{}, loadedWorkspace{}, err
	}
	config, err := LoadConfig(workspace.Root)
	if err != nil {
		if isErrorCodeValue(err, "ECP_CONFIG_NOT_FOUND") || isErrorCodeValue(err, "CONFIG_FILE_NOT_FOUND") {
			return Service{}, loadedWorkspace{}, newError(KindIntegrity, "REGISTERED_CONFIG_MISSING", "registered Workspace is missing required Draft Config", err)
		}
		return Service{}, loadedWorkspace{}, err
	}
	workspace.Config = config
	return service, workspace, nil
}

func (s Service) loadAuthority(ctx context.Context, start string) (Service, loadedWorkspace, error) {
	service, workspace, err := s.resolveAuthorityTarget(ctx, start)
	if err != nil {
		return Service{}, loadedWorkspace{}, err
	}
	projection, err := workspace.Store.Load(ctx)
	if err != nil {
		return Service{}, loadedWorkspace{}, err
	}
	if projection.AcceptedTruth != nil {
		if _, err := workspace.Store.ReadTruthFiles(projection.AcceptedTruth.Files); err != nil {
			return Service{}, loadedWorkspace{}, err
		}
	}
	if projection.Registration == nil || projection.Registration.AuthorityID != service.authorityID {
		return Service{}, loadedWorkspace{}, newError(KindIntegrity, "AUTHORITY_REGISTRATION_MISMATCH", "authority history belongs to a different authority-state location", nil)
	}
	workspace.Projection = projection
	return service, workspace, nil
}

// resolveAuthorityTarget selects the immutable Workspace binding and its
// repository-external Store without loading candidate .ecp files or assuming
// that referenced authority objects are healthy. Callers that diagnose the
// authority use this boundary so missing/corrupt blobs can become findings
// instead of preventing the diagnostic report itself from being constructed.
func (s Service) resolveAuthorityTarget(ctx context.Context, start string) (Service, loadedWorkspace, error) {
	service, err := s.withDefaults()
	if err != nil {
		return Service{}, loadedWorkspace{}, err
	}
	if err := requireAuthorityPlatform(); err != nil {
		return Service{}, loadedWorkspace{}, err
	}
	root, err := service.Git.DiscoverRoot(ctx, start)
	if err != nil {
		return Service{}, loadedWorkspace{}, err
	}
	if err := ensureOutsideRoot(root, service.StateDir); err != nil {
		return Service{}, loadedWorkspace{}, err
	}
	workspaceID, err := service.Git.WorkspaceID(ctx, root)
	if err != nil {
		return Service{}, loadedWorkspace{}, err
	}
	binding, err := loadWorkspaceBinding(service.StateDir, workspaceID)
	if err != nil {
		if isErrorCodeValue(err, "WORKSPACE_NOT_REGISTERED") {
			return Service{}, loadedWorkspace{}, newError(KindBlocked, "PROJECT_REGISTRATION_REQUIRED", "this Workspace must be initialized or explicitly registered before ECP operations", err)
		}
		return Service{}, loadedWorkspace{}, err
	}
	if binding.AuthorityID != service.authorityID {
		return Service{}, loadedWorkspace{}, newError(KindIntegrity, "AUTHORITY_BINDING_MISMATCH", "Workspace binding belongs to a different authority-state location", nil)
	}
	store, err := NewStore(service.StateDir, binding.ProjectID, workspaceID, 5*time.Second)
	if err != nil {
		return Service{}, loadedWorkspace{}, err
	}
	return service, loadedWorkspace{Root: root, ProjectID: binding.ProjectID, Binding: binding, WorkspaceID: workspaceID, Store: store}, nil
}

func requireAcceptedConfig(workspace loadedWorkspace) error {
	if workspace.Projection.AcceptedConfig == nil || workspace.Projection.AcceptedConfig.ConfigDigest != workspace.Config.Digest || workspace.Config.Project.ProjectID != workspace.ProjectID {
		return newError(KindBlocked, "CONFIG_NOT_ACCEPTED", "candidate .ecp config differs from the accepted config epoch", nil)
	}
	return nil
}

func requireAcceptedTruth(workspace loadedWorkspace) error {
	if workspace.Projection.AcceptedTruth == nil || workspace.Projection.AcceptedTruth.TruthDigest != workspace.Config.TruthDigest {
		return newError(KindBlocked, "PROJECT_TRUTH_NOT_ACCEPTED", "candidate Project Truth differs from the accepted truth epoch", nil)
	}
	return nil
}

func requireECPEnabled(workspace loadedWorkspace) error {
	if !workspace.Projection.ECPEnabled() {
		return newError(KindBlocked, "ECP_DISABLED", "ECP is not enabled for this Workspace", nil)
	}
	return nil
}

func normalizeExpectedChangeID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", newError(KindUsage, "CHANGE_ID_REQUIRED", "--change is required for an active Change mutation", nil)
	}
	if len(value) > 200 {
		return "", newError(KindUsage, "CHANGE_ID_TOO_LONG", "--change exceeds the supported limit", nil)
	}
	return value, nil
}

func normalizeExpectedAuthorityID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", newError(KindUsage, "AUTHORITY_ID_REQUIRED", "--authority is required for this authority-state mutation", nil)
	}
	if !strings.HasPrefix(value, "auth-") || len(value) != len("auth-")+32 {
		return "", newError(KindUsage, "INVALID_AUTHORITY_ID", "--authority must be a complete ECP authority ID", nil)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(value, "auth-")); err != nil {
		return "", newError(KindUsage, "INVALID_AUTHORITY_ID", "--authority must be a complete ECP authority ID", err)
	}
	return value, nil
}

func authorityIDForStateDir(stateDir string) string {
	digest := strings.TrimPrefix(digestBytes([]byte("ecp-authority-state-v1\x00"+stateDir)), "sha256:")
	return "auth-" + digest[:32]
}

func normalizeExpectedWorkspaceID(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", newError(KindUsage, "WORKSPACE_ID_REQUIRED", "--workspace is required to establish a Change baseline", nil)
	}
	if !strings.HasPrefix(value, "ws-") || len(value) != len("ws-")+32 {
		return "", newError(KindUsage, "INVALID_WORKSPACE_ID", "--workspace must be a complete ECP Workspace ID", nil)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(value, "ws-")); err != nil {
		return "", newError(KindUsage, "INVALID_WORKSPACE_ID", "--workspace must be a complete ECP Workspace ID", err)
	}
	return value, nil
}

func normalizeExpectedConfigDigest(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", newError(KindUsage, "CONFIG_DIGEST_REQUIRED", "--config-digest is required for a config acceptance mutation", nil)
	}
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return "", newError(KindUsage, "INVALID_CONFIG_DIGEST", "--config-digest must be a complete sha256 digest", nil)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:")); err != nil {
		return "", newError(KindUsage, "INVALID_CONFIG_DIGEST", "--config-digest must be a complete sha256 digest", err)
	}
	return value, nil
}

func normalizeExpectedTruthDigest(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", newError(KindUsage, "TRUTH_DIGEST_REQUIRED", "--truth-digest is required for a Project Truth-bound mutation", nil)
	}
	if !isSHA256Digest(value) {
		return "", newError(KindUsage, "INVALID_TRUTH_DIGEST", "--truth-digest must be a complete sha256 digest", nil)
	}
	return value, nil
}

func normalizeExpectedActivationToken(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", newError(KindUsage, "ACTIVATION_TOKEN_REQUIRED", "--activation-token is required for this project-mode operation", nil)
	}
	if !isSHA256Digest(value) {
		return "", newError(KindUsage, "INVALID_ACTIVATION_TOKEN", "--activation-token must be a complete ECP activation token", nil)
	}
	return value, nil
}

func requireExpectedActivationToken(service Service, workspace loadedWorkspace, expected string) error {
	current, err := activationTokenForWorkspace(service, workspace)
	if err != nil {
		return err
	}
	if current != expected {
		return newError(KindConflict, "ACTIVATION_TOKEN_MISMATCH", "the project activation state no longer matches --activation-token", nil)
	}
	return nil
}

func normalizeExpectedSubjectDigest(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", newError(KindUsage, "SUBJECT_DIGEST_REQUIRED", "--subject-digest is required for a subject confirmation mutation", nil)
	}
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return "", newError(KindUsage, "INVALID_SUBJECT_DIGEST", "--subject-digest must be a complete sha256 digest", nil)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:")); err != nil {
		return "", newError(KindUsage, "INVALID_SUBJECT_DIGEST", "--subject-digest must be a complete sha256 digest", err)
	}
	return value, nil
}

func normalizeExpectedSourceFingerprint(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", newError(KindUsage, "SOURCE_FINGERPRINT_REQUIRED", "--source-fingerprint is required to establish a Change baseline", nil)
	}
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return "", newError(KindUsage, "INVALID_SOURCE_FINGERPRINT", "--source-fingerprint must be a complete sha256 digest", nil)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:")); err != nil {
		return "", newError(KindUsage, "INVALID_SOURCE_FINGERPRINT", "--source-fingerprint must be a complete sha256 digest", err)
	}
	return value, nil
}

func normalizeExpectedPlanDigest(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", newError(KindUsage, "PLAN_DIGEST_REQUIRED", "--plan-digest is required to execute configured Gates", nil)
	}
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return "", newError(KindUsage, "INVALID_PLAN_DIGEST", "--plan-digest must be a complete sha256 digest", nil)
	}
	if _, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:")); err != nil {
		return "", newError(KindUsage, "INVALID_PLAN_DIGEST", "--plan-digest must be a complete sha256 digest", err)
	}
	return value, nil
}

func requireExpectedActiveChange(projection Projection, expectedChangeID string) (*Change, error) {
	change := projection.ActiveChange()
	if change == nil || change.ChangeID != expectedChangeID {
		return nil, newError(KindConflict, "ACTIVE_CHANGE_MISMATCH", "the active Change no longer matches --change", nil)
	}
	return change, nil
}

func requireExpectedAuthorityWorkspace(service Service, workspace loadedWorkspace, expectedAuthorityID, expectedWorkspaceID string) error {
	if service.authorityID != expectedAuthorityID {
		return newError(KindConflict, "AUTHORITY_ID_MISMATCH", "the selected authority state no longer matches --authority", nil)
	}
	if workspace.WorkspaceID != expectedWorkspaceID {
		return newError(KindConflict, "WORKSPACE_ID_MISMATCH", "the target Workspace no longer matches --workspace", nil)
	}
	return nil
}

func (workspace loadedWorkspace) effectiveConfig() ConfigBundle {
	accepted := workspace.Projection.AcceptedConfig
	if accepted == nil {
		return workspace.Config
	}
	effective := ConfigBundle{
		Root:    workspace.Root,
		Project: accepted.Project,
		Policy:  accepted.Policy,
		Gates:   accepted.Gates,
		Digest:  accepted.ConfigDigest,
	}
	if workspace.Projection.AcceptedTruth != nil {
		effective.Truth = workspace.Projection.AcceptedTruth.Truth
		effective.TruthFiles = workspace.Projection.AcceptedTruth.Files
		effective.TruthDigest = workspace.Projection.AcceptedTruth.TruthDigest
	} else {
		effective.Truth = workspace.Config.Truth
		effective.TruthFiles = workspace.Config.TruthFiles
		effective.TruthDigest = workspace.Config.TruthDigest
	}
	return effective
}

func acceptedTruthAtDigest(projection Projection, digest string) (ProjectTruthConfig, error) {
	for i := len(projection.TruthHistory) - 1; i >= 0; i-- {
		if projection.TruthHistory[i].TruthDigest == digest {
			return projection.TruthHistory[i].Truth, nil
		}
	}
	return ProjectTruthConfig{}, newError(KindIntegrity, "CHANGE_START_TRUTH_MISSING", "the Project Truth epoch bound to the Change is missing from authority history", nil)
}

func latestChange(projection Projection) *Change {
	if len(projection.ChangeOrder) == 0 {
		return nil
	}
	change := projection.Changes[projection.ChangeOrder[len(projection.ChangeOrder)-1]]
	if change == nil {
		return nil
	}
	copyOfChange := *change
	return &copyOfChange
}

func normalizeUniquePathRoots(values []string) ([]string, error) {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized, err := normalizePathRoot(value)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	sort.Strings(result)
	return result, nil
}

func normalizeTextList(values []string, required bool) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if len(value) > 2000 {
			return nil, newError(KindUsage, "CONTRACT_TEXT_TOO_LONG", "contract list item exceeds 2000 characters", nil)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if required && len(result) == 0 {
		return nil, newError(KindUsage, "CONTRACT_LIST_REQUIRED", "at least one non-empty contract item is required", nil)
	}
	return result, nil
}

func effectiveRisk(change Change, touched []string, policy PolicyConfig) Risk {
	risk := change.DeclaredRisk
	for _, path := range touched {
		for _, rule := range policy.RiskRules {
			// A submodule is represented as one touched mount path even though its
			// recursive content participates in the source fingerprint. Treat an
			// overlapping descendant rule as matching that collapsed path so a
			// nested high-risk area cannot be silently under-classified.
			if containsPath(rule.PathRoot, path) || containsPath(path, rule.PathRoot) {
				risk = MaxRisk(risk, rule.Risk)
			}
		}
	}
	return risk
}

func effectiveRiskForChange(change Change, touched []string, policy PolicyConfig, startingTruth, currentTruth ProjectTruthConfig, truthChanged bool) Risk {
	risk := effectiveRisk(change, touched, policy)
	startingImpact := effectiveChangeImpact(change.Impact, inferImpact(touched, startingTruth))
	currentImpact := effectiveChangeImpact(change.Impact, inferImpact(touched, currentTruth))
	risk = MaxRisk(risk, impactTruthRisk(startingImpact, startingTruth, truthChanged))
	risk = MaxRisk(risk, impactTruthRisk(currentImpact, currentTruth, truthChanged))
	return risk
}

func effectiveRiskForChangeLegacy(change Change, touched []string, policy PolicyConfig, startingTruth, currentTruth ProjectTruthConfig, truthChanged bool) Risk {
	risk := effectiveRisk(change, touched, policy)
	risk = MaxRisk(risk, impactTruthRisk(change.Impact, startingTruth, truthChanged))
	risk = MaxRisk(risk, impactTruthRisk(change.Impact, currentTruth, truthChanged))
	return risk
}

func effectiveRiskForContract(change Change, touched []string, policy PolicyConfig, startingTruth, currentTruth ProjectTruthConfig, truthChanged bool) Risk {
	if change.ContractVersion == 0 {
		return effectiveRiskForChangeLegacy(change, touched, policy, startingTruth, currentTruth, truthChanged)
	}
	return effectiveRiskForChange(change, touched, policy, startingTruth, currentTruth, truthChanged)
}

func inferImpact(paths []string, truth ProjectTruthConfig) InferredImpact {
	components := make(map[string]struct{})
	unmapped := make(map[string]struct{})
	for _, path := range paths {
		matched := false
		for _, component := range truth.Components {
			for _, root := range component.PathRoots {
				if containsPath(root, path) || containsPath(path, root) {
					components[component.ID] = struct{}{}
					matched = true
					break
				}
			}
		}
		if !matched && truth.Maturity == TruthMaturityEstablished {
			unmapped[path] = struct{}{}
		}
	}
	// A change to a component can affect every component that directly or
	// transitively depends on it. Expand the reverse dependency closure before
	// resolving capabilities/invariants so declared architecture relationships
	// have executable consequences without selecting unrelated components.
	for changed := true; changed; {
		changed = false
		for _, component := range truth.Components {
			if _, alreadyIncluded := components[component.ID]; alreadyIncluded {
				continue
			}
			for _, dependencyID := range component.DependsOn {
				if _, dependencyAffected := components[dependencyID]; dependencyAffected {
					components[component.ID] = struct{}{}
					changed = true
					break
				}
			}
		}
	}
	capabilities := make(map[string]struct{})
	invariants := make(map[string]struct{})
	for _, capability := range truth.Capabilities {
		for _, componentID := range capability.ComponentIDs {
			if _, ok := components[componentID]; ok {
				capabilities[capability.ID] = struct{}{}
				for _, invariantID := range capability.InvariantIDs {
					invariants[invariantID] = struct{}{}
				}
				break
			}
		}
	}
	return InferredImpact{
		ComponentIDs:  sortedSetKeys(components),
		CapabilityIDs: sortedSetKeys(capabilities),
		InvariantIDs:  sortedSetKeys(invariants),
		UnmappedPaths: sortedSetKeys(unmapped),
	}
}

func sortedSetKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func effectiveChangeImpact(declared ChangeImpact, inferred InferredImpact) ChangeImpact {
	declared.ComponentIDs = unionSorted(declared.ComponentIDs, inferred.ComponentIDs)
	declared.CapabilityIDs = unionSorted(declared.CapabilityIDs, inferred.CapabilityIDs)
	declared.InvariantIDs = unionSorted(declared.InvariantIDs, inferred.InvariantIDs)
	return declared
}

func unionSorted(left, right []string) []string {
	values := make(map[string]struct{}, len(left)+len(right))
	for _, value := range left {
		values[value] = struct{}{}
	}
	for _, value := range right {
		values[value] = struct{}{}
	}
	return sortedSetKeys(values)
}

func undeclaredInferredImpact(declared ChangeImpact, inferred InferredImpact) InferredImpact {
	missing := func(values, declaredValues []string) []string {
		declaredSet := truthStringSet(declaredValues)
		result := make([]string, 0)
		for _, value := range values {
			if _, ok := declaredSet[value]; !ok {
				result = append(result, value)
			}
		}
		return result
	}
	return InferredImpact{
		ComponentIDs:  missing(inferred.ComponentIDs, declared.ComponentIDs),
		CapabilityIDs: missing(inferred.CapabilityIDs, declared.CapabilityIDs),
		InvariantIDs:  missing(inferred.InvariantIDs, declared.InvariantIDs),
		UnmappedPaths: append([]string(nil), inferred.UnmappedPaths...),
	}
}

func undeclaredInferredImpactForChange(change Change, inferred InferredImpact, startingTruth ProjectTruthConfig, truthChanged bool) InferredImpact {
	missing := undeclaredInferredImpact(change.Impact, inferred)
	if !truthChanged || len(change.Impact.Unknowns) == 0 {
		return missing
	}
	// A concrete startup uncertainty may authorize adding a fact that had no
	// accepted ID at Change start. Semantic reconciliation separately proves
	// that the fact was actually added (not modified/removed). Do not make an
	// impossible demand that the original Change name a not-yet-existent ID.
	retainPreviouslyKnown := func(values []string, known map[string]struct{}) []string {
		result := make([]string, 0, len(values))
		for _, value := range values {
			if _, existed := known[value]; existed {
				result = append(result, value)
			}
		}
		return result
	}
	missing.ComponentIDs = retainPreviouslyKnown(missing.ComponentIDs, truthIDSetComponents(startingTruth.Components))
	missing.CapabilityIDs = retainPreviouslyKnown(missing.CapabilityIDs, truthIDSetCapabilities(startingTruth.Capabilities))
	missing.InvariantIDs = retainPreviouslyKnown(missing.InvariantIDs, truthIDSetInvariants(startingTruth.Invariants))
	return missing
}

func impactInferenceEmpty(inferred InferredImpact) bool {
	return len(inferred.ComponentIDs)+len(inferred.CapabilityIDs)+len(inferred.InvariantIDs)+len(inferred.UnmappedPaths) == 0
}

// impactTruthRefs resolves the durable facts whose risk and Gates apply to a
// Change. A Project Truth evolution is deliberately broad: all old and new
// invariants/unknowns participate so deleting or weakening a fact cannot make
// the same Change easier to pass. Project-purpose changes receive the same
// conservative treatment.
func impactTruthRefs(impact ChangeImpact, truth ProjectTruthConfig, truthChanged bool) (map[string]struct{}, map[string]struct{}) {
	invariants := make(map[string]struct{})
	unknowns := make(map[string]struct{})
	broad := truthChanged || impact.ProjectPurpose
	if broad {
		for _, invariant := range truth.Invariants {
			invariants[invariant.ID] = struct{}{}
		}
		for _, unknown := range truth.Unknowns {
			unknowns[unknown.ID] = struct{}{}
		}
		return invariants, unknowns
	}

	selectedCapabilities := truthStringSet(impact.CapabilityIDs)
	selectedComponents := truthStringSet(impact.ComponentIDs)
	for _, id := range impact.InvariantIDs {
		invariants[id] = struct{}{}
	}
	for _, id := range impact.UnknownIDs {
		unknowns[id] = struct{}{}
	}
	selectedDecisions := truthStringSet(impact.DecisionIDs)
	for _, decision := range truth.Decisions {
		if _, selected := selectedDecisions[decision.ID]; !selected {
			continue
		}
		for _, ref := range decision.AffectedRefs {
			selectedCapabilities[ref] = struct{}{}
			selectedComponents[ref] = struct{}{}
			invariants[ref] = struct{}{}
			unknowns[ref] = struct{}{}
		}
	}
	for _, capability := range truth.Capabilities {
		_, selected := selectedCapabilities[capability.ID]
		if !selected {
			for _, componentID := range capability.ComponentIDs {
				if _, impacted := selectedComponents[componentID]; impacted {
					selected = true
					break
				}
			}
		}
		if selected {
			for _, invariantID := range capability.InvariantIDs {
				invariants[invariantID] = struct{}{}
			}
		}
	}
	return invariants, unknowns
}

func impactTruthRisk(impact ChangeImpact, truth ProjectTruthConfig, truthChanged bool) Risk {
	invariantIDs, unknownIDs := impactTruthRefs(impact, truth, truthChanged)
	var risk Risk
	for _, invariant := range truth.Invariants {
		if _, applies := invariantIDs[invariant.ID]; applies {
			risk = MaxRisk(risk, invariant.Risk)
		}
	}
	for _, unknown := range truth.Unknowns {
		if _, applies := unknownIDs[unknown.ID]; applies {
			risk = MaxRisk(risk, unknown.Risk)
		}
	}
	return risk
}

func requiredGates(config GatesConfig, risk Risk) []GateConfig {
	result := make([]GateConfig, 0)
	for _, gate := range config.Gates {
		for _, requiredRisk := range gate.RequiredFor {
			if requiredRisk == risk {
				result = append(result, gate)
				break
			}
		}
	}
	sortGates(result)
	return result
}

func gateTierRank(tier GateTier) int {
	switch effectiveGateTier(tier) {
	case GateTierFast:
		return 1
	case GateTierAffected:
		return 2
	case GateTierFull:
		return 3
	default:
		return 4
	}
}

func sortGates(gates []GateConfig) {
	sort.Slice(gates, func(i, j int) bool {
		leftRank := gateTierRank(gates[i].Tier)
		rightRank := gateTierRank(gates[j].Tier)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		return gates[i].ID < gates[j].ID
	})
}

func requiredGatesForChange(config GatesConfig, startingTruth, currentTruth ProjectTruthConfig, change Change, risk Risk, truthChanged bool, paths []string) []GateConfig {
	if change.ContractVersion == 0 {
		return requiredGatesForLegacyChange(config, startingTruth, currentTruth, change, risk, truthChanged)
	}
	wanted := make(map[string]struct{})
	inferred := inferImpact(paths, currentTruth)
	effectiveImpact := effectiveChangeImpact(change.Impact, inferred)
	for _, gate := range requiredGates(config, risk) {
		if gateAppliesToChange(gate, paths, effectiveImpact) {
			wanted[gate.ID] = struct{}{}
		}
	}
	for _, truth := range []ProjectTruthConfig{startingTruth, currentTruth} {
		truthImpact := effectiveChangeImpact(change.Impact, inferImpact(paths, truth))
		invariantIDs, _ := impactTruthRefs(truthImpact, truth, truthChanged)
		for _, invariant := range truth.Invariants {
			if _, applies := invariantIDs[invariant.ID]; !applies {
				continue
			}
			for _, gateID := range invariant.GateIDs {
				wanted[gateID] = struct{}{}
			}
		}
	}
	for _, requirement := range change.Requirements {
		for _, gateID := range requirement.RequiredGateIDs {
			wanted[gateID] = struct{}{}
		}
	}
	result := make([]GateConfig, 0, len(wanted))
	for _, gate := range config.Gates {
		if _, required := wanted[gate.ID]; required {
			result = append(result, gate)
		}
	}
	sortGates(result)
	return result
}

func requiredGatesForLegacyChange(config GatesConfig, startingTruth, currentTruth ProjectTruthConfig, change Change, risk Risk, truthChanged bool) []GateConfig {
	wanted := make(map[string]struct{})
	for _, gate := range requiredGates(config, risk) {
		wanted[gate.ID] = struct{}{}
	}
	for _, truth := range []ProjectTruthConfig{startingTruth, currentTruth} {
		invariantIDs, _ := impactTruthRefs(change.Impact, truth, truthChanged)
		for _, invariant := range truth.Invariants {
			if _, applies := invariantIDs[invariant.ID]; !applies {
				continue
			}
			for _, gateID := range invariant.GateIDs {
				wanted[gateID] = struct{}{}
			}
		}
	}
	result := make([]GateConfig, 0, len(wanted))
	for _, gate := range config.Gates {
		if _, required := wanted[gate.ID]; required {
			result = append(result, gate)
		}
	}
	sortGates(result)
	return result
}

func gateAppliesToChange(gate GateConfig, paths []string, impact ChangeImpact) bool {
	if len(gate.PathRoots)+len(gate.ComponentIDs)+len(gate.CapabilityIDs)+len(gate.InvariantIDs) == 0 {
		return true
	}
	for _, gateRoot := range gate.PathRoots {
		for _, path := range paths {
			if containsPath(gateRoot, path) || containsPath(path, gateRoot) {
				return true
			}
		}
	}
	intersects := func(left, right []string) bool {
		values := truthStringSet(right)
		for _, value := range left {
			if _, ok := values[value]; ok {
				return true
			}
		}
		return false
	}
	return intersects(gate.ComponentIDs, impact.ComponentIDs) || intersects(gate.CapabilityIDs, impact.CapabilityIDs) || intersects(gate.InvariantIDs, impact.InvariantIDs)
}

func effectiveGateTier(tier GateTier) GateTier {
	if tier == "" {
		return GateTierAffected
	}
	return tier
}

func reachableRisks(declared Risk, scope []string, policy PolicyConfig) []Risk {
	seen := map[Risk]struct{}{declared: {}}
	for _, rule := range policy.RiskRules {
		overlaps := false
		for _, scopeRoot := range scope {
			if containsPath(scopeRoot, rule.PathRoot) || containsPath(rule.PathRoot, scopeRoot) {
				overlaps = true
				break
			}
		}
		if overlaps {
			seen[MaxRisk(declared, rule.Risk)] = struct{}{}
		}
	}
	result := make([]Risk, 0, len(seen))
	for risk := range seen {
		result = append(result, risk)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Rank() < result[j].Rank() })
	return result
}

func selectGates(required []GateConfig, selected []string) ([]GateConfig, error) {
	if len(selected) == 0 {
		return required, nil
	}
	wanted := make(map[string]struct{})
	for _, id := range selected {
		if err := validateIdentifier(id, "gate id"); err != nil {
			return nil, err
		}
		wanted[id] = struct{}{}
	}
	result := make([]GateConfig, 0, len(wanted))
	for _, gate := range required {
		if _, ok := wanted[gate.ID]; ok {
			result = append(result, gate)
			delete(wanted, gate.ID)
		}
	}
	if len(wanted) > 0 {
		missing := make([]string, 0, len(wanted))
		for id := range wanted {
			missing = append(missing, id)
		}
		sort.Strings(missing)
		return nil, newError(KindUsage, "UNKNOWN_GATE", "unknown Gate IDs: "+strings.Join(missing, ", "), nil)
	}
	sortGates(result)
	return result, nil
}

func pathInAnyScope(scope []string, path string) bool {
	for _, root := range scope {
		if containsPath(root, path) {
			return true
		}
	}
	return false
}

func acknowledgementRequired(policy PolicyConfig, risk Risk) bool {
	for _, value := range policy.AcknowledgementRisks {
		if value == risk {
			return true
		}
	}
	return false
}

func hasEvidenceForGate(evidence []Evidence, gateID string) bool {
	for _, item := range evidence {
		if item.GateID == gateID {
			return true
		}
	}
	return false
}

func onlyAcknowledgementBlocks(reasons []VerdictReason) bool {
	if len(reasons) == 0 {
		return false
	}
	for _, reason := range reasons {
		if reason.Code != "ACKNOWLEDGEMENT_REQUIRED" {
			return false
		}
	}
	return true
}
