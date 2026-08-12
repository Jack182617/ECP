package ecp

import (
	"encoding/json"
	"time"
)

const (
	SchemaVersion = 1
	CoreVersion   = "0.3.0-dev"
)

type Risk string

const (
	RiskLow      Risk = "low"
	RiskModerate Risk = "moderate"
	RiskHigh     Risk = "high"
	RiskCritical Risk = "critical"
)

func (r Risk) Rank() int {
	switch r {
	case RiskLow:
		return 1
	case RiskModerate:
		return 2
	case RiskHigh:
		return 3
	case RiskCritical:
		return 4
	default:
		return 0
	}
}

func MaxRisk(a, b Risk) Risk {
	if b.Rank() > a.Rank() {
		return b
	}
	return a
}

type ProjectConfig struct {
	SchemaVersion int       `json:"schema_version"`
	ProjectID     string    `json:"project_id"`
	Name          string    `json:"name"`
	CreatedAt     time.Time `json:"created_at"`
}

type TruthMaturity string

const (
	TruthMaturitySeed        TruthMaturity = "seed"
	TruthMaturityEstablished TruthMaturity = "established"
)

// ProjectTruthConfig is the durable, project-specific knowledge that a future
// agent must understand before it can safely change the repository. It is
// deliberately structured and bounded; long-form detail remains in referenced
// contract documents and source files.
type ProjectTruthConfig struct {
	SchemaVersion int                `json:"schema_version"`
	Maturity      TruthMaturity      `json:"maturity"`
	Purpose       string             `json:"purpose"`
	Capabilities  []TruthCapability  `json:"capabilities"`
	Invariants    []TruthInvariant   `json:"invariants"`
	Components    []TruthComponent   `json:"components"`
	Decisions     []TruthDecision    `json:"decisions"`
	Contracts     []TruthContractRef `json:"contracts"`
	Unknowns      []TruthUnknown     `json:"unknowns"`
}

type TruthCapability struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Status       string   `json:"status"`
	ComponentIDs []string `json:"component_ids"`
	InvariantIDs []string `json:"invariant_ids"`
}

type TruthInvariant struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Statement  string   `json:"statement"`
	Category   string   `json:"category"`
	Risk       Risk     `json:"risk"`
	GateIDs    []string `json:"gate_ids"`
	SourceRefs []string `json:"source_refs"`
}

type TruthComponent struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Responsibility string   `json:"responsibility"`
	PathRoots      []string `json:"path_roots"`
	DependsOn      []string `json:"depends_on"`
}

type TruthDecision struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Status       string   `json:"status"`
	Decision     string   `json:"decision"`
	Rationale    string   `json:"rationale"`
	Supersedes   []string `json:"supersedes"`
	AffectedRefs []string `json:"affected_refs"`
}

type TruthContractRef struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

type TruthUnknown struct {
	ID                  string `json:"id"`
	Statement           string `json:"statement"`
	Risk                Risk   `json:"risk"`
	ResolutionCondition string `json:"resolution_condition"`
}

type TruthFileDigest struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

// TruthFileContent is the exact candidate truth file payload read from one
// stable config epoch. It is internal transport for the authority blob store
// and is deliberately omitted from generic ConfigBundle JSON output.
type TruthFileContent struct {
	Path    string `json:"path"`
	Digest  string `json:"digest"`
	Content []byte `json:"-"`
}

type AcceptedTruthFile struct {
	Path    string `json:"path"`
	Digest  string `json:"digest"`
	Content string `json:"content"`
}

type PolicyConfig struct {
	SchemaVersion        int            `json:"schema_version"`
	DefaultRisk          Risk           `json:"default_risk"`
	AcknowledgementRisks []Risk         `json:"acknowledgement_required_for"`
	DeniedPathRoots      []string       `json:"denied_path_roots"`
	RiskRules            []PathRiskRule `json:"risk_rules"`
	InheritedEnvironment []string       `json:"inherited_environment"`
	MaxGateOutputBytes   int64          `json:"max_gate_output_bytes"`
	MaxSourceFiles       int            `json:"max_source_files"`
	MaxSourceBytes       int64          `json:"max_source_bytes"`
}

type PathRiskRule struct {
	PathRoot string `json:"path_root"`
	Risk     Risk   `json:"risk"`
}

type GatesConfig struct {
	SchemaVersion int          `json:"schema_version"`
	Gates         []GateConfig `json:"gates"`
}

type GateConfig struct {
	ID                  string            `json:"id"`
	Description         string            `json:"description"`
	Tier                GateTier          `json:"tier,omitempty"`
	Command             []string          `json:"command"`
	WorkingDirectory    string            `json:"working_directory"`
	TimeoutSeconds      int               `json:"timeout_seconds"`
	AllowedExitCodes    []int             `json:"allowed_exit_codes"`
	RequiredFor         []Risk            `json:"required_for"`
	PathRoots           []string          `json:"path_roots,omitempty"`
	ComponentIDs        []string          `json:"component_ids,omitempty"`
	CapabilityIDs       []string          `json:"capability_ids,omitempty"`
	InvariantIDs        []string          `json:"invariant_ids,omitempty"`
	Environment         map[string]string `json:"environment"`
	InheritEnvironment  []string          `json:"inherit_environment"`
	MaxOutputBytes      int64             `json:"max_output_bytes,omitempty"`
	RequiresNetwork     bool              `json:"requires_network"`
	ProducesSideEffects bool              `json:"produces_external_side_effects"`
}

type GateTier string

const (
	GateTierFast     GateTier = "fast"
	GateTierAffected GateTier = "affected"
	GateTierFull     GateTier = "full"
)

type ConfigBundle struct {
	Root         string             `json:"root"`
	Project      ProjectConfig      `json:"project"`
	Policy       PolicyConfig       `json:"policy"`
	Gates        GatesConfig        `json:"gates"`
	Truth        ProjectTruthConfig `json:"truth"`
	TruthFiles   []TruthFileDigest  `json:"truth_files"`
	TruthContent []TruthFileContent `json:"-"`
	Digest       string             `json:"config_digest"`
	TruthDigest  string             `json:"truth_digest"`
	BundleDigest string             `json:"bundle_digest"`
}

type FileEntry struct {
	Path          string `json:"path"`
	HeadMode      string `json:"head_mode,omitempty"`
	HeadObject    string `json:"head_object,omitempty"`
	IndexMode     string `json:"index_mode,omitempty"`
	IndexObject   string `json:"index_object,omitempty"`
	IndexStage    string `json:"index_stage,omitempty"`
	IndexState    string `json:"index_state,omitempty"`
	WorktreeType  string `json:"worktree_type"`
	WorktreeMode  uint32 `json:"worktree_mode,omitempty"`
	ContentDigest string `json:"content_digest,omitempty"`
	Status        string `json:"status,omitempty"`
}

type SourceSnapshot struct {
	SchemaVersion       int         `json:"schema_version"`
	Head                string      `json:"head"`
	Branch              string      `json:"branch"`
	GitExecutable       string      `json:"git_executable"`
	GitExecutableDigest string      `json:"git_executable_digest"`
	Fingerprint         string      `json:"source_fingerprint"`
	Entries             []FileEntry `json:"entries"`
}

type SnapshotRef struct {
	Head                string `json:"head"`
	Branch              string `json:"branch"`
	GitExecutable       string `json:"git_executable"`
	GitExecutableDigest string `json:"git_executable_digest"`
	Fingerprint         string `json:"source_fingerprint"`
}

func (s SourceSnapshot) Ref() SnapshotRef {
	return SnapshotRef{Head: s.Head, Branch: s.Branch, GitExecutable: s.GitExecutable, GitExecutableDigest: s.GitExecutableDigest, Fingerprint: s.Fingerprint}
}

type ChangeState string

const (
	ChangeActive    ChangeState = "ACTIVE"
	ChangeCompleted ChangeState = "COMPLETED"
	ChangeCancelled ChangeState = "CANCELLED"
)

type Change struct {
	SchemaVersion       int                 `json:"schema_version"`
	ContractVersion     int                 `json:"contract_version,omitempty"`
	ChangeID            string              `json:"change_id"`
	ActivationID        string              `json:"activation_id"`
	Title               string              `json:"title"`
	Goal                string              `json:"goal"`
	Scope               []string            `json:"scope"`
	NonGoals            []string            `json:"non_goals"`
	AcceptanceCriteria  []string            `json:"acceptance_criteria"`
	Impact              ChangeImpact        `json:"impact"`
	Requirements        []ChangeRequirement `json:"requirements,omitempty"`
	SupersedesChangeID  string              `json:"supersedes_change_id,omitempty"`
	LineageRootChangeID string              `json:"lineage_root_change_id,omitempty"`
	DeclaredRisk        Risk                `json:"declared_risk"`
	ContractDigest      string              `json:"contract_digest"`
	ConfigDigest        string              `json:"config_digest"`
	TruthDigest         string              `json:"truth_digest"`
	Baseline            SourceSnapshot      `json:"baseline"`
	State               ChangeState         `json:"state"`
	CreatedAt           time.Time           `json:"created_at"`
}

// ChangeImpact is a compact impact hypothesis, not a second requirements
// document. References are checked against the accepted Project Truth.
type ChangeImpact struct {
	ProjectPurpose        bool     `json:"project_purpose"`
	CapabilityIDs         []string `json:"capability_ids"`
	InvariantIDs          []string `json:"invariant_ids"`
	ComponentIDs          []string `json:"component_ids"`
	DecisionIDs           []string `json:"decision_ids"`
	ContractIDs           []string `json:"contract_ids"`
	UnknownIDs            []string `json:"unknown_ids"`
	UserJourneys          []string `json:"user_journeys"`
	DataEffects           []string `json:"data_effects"`
	OperationalEffects    []string `json:"operational_effects"`
	ExpectedChanges       []string `json:"expected_changes"`
	ExpectedPreservations []string `json:"expected_preservations"`
	Unknowns              []string `json:"unknowns"`
}

type ChangeRequirementStatus string

const (
	RequirementDecided         ChangeRequirementStatus = "DECIDED"
	RequirementNotApplicable   ChangeRequirementStatus = "NOT_APPLICABLE"
	RequirementDeferredSafe    ChangeRequirementStatus = "DEFERRED_SAFE"
	RequirementBlockingUnknown ChangeRequirementStatus = "BLOCKING_UNKNOWN"
)

type RequirementVerification string

const (
	RequirementVerificationAutomated RequirementVerification = "AUTOMATED"
	RequirementVerificationReview    RequirementVerification = "REVIEW"
	RequirementVerificationExternal  RequirementVerification = "EXTERNAL"
)

// RequirementCoverage binds a decision to exact Change contract items. Values
// are exact strings from the corresponding Change or ChangeImpact list; this
// keeps coverage machine-checkable without duplicating the requirement text.
type RequirementCoverage struct {
	AcceptanceCriteria    []string `json:"acceptance_criteria,omitempty"`
	UserJourneys          []string `json:"user_journeys,omitempty"`
	DataEffects           []string `json:"data_effects,omitempty"`
	OperationalEffects    []string `json:"operational_effects,omitempty"`
	ExpectedChanges       []string `json:"expected_changes,omitempty"`
	ExpectedPreservations []string `json:"expected_preservations,omitempty"`
	Unknowns              []string `json:"unknowns,omitempty"`
}

// ChangeRequirement is the bounded decision ledger for one material behavior,
// state, failure, data, operational, or compatibility question discovered
// before implementation. BLOCKING_UNKNOWN is representable for adapters and
// drafts, but Core refuses to start a Change containing one.
type ChangeRequirement struct {
	ID               string                  `json:"id"`
	Statement        string                  `json:"statement"`
	Status           ChangeRequirementStatus `json:"status"`
	Verification     RequirementVerification `json:"verification"`
	Rationale        string                  `json:"rationale"`
	DecisionSource   string                  `json:"decision_source"`
	RevisitCondition string                  `json:"revisit_condition,omitempty"`
	RequiredGateIDs  []string                `json:"required_gate_ids,omitempty"`
	Covers           RequirementCoverage     `json:"covers"`
}

type RequirementOutcome string

const (
	RequirementOutcomeVerified        RequirementOutcome = "VERIFIED"
	RequirementOutcomeNotApplicable   RequirementOutcome = "NOT_APPLICABLE"
	RequirementOutcomeDeferredSafe    RequirementOutcome = "DEFERRED_SAFE"
	RequirementOutcomeExternalPending RequirementOutcome = "EXTERNAL_PENDING"
)

type RequirementAssessment struct {
	RequirementID   string             `json:"requirement_id"`
	Outcome         RequirementOutcome `json:"outcome"`
	EvidenceGateIDs []string           `json:"evidence_gate_ids,omitempty"`
	Summary         string             `json:"summary"`
}

type InferredImpact struct {
	ComponentIDs  []string `json:"component_ids"`
	CapabilityIDs []string `json:"capability_ids"`
	InvariantIDs  []string `json:"invariant_ids"`
	UnmappedPaths []string `json:"unmapped_paths"`
}

type Evidence struct {
	SchemaVersion            int           `json:"schema_version"`
	EvidenceID               string        `json:"evidence_id"`
	ChangeID                 string        `json:"change_id"`
	GateRunID                string        `json:"gate_run_id,omitempty"`
	ActivationID             string        `json:"activation_id"`
	GateID                   string        `json:"gate_id"`
	PlanDigest               string        `json:"plan_digest"`
	GateDigest               string        `json:"gate_digest"`
	ContractDigest           string        `json:"contract_digest"`
	ConfigDigest             string        `json:"config_digest"`
	TruthDigest              string        `json:"truth_digest"`
	ProjectID                string        `json:"project_id"`
	AuthorityID              string        `json:"authority_id"`
	WorkspaceID              string        `json:"workspace_id"`
	PreSnapshot              SnapshotRef   `json:"pre_snapshot"`
	PostSnapshot             SnapshotRef   `json:"post_snapshot"`
	Command                  []string      `json:"command"`
	WorkingDirectory         string        `json:"working_directory"`
	ResolvedExecutable       string        `json:"resolved_executable"`
	ResolvedExecutableDigest string        `json:"resolved_executable_digest"`
	EnvironmentNames         []string      `json:"environment_names"`
	EnvironmentDigest        string        `json:"environment_digest"`
	StartedAt                time.Time     `json:"started_at"`
	FinishedAt               time.Time     `json:"finished_at"`
	Duration                 time.Duration `json:"duration_ns"`
	ExitCode                 int           `json:"exit_code"`
	ProcessError             string        `json:"process_error,omitempty"`
	TimedOut                 bool          `json:"timed_out"`
	SourceMutated            bool          `json:"source_mutated"`
	ExitCodeAllowed          bool          `json:"exit_code_allowed"`
	StdoutDigest             string        `json:"stdout_digest"`
	StderrDigest             string        `json:"stderr_digest"`
	StdoutStoredDigest       string        `json:"stdout_stored_digest"`
	StderrStoredDigest       string        `json:"stderr_stored_digest"`
	StdoutBytes              int64         `json:"stdout_bytes"`
	StderrBytes              int64         `json:"stderr_bytes"`
	StdoutTruncated          bool          `json:"stdout_truncated"`
	StderrTruncated          bool          `json:"stderr_truncated"`
	StdoutArtifact           string        `json:"stdout_artifact"`
	StderrArtifact           string        `json:"stderr_artifact"`
	RunnerVersion            string        `json:"runner_version"`
	Scope                    string        `json:"scope"`
}

func (e Evidence) Passed() bool {
	return e.ExitCodeAllowed && !e.TimedOut && !e.SourceMutated && e.ProcessError == ""
}

type Acknowledgement struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"acknowledgement_id"`
	ChangeID      string    `json:"change_id"`
	ActivationID  string    `json:"activation_id"`
	SubjectDigest string    `json:"subject_digest"`
	Actor         string    `json:"actor"`
	Reason        string    `json:"reason"`
	RecordedAt    time.Time `json:"recorded_at"`
	Trust         string    `json:"trust"`
}

type ProjectRegistration struct {
	SchemaVersion int       `json:"schema_version"`
	ProjectID     string    `json:"project_id"`
	AuthorityID   string    `json:"authority_id"`
	WorkspaceID   string    `json:"workspace_id"`
	RegisteredAt  time.Time `json:"registered_at"`
	CoreVersion   string    `json:"core_version"`
	CoreIdentity  string    `json:"core_identity"`
}

type WorkspaceBinding struct {
	SchemaVersion       int       `json:"schema_version"`
	AuthorityID         string    `json:"authority_id"`
	WorkspaceID         string    `json:"workspace_id"`
	ProjectID           string    `json:"project_id"`
	InitialConfigDigest string    `json:"initial_config_digest"`
	InitialTruthDigest  string    `json:"initial_truth_digest"`
	Actor               string    `json:"actor"`
	Reason              string    `json:"reason"`
	Trust               string    `json:"trust"`
	CoreIdentity        string    `json:"core_identity"`
	BoundAt             time.Time `json:"bound_at"`
}

// ProjectActivation is the authoritative, repository-external record of
// whether ECP governs this exact local Workspace. Repository configuration is
// deliberately not allowed to enable or disable itself.
type ProjectActivation struct {
	SchemaVersion      int       `json:"schema_version"`
	ActivationID       string    `json:"activation_id"`
	PreviousActivation string    `json:"previous_activation_id,omitempty"`
	Enabled            bool      `json:"enabled"`
	ProjectID          string    `json:"project_id"`
	AuthorityID        string    `json:"authority_id"`
	WorkspaceID        string    `json:"workspace_id"`
	ConfigDigest       string    `json:"config_digest,omitempty"`
	TruthDigest        string    `json:"truth_digest,omitempty"`
	Actor              string    `json:"actor"`
	Reason             string    `json:"reason"`
	Trust              string    `json:"trust"`
	CoreIdentity       string    `json:"core_identity"`
	ChangedAt          time.Time `json:"changed_at"`
}

type ConfigAcceptance struct {
	SchemaVersion int           `json:"schema_version"`
	ConfigDigest  string        `json:"config_digest"`
	Project       ProjectConfig `json:"effective_project"`
	Policy        PolicyConfig  `json:"effective_policy"`
	Gates         GatesConfig   `json:"effective_gates"`
	Actor         string        `json:"actor"`
	Reason        string        `json:"reason"`
	AcceptedAt    time.Time     `json:"accepted_at"`
	Trust         string        `json:"trust"`
	CoreIdentity  string        `json:"core_identity"`
}

type ControlConfigView struct {
	SchemaVersion int           `json:"schema_version"`
	ProjectID     string        `json:"project_id"`
	AuthorityID   string        `json:"authority_id"`
	WorkspaceID   string        `json:"workspace_id"`
	ConfigDigest  string        `json:"config_digest"`
	Project       ProjectConfig `json:"accepted_project"`
	Policy        PolicyConfig  `json:"accepted_policy"`
	Gates         GatesConfig   `json:"accepted_gates"`
	Actor         string        `json:"actor"`
	Reason        string        `json:"reason"`
	AcceptedAt    time.Time     `json:"accepted_at"`
	Trust         string        `json:"trust"`
}

type ProjectTruthAcceptance struct {
	SchemaVersion       int                `json:"schema_version"`
	TruthDigest         string             `json:"truth_digest"`
	PreviousTruthDigest string             `json:"previous_truth_digest,omitempty"`
	ChangeID            string             `json:"change_id,omitempty"`
	Truth               ProjectTruthConfig `json:"effective_truth"`
	Files               []TruthFileDigest  `json:"effective_truth_files"`
	Actor               string             `json:"actor"`
	Reason              string             `json:"reason"`
	AcceptedAt          time.Time          `json:"accepted_at"`
	Trust               string             `json:"trust"`
	CoreIdentity        string             `json:"core_identity"`
}

type SemanticBehavior string

const (
	SemanticBehaviorPreserved SemanticBehavior = "PRESERVED"
	SemanticBehaviorChanged   SemanticBehavior = "CHANGED"
	SemanticBehaviorUnknown   SemanticBehavior = "UNKNOWN"
)

type TruthDelta struct {
	Section   string `json:"section"`
	ID        string `json:"id"`
	Operation string `json:"operation"`
	Protected bool   `json:"protected"`
}

type SemanticAssessment struct {
	SchemaVersion          int                     `json:"schema_version"`
	AssessmentID           string                  `json:"assessment_id"`
	ChangeID               string                  `json:"change_id"`
	ActivationID           string                  `json:"activation_id"`
	PreviousTruthDigest    string                  `json:"previous_truth_digest"`
	CurrentTruthDigest     string                  `json:"current_truth_digest"`
	SourceFingerprint      string                  `json:"source_fingerprint"`
	Behavior               SemanticBehavior        `json:"behavior"`
	Summary                string                  `json:"summary"`
	Categories             []string                `json:"categories"`
	RequirementAssessments []RequirementAssessment `json:"requirement_assessments,omitempty"`
	TruthDelta             []TruthDelta            `json:"truth_delta"`
	ProtectedChange        bool                    `json:"protected_change"`
	ProtectedConfirmed     bool                    `json:"protected_confirmed"`
	Actor                  string                  `json:"actor"`
	Reason                 string                  `json:"reason"`
	RecordedAt             time.Time               `json:"recorded_at"`
	Trust                  string                  `json:"trust"`
}

type TruthDiffReport struct {
	SchemaVersion        int          `json:"schema_version"`
	ProjectID            string       `json:"project_id"`
	AuthorityID          string       `json:"authority_id"`
	WorkspaceID          string       `json:"workspace_id"`
	ChangeID             string       `json:"change_id"`
	PreviousTruthDigest  string       `json:"previous_truth_digest"`
	CandidateTruthDigest string       `json:"candidate_truth_digest"`
	Changed              bool         `json:"changed"`
	ProtectedChange      bool         `json:"protected_change"`
	Delta                []TruthDelta `json:"delta"`
}

type ProjectTruthView struct {
	SchemaVersion int                 `json:"schema_version"`
	ProjectID     string              `json:"project_id"`
	AuthorityID   string              `json:"authority_id"`
	WorkspaceID   string              `json:"workspace_id"`
	TruthDigest   string              `json:"truth_digest"`
	Truth         ProjectTruthConfig  `json:"accepted_truth"`
	Files         []AcceptedTruthFile `json:"accepted_truth_files"`
	Actor         string              `json:"actor"`
	Reason        string              `json:"reason"`
	AcceptedAt    time.Time           `json:"accepted_at"`
	ChangeID      string              `json:"change_id,omitempty"`
	Trust         string              `json:"trust"`
}

type SemanticAssessmentInput struct {
	ExpectedAuthority      string
	ExpectedWorkspace      string
	ExpectedActivation     string
	ExpectedChangeID       string
	ExpectedSource         string
	ExpectedPreviousTruth  string
	ExpectedCandidateTruth string
	Behavior               SemanticBehavior
	Summary                string
	Categories             []string
	RequirementAssessments []RequirementAssessment
	Actor                  string
	Reason                 string
	ConfirmProtected       bool
}

type ChangeCompletion struct {
	SchemaVersion int       `json:"schema_version"`
	ChangeID      string    `json:"change_id"`
	ActivationID  string    `json:"activation_id"`
	SubjectDigest string    `json:"subject_digest"`
	VerdictDigest string    `json:"verdict_digest"`
	FinalVerdict  Verdict   `json:"final_verdict"`
	CompletedAt   time.Time `json:"completed_at"`
}

type ChangeCancellation struct {
	SchemaVersion int       `json:"schema_version"`
	ChangeID      string    `json:"change_id"`
	ActivationID  string    `json:"activation_id"`
	Actor         string    `json:"actor"`
	Reason        string    `json:"reason"`
	Trust         string    `json:"trust"`
	CancelledAt   time.Time `json:"cancelled_at"`
}

type VerdictStatus string

const (
	VerdictPass          VerdictStatus = "PASS"
	VerdictBlocked       VerdictStatus = "BLOCKED"
	VerdictIndeterminate VerdictStatus = "INDETERMINATE"
)

type VerdictReason struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	GateID  string `json:"gate_id,omitempty"`
	Path    string `json:"path,omitempty"`
}

type GateAssessment struct {
	GateID     string `json:"gate_id"`
	Required   bool   `json:"required"`
	EvidenceID string `json:"evidence_id,omitempty"`
	State      string `json:"state"`
}

type Verdict struct {
	SchemaVersion        int              `json:"schema_version"`
	Status               VerdictStatus    `json:"status"`
	ProjectID            string           `json:"project_id"`
	AuthorityID          string           `json:"authority_id"`
	WorkspaceID          string           `json:"workspace_id"`
	ChangeID             string           `json:"change_id"`
	ActivationID         string           `json:"activation_id"`
	ChangeState          ChangeState      `json:"change_state"`
	Source               SnapshotRef      `json:"source"`
	ConfigDigest         string           `json:"config_digest"`
	TruthDigest          string           `json:"truth_digest"`
	ContractDigest       string           `json:"contract_digest"`
	SemanticAssessmentID string           `json:"semantic_assessment_id"`
	DeclaredRisk         Risk             `json:"declared_risk"`
	EffectiveRisk        Risk             `json:"effective_risk"`
	TouchedPaths         []string         `json:"touched_paths"`
	InferredImpact       InferredImpact   `json:"inferred_impact"`
	GateAssessments      []GateAssessment `json:"gate_assessments"`
	Acknowledgement      string           `json:"acknowledgement_id,omitempty"`
	Reasons              []VerdictReason  `json:"reasons"`
	SubjectDigest        string           `json:"subject_digest"`
	EvaluatorVersion     string           `json:"evaluator_version"`
	EvaluatedAt          time.Time        `json:"evaluated_at"`
}

type Event struct {
	SchemaVersion int             `json:"schema_version"`
	Sequence      uint64          `json:"sequence"`
	EventID       string          `json:"event_id"`
	Timestamp     time.Time       `json:"timestamp"`
	Type          string          `json:"type"`
	Origin        string          `json:"origin"`
	PreviousHash  string          `json:"previous_hash"`
	Payload       json.RawMessage `json:"payload"`
	Hash          string          `json:"hash"`
}

type Projection struct {
	Registration        *ProjectRegistration
	Activation          *ProjectActivation
	ActivationIDs       map[string]struct{}
	AcceptedConfig      *ConfigAcceptance
	ConfigHistory       []ConfigAcceptance
	AcceptedTruth       *ProjectTruthAcceptance
	TruthHistory        []ProjectTruthAcceptance
	Changes             map[string]*Change
	ChangeOrder         []string
	GateRuns            map[string]*GateRun
	GateRunOrder        []string
	Evidence            map[string][]Evidence
	Acknowledgements    map[string][]Acknowledgement
	SemanticAssessments map[string][]SemanticAssessment
	Completions         map[string]ChangeCompletion
	Cancellations       map[string]ChangeCancellation
	Revision            uint64
	EventHead           string
}

func NewProjection() Projection {
	return Projection{
		ActivationIDs:       make(map[string]struct{}),
		Changes:             make(map[string]*Change),
		GateRuns:            make(map[string]*GateRun),
		Evidence:            make(map[string][]Evidence),
		Acknowledgements:    make(map[string][]Acknowledgement),
		SemanticAssessments: make(map[string][]SemanticAssessment),
		Completions:         make(map[string]ChangeCompletion),
		Cancellations:       make(map[string]ChangeCancellation),
	}
}

func (p Projection) ActiveGateRun() *GateRun {
	for i := len(p.GateRunOrder) - 1; i >= 0; i-- {
		run := p.GateRuns[p.GateRunOrder[i]]
		if run != nil && run.State == GateRunInProgress {
			copyOfRun := *run
			copyOfRun.GateIDs = append([]string(nil), run.GateIDs...)
			copyOfRun.EvidenceIDs = append([]string(nil), run.EvidenceIDs...)
			return &copyOfRun
		}
	}
	return nil
}

func (p Projection) ActiveChange() *Change {
	for i := len(p.ChangeOrder) - 1; i >= 0; i-- {
		change := p.Changes[p.ChangeOrder[i]]
		if change != nil && change.State == ChangeActive {
			copyOfChange := *change
			return &copyOfChange
		}
	}
	return nil
}

func (p Projection) ECPEnabled() bool {
	return p.Activation != nil && p.Activation.Enabled
}

type ProjectConfigState string

const (
	ProjectConfigAbsent            ProjectConfigState = "ABSENT"
	ProjectConfigMissing           ProjectConfigState = "MISSING"
	ProjectConfigInvalid           ProjectConfigState = "INVALID"
	ProjectConfigUnregistered      ProjectConfigState = "UNREGISTERED"
	ProjectConfigPendingAcceptance ProjectConfigState = "PENDING_ACCEPTANCE"
	ProjectConfigAccepted          ProjectConfigState = "ACCEPTED"
)

type ProjectAssurance string

const (
	ProjectAssuranceDisabled      ProjectAssurance = "DISABLED"
	ProjectAssuranceReady         ProjectAssurance = "READY"
	ProjectAssuranceActive        ProjectAssurance = "ACTIVE"
	ProjectAssuranceBlocked       ProjectAssurance = "BLOCKED"
	ProjectAssuranceIndeterminate ProjectAssurance = "INDETERMINATE"
)

// ProjectStatus is intentionally lightweight and never executes project code
// or snapshots the source tree. It is safe to query before a Workspace has
// been initialized: absence of an authority activation record means disabled.
type ProjectStatus struct {
	SchemaVersion         int                `json:"schema_version"`
	Root                  string             `json:"root"`
	AuthorityID           string             `json:"authority_id"`
	WorkspaceID           string             `json:"workspace_id"`
	ActivationID          string             `json:"activation_id,omitempty"`
	ActivationToken       string             `json:"activation_token"`
	ProjectID             string             `json:"project_id,omitempty"`
	CandidateProjectID    string             `json:"candidate_project_id,omitempty"`
	CandidateConfigDigest string             `json:"candidate_config_digest,omitempty"`
	AcceptedConfigDigest  string             `json:"accepted_config_digest,omitempty"`
	CandidateTruthDigest  string             `json:"candidate_truth_digest,omitempty"`
	AcceptedTruthDigest   string             `json:"accepted_truth_digest,omitempty"`
	TruthState            ProjectConfigState `json:"truth_state"`
	Enabled               bool               `json:"enabled"`
	Registered            bool               `json:"registered"`
	ConfigPresent         bool               `json:"config_present"`
	ConfigState           ProjectConfigState `json:"config_state"`
	Operational           bool               `json:"operational"`
	Assurance             ProjectAssurance   `json:"assurance"`
	ActiveChange          *ChangeSummary     `json:"active_change,omitempty"`
	ActiveGateRun         *GateRun           `json:"active_gate_run,omitempty"`
	Diagnostics           []VerdictReason    `json:"diagnostics"`
}

type ProjectContext struct {
	SchemaVersion      int             `json:"schema_version"`
	ECPEnabled         bool            `json:"ecp_enabled"`
	ProjectID          string          `json:"project_id"`
	AuthorityID        string          `json:"authority_id"`
	CandidateProjectID string          `json:"candidate_project_id"`
	WorkspaceID        string          `json:"workspace_id"`
	ActivationID       string          `json:"activation_id,omitempty"`
	ActivationToken    string          `json:"activation_token"`
	Root               string          `json:"root"`
	CandidateConfig    string          `json:"candidate_config_digest"`
	AcceptedConfig     string          `json:"accepted_config_digest,omitempty"`
	CandidateTruth     string          `json:"candidate_truth_digest"`
	AcceptedTruth      string          `json:"accepted_truth_digest,omitempty"`
	ConfigAccepted     bool            `json:"config_accepted"`
	TruthAccepted      bool            `json:"truth_accepted"`
	Source             SnapshotRef     `json:"source"`
	ActiveChange       *ChangeSummary  `json:"active_change,omitempty"`
	ActiveGateRun      *GateRun        `json:"active_gate_run,omitempty"`
	NextActions        []string        `json:"next_actions"`
	Assurance          string          `json:"assurance"`
	Diagnostics        []VerdictReason `json:"diagnostics"`
}

type ProjectInspection struct {
	SchemaVersion         int    `json:"schema_version"`
	Root                  string `json:"root"`
	AuthorityID           string `json:"authority_id"`
	WorkspaceID           string `json:"workspace_id"`
	CandidateProjectID    string `json:"candidate_project_id"`
	CandidateConfigDigest string `json:"candidate_config_digest"`
	CandidateTruthDigest  string `json:"candidate_truth_digest"`
	Registered            bool   `json:"registered"`
	BoundProjectID        string `json:"bound_project_id,omitempty"`
	InitialConfigDigest   string `json:"initial_config_digest,omitempty"`
	InitialTruthDigest    string `json:"initial_truth_digest,omitempty"`
}

type ChangeSummary struct {
	ChangeID            string      `json:"change_id"`
	ActivationID        string      `json:"activation_id"`
	Title               string      `json:"title"`
	State               ChangeState `json:"state"`
	DeclaredRisk        Risk        `json:"declared_risk"`
	Scope               []string    `json:"scope"`
	ConfigDigest        string      `json:"config_digest"`
	TruthDigest         string      `json:"truth_digest"`
	Baseline            SnapshotRef `json:"baseline"`
	SupersedesChangeID  string      `json:"supersedes_change_id,omitempty"`
	LineageRootChangeID string      `json:"lineage_root_change_id,omitempty"`
}

type GatePlan struct {
	SchemaVersion  int            `json:"schema_version"`
	ProjectID      string         `json:"project_id"`
	AuthorityID    string         `json:"authority_id"`
	WorkspaceID    string         `json:"workspace_id"`
	ChangeID       string         `json:"change_id"`
	ActivationID   string         `json:"activation_id"`
	ContractDigest string         `json:"contract_digest"`
	ConfigDigest   string         `json:"config_digest"`
	TruthDigest    string         `json:"truth_digest"`
	Source         SnapshotRef    `json:"source"`
	EffectiveRisk  Risk           `json:"effective_risk"`
	TouchedPaths   []string       `json:"touched_paths"`
	InferredImpact InferredImpact `json:"inferred_impact"`
	Gates          []PlannedGate  `json:"gates"`
	CoreIdentity   string         `json:"core_identity"`
	PlanDigest     string         `json:"plan_digest"`
}

type PlannedGate struct {
	ID                       string   `json:"id"`
	Description              string   `json:"description"`
	Tier                     GateTier `json:"tier"`
	Command                  []string `json:"command"`
	WorkingDirectory         string   `json:"working_directory"`
	ResolvedWorkingDirectory string   `json:"resolved_working_directory"`
	ResolvedExecutable       string   `json:"resolved_executable"`
	ResolvedExecutableDigest string   `json:"resolved_executable_digest"`
	EnvironmentNames         []string `json:"environment_names"`
	EnvironmentDigest        string   `json:"environment_digest"`
	TimeoutSeconds           int      `json:"timeout_seconds"`
	RequiresNetwork          bool     `json:"requires_network"`
	ProducesSideEffects      bool     `json:"produces_external_side_effects"`
}

type GateRunResult struct {
	SchemaVersion int          `json:"schema_version"`
	RunID         string       `json:"run_id"`
	ChangeID      string       `json:"change_id"`
	State         GateRunState `json:"state"`
	Evidence      []Evidence   `json:"evidence"`
}

type GateRunState string

const (
	GateRunInProgress  GateRunState = "IN_PROGRESS"
	GateRunCompleted   GateRunState = "COMPLETED"
	GateRunFailed      GateRunState = "FAILED"
	GateRunCancelled   GateRunState = "CANCELLED"
	GateRunInterrupted GateRunState = "INTERRUPTED"
)

type GateRun struct {
	SchemaVersion int          `json:"schema_version"`
	RunID         string       `json:"run_id"`
	ChangeID      string       `json:"change_id"`
	ActivationID  string       `json:"activation_id"`
	PlanDigest    string       `json:"plan_digest"`
	GateIDs       []string     `json:"gate_ids"`
	State         GateRunState `json:"state"`
	StartedAt     time.Time    `json:"started_at"`
	FinishedAt    time.Time    `json:"finished_at,omitempty"`
	EvidenceIDs   []string     `json:"evidence_ids"`
	OutcomeCode   string       `json:"outcome_code,omitempty"`
	Reason        string       `json:"reason,omitempty"`
}

type GateRunTerminal struct {
	SchemaVersion int          `json:"schema_version"`
	RunID         string       `json:"run_id"`
	ChangeID      string       `json:"change_id"`
	ActivationID  string       `json:"activation_id"`
	State         GateRunState `json:"state"`
	FinishedAt    time.Time    `json:"finished_at"`
	EvidenceIDs   []string     `json:"evidence_ids"`
	OutcomeCode   string       `json:"outcome_code"`
	Reason        string       `json:"reason"`
}

type GateRunReport struct {
	SchemaVersion int       `json:"schema_version"`
	ProjectID     string    `json:"project_id"`
	AuthorityID   string    `json:"authority_id"`
	WorkspaceID   string    `json:"workspace_id"`
	ChangeID      string    `json:"change_id,omitempty"`
	Runs          []GateRun `json:"runs"`
}

type ChangeHistoryItem struct {
	SchemaVersion       int                  `json:"schema_version"`
	ContractVersion     int                  `json:"contract_version,omitempty"`
	ProjectID           string               `json:"project_id"`
	AuthorityID         string               `json:"authority_id"`
	WorkspaceID         string               `json:"workspace_id"`
	ChangeID            string               `json:"change_id"`
	ActivationID        string               `json:"activation_id"`
	Title               string               `json:"title"`
	Goal                string               `json:"goal"`
	State               ChangeState          `json:"state"`
	DeclaredRisk        Risk                 `json:"declared_risk"`
	Scope               []string             `json:"scope"`
	NonGoals            []string             `json:"non_goals"`
	AcceptanceCriteria  []string             `json:"acceptance_criteria"`
	Impact              ChangeImpact         `json:"impact"`
	Requirements        []ChangeRequirement  `json:"requirements,omitempty"`
	SupersedesChangeID  string               `json:"supersedes_change_id,omitempty"`
	LineageRootChangeID string               `json:"lineage_root_change_id,omitempty"`
	ConfigDigest        string               `json:"config_digest"`
	TruthDigest         string               `json:"truth_digest"`
	ContractDigest      string               `json:"contract_digest"`
	CreatedAt           time.Time            `json:"created_at"`
	SemanticAssessments []SemanticAssessment `json:"semantic_assessments"`
	Completion          *ChangeCompletion    `json:"completion,omitempty"`
	Cancellation        *ChangeCancellation  `json:"cancellation,omitempty"`
}

type EvidenceReport struct {
	SchemaVersion int         `json:"schema_version"`
	ProjectID     string      `json:"project_id"`
	AuthorityID   string      `json:"authority_id"`
	WorkspaceID   string      `json:"workspace_id"`
	ChangeID      string      `json:"change_id"`
	ChangeState   ChangeState `json:"change_state"`
	Evidence      []Evidence  `json:"evidence"`
}
