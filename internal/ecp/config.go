package ecp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxConfigFileBytes = 1 << 20
	maxConfigTreeBytes = 10 << 20
	maxConfigTreeFiles = 1000
)

func DefaultConfig(name, projectID string, now time.Time) (ProjectConfig, PolicyConfig, GatesConfig) {
	return ProjectConfig{
			SchemaVersion: SchemaVersion,
			ProjectID:     projectID,
			Name:          name,
			CreatedAt:     now.UTC(),
		}, PolicyConfig{
			SchemaVersion:        SchemaVersion,
			DefaultRisk:          RiskModerate,
			AcknowledgementRisks: []Risk{RiskHigh, RiskCritical},
			DeniedPathRoots:      []string{".ecp"},
			RiskRules:            []PathRiskRule{},
			InheritedEnvironment: []string{"PATH", "TMPDIR", "LANG", "LC_ALL", "TERM", "CI"},
			MaxGateOutputBytes:   256 * 1024,
			MaxSourceFiles:       100000,
			MaxSourceBytes:       2 * 1024 * 1024 * 1024,
		}, GatesConfig{
			SchemaVersion: SchemaVersion,
			Gates:         []GateConfig{},
		}
}

func DefaultProjectTruth(name string) ProjectTruthConfig {
	return ProjectTruthConfig{
		SchemaVersion: SchemaVersion,
		Maturity:      TruthMaturitySeed,
		Purpose:       "The durable product purpose for " + strings.TrimSpace(name) + " has not been reviewed yet.",
		Capabilities:  []TruthCapability{},
		Invariants:    []TruthInvariant{},
		Components:    []TruthComponent{},
		Decisions:     []TruthDecision{},
		Contracts: []TruthContractRef{
			{
				ID:          "project-boundaries",
				Kind:        "boundary",
				Path:        "contracts/boundaries.md",
				Description: "Stable product, architecture, data, permission, and release boundaries.",
			},
		},
		Unknowns: []TruthUnknown{
			{
				ID:                  "project-purpose-unreviewed",
				Statement:           "The product purpose, capabilities, invariants, and component boundaries have not been established from evidence.",
				Risk:                RiskHigh,
				ResolutionCondition: "Review the repository and product intent, then replace this seed truth with an established Project Truth.",
			},
		},
	}
}

func WriteInitialConfig(root string, project ProjectConfig, policy PolicyConfig, gates GatesConfig) error {
	_, _, err := writeInitialConfigWithDigest(root, project, policy, gates)
	return err
}

func writeInitialConfigWithDigest(root string, project ProjectConfig, policy PolicyConfig, gates GatesConfig) (string, string, error) {
	target := filepath.Join(root, ".ecp")
	if _, err := os.Lstat(target); err == nil {
		return "", "", newError(KindConflict, "ECP_CONFIG_EXISTS", ".ecp already exists; initialization will not overwrite it", nil)
	} else if !os.IsNotExist(err) {
		return "", "", newError(KindRuntime, "CONFIG_STAT_FAILED", "could not inspect .ecp", err)
	}

	temp, err := os.MkdirTemp(root, ".ecp-init-")
	if err != nil {
		return "", "", newError(KindRuntime, "CONFIG_TEMP_FAILED", "could not create temporary config directory", err)
	}
	defer os.RemoveAll(temp)
	if err := os.Chmod(temp, 0o755); err != nil {
		return "", "", newError(KindRuntime, "CONFIG_CHMOD_FAILED", "could not set config directory permissions", err)
	}
	if err := os.Mkdir(filepath.Join(temp, "contracts"), 0o755); err != nil {
		return "", "", newError(KindRuntime, "CONTRACT_DIR_FAILED", "could not create contracts directory", err)
	}
	projectBytes, err := prettyJSONBytes(project)
	if err != nil {
		return "", "", err
	}
	policyBytes, err := prettyJSONBytes(policy)
	if err != nil {
		return "", "", err
	}
	gatesBytes, err := prettyJSONBytes(gates)
	if err != nil {
		return "", "", err
	}
	truthBytes, err := prettyJSONBytes(DefaultProjectTruth(project.Name))
	if err != nil {
		return "", "", err
	}
	contract := []byte("# Project Boundaries\n\nDescribe stable product, architecture, data, permission, and release boundaries here.\n")
	entries := []configTreeEntry{
		{Path: "contracts/boundaries.md", Content: contract},
		{Path: "gates.json", Content: gatesBytes},
		{Path: "policy.json", Content: policyBytes},
		{Path: "project.json", Content: projectBytes},
		{Path: "truth.json", Content: truthBytes},
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	expectedDigest, err := digestJSON(controlConfigEntries(entries))
	if err != nil {
		return "", "", newError(KindRuntime, "CONFIG_DIGEST_FAILED", "could not digest the initial configuration", err)
	}
	expectedTruthDigest, err := digestJSON(projectTruthEntries(entries))
	if err != nil {
		return "", "", newError(KindRuntime, "TRUTH_DIGEST_FAILED", "could not digest the initial Project Truth", err)
	}
	files := []struct {
		path    string
		content []byte
	}{
		{filepath.Join(temp, "project.json"), projectBytes},
		{filepath.Join(temp, "policy.json"), policyBytes},
		{filepath.Join(temp, "gates.json"), gatesBytes},
		{filepath.Join(temp, "truth.json"), truthBytes},
		{filepath.Join(temp, "contracts", "boundaries.md"), contract},
	}
	for _, file := range files {
		if err := os.WriteFile(file.path, file.content, 0o644); err != nil {
			return "", "", newError(KindRuntime, "CONFIG_WRITE_FAILED", "could not write the initial configuration", err)
		}
	}
	if err := os.Rename(temp, target); err != nil {
		return "", "", newError(KindRuntime, "CONFIG_COMMIT_FAILED", "could not atomically install .ecp", err)
	}
	return expectedDigest, expectedTruthDigest, nil
}

func LoadConfig(root string) (ConfigBundle, error) {
	return loadConfig(root, nil)
}

func loadConfig(root string, afterFirstRead func()) (ConfigBundle, error) {
	ecpDir := filepath.Join(root, ".ecp")
	first, err := readConfigTree(ecpDir)
	if err != nil {
		return ConfigBundle{}, err
	}
	if afterFirstRead != nil {
		afterFirstRead()
	}
	second, err := readConfigTree(ecpDir)
	if err != nil {
		return ConfigBundle{}, err
	}
	if first.BundleDigest != second.BundleDigest {
		return ConfigBundle{}, newError(KindConflict, "CONFIG_CHANGED_DURING_READ", ".ecp changed while Core was reading one configuration epoch", nil)
	}

	var project ProjectConfig
	var policy PolicyConfig
	var gates GatesConfig
	var truth ProjectTruthConfig
	projectBytes, ok := second.Files["project.json"]
	if !ok {
		return ConfigBundle{}, newError(KindNotFound, "CONFIG_FILE_NOT_FOUND", "required ECP config file project.json is missing", nil)
	}
	policyBytes, ok := second.Files["policy.json"]
	if !ok {
		return ConfigBundle{}, newError(KindNotFound, "CONFIG_FILE_NOT_FOUND", "required ECP config file policy.json is missing", nil)
	}
	gatesBytes, ok := second.Files["gates.json"]
	if !ok {
		return ConfigBundle{}, newError(KindNotFound, "CONFIG_FILE_NOT_FOUND", "required ECP config file gates.json is missing", nil)
	}
	truthBytes, ok := second.Files["truth.json"]
	if !ok {
		return ConfigBundle{}, newError(KindNotFound, "CONFIG_FILE_NOT_FOUND", "required ECP Project Truth file truth.json is missing", nil)
	}
	if err := decodeStrictJSON(projectBytes, &project); err != nil {
		return ConfigBundle{}, newError(KindIntegrity, "PROJECT_CONFIG_INVALID", "project.json is invalid", err)
	}
	if err := decodeStrictJSON(policyBytes, &policy); err != nil {
		return ConfigBundle{}, newError(KindIntegrity, "POLICY_CONFIG_INVALID", "policy.json is invalid", err)
	}
	if err := decodeStrictJSON(gatesBytes, &gates); err != nil {
		return ConfigBundle{}, newError(KindIntegrity, "GATES_CONFIG_INVALID", "gates.json is invalid", err)
	}
	if err := decodeStrictJSON(truthBytes, &truth); err != nil {
		return ConfigBundle{}, newError(KindIntegrity, "PROJECT_TRUTH_INVALID", "truth.json is invalid", err)
	}
	if err := validateConfig(project, policy, gates); err != nil {
		return ConfigBundle{}, err
	}
	if err := validateProjectTruth(truth, gates); err != nil {
		return ConfigBundle{}, err
	}
	if err := validateTruthFileReferences(truth, second.TruthFiles); err != nil {
		return ConfigBundle{}, err
	}
	return ConfigBundle{
		Root:         root,
		Project:      project,
		Policy:       policy,
		Gates:        gates,
		Truth:        truth,
		TruthFiles:   second.TruthFiles,
		TruthContent: second.TruthContent,
		Digest:       second.ConfigDigest,
		TruthDigest:  second.TruthDigest,
		BundleDigest: second.BundleDigest,
	}, nil
}

func validateProjectTruth(truth ProjectTruthConfig, gates GatesConfig) error {
	if truth.SchemaVersion != SchemaVersion {
		return newError(KindIntegrity, "UNSUPPORTED_TRUTH_SCHEMA", "truth.json must use the supported schema_version", nil)
	}
	if truth.Maturity != TruthMaturitySeed && truth.Maturity != TruthMaturityEstablished {
		return newError(KindIntegrity, "INVALID_TRUTH_MATURITY", "Project Truth maturity must be seed or established", nil)
	}
	if strings.TrimSpace(truth.Purpose) == "" || truth.Purpose != strings.TrimSpace(truth.Purpose) || len(truth.Purpose) > 4000 {
		return newError(KindIntegrity, "INVALID_TRUTH_PURPOSE", "Project Truth purpose is empty, non-canonical, or too long", nil)
	}
	if truth.Maturity == TruthMaturityEstablished && (len(truth.Capabilities) == 0 || len(truth.Invariants) == 0 || len(truth.Components) == 0) {
		return newError(KindIntegrity, "INCOMPLETE_ESTABLISHED_TRUTH", "established Project Truth requires at least one capability, invariant, and component", nil)
	}

	gateIDs := make(map[string]struct{}, len(gates.Gates))
	for _, gate := range gates.Gates {
		gateIDs[gate.ID] = struct{}{}
	}
	capabilityIDs := make(map[string]struct{}, len(truth.Capabilities))
	invariantIDs := make(map[string]struct{}, len(truth.Invariants))
	componentIDs := make(map[string]struct{}, len(truth.Components))
	decisionIDs := make(map[string]struct{}, len(truth.Decisions))
	contractIDs := make(map[string]struct{}, len(truth.Contracts))
	unknownIDs := make(map[string]struct{}, len(truth.Unknowns))
	allIDs := make(map[string]string)
	register := func(id, section string, sectionIDs map[string]struct{}) error {
		if err := validateIdentifier(id, section+" id"); err != nil {
			return newError(KindIntegrity, "INVALID_TRUTH_ID", fmt.Sprintf("%s contains an invalid id", section), err)
		}
		if previous, duplicate := allIDs[id]; duplicate {
			return newError(KindIntegrity, "DUPLICATE_TRUTH_ID", fmt.Sprintf("Project Truth id %q is reused by %s and %s", id, previous, section), nil)
		}
		allIDs[id] = section
		sectionIDs[id] = struct{}{}
		return nil
	}
	for _, item := range truth.Capabilities {
		if err := register(item.ID, "capability", capabilityIDs); err != nil {
			return err
		}
	}
	for _, item := range truth.Invariants {
		if err := register(item.ID, "invariant", invariantIDs); err != nil {
			return err
		}
	}
	for _, item := range truth.Components {
		if err := register(item.ID, "component", componentIDs); err != nil {
			return err
		}
	}
	for _, item := range truth.Decisions {
		if err := register(item.ID, "decision", decisionIDs); err != nil {
			return err
		}
	}
	for _, item := range truth.Contracts {
		if err := register(item.ID, "contract", contractIDs); err != nil {
			return err
		}
	}
	for _, item := range truth.Unknowns {
		if err := register(item.ID, "unknown", unknownIDs); err != nil {
			return err
		}
	}
	for _, gate := range gates.Gates {
		for _, selector := range []struct {
			values []string
			known  map[string]struct{}
		}{
			{gate.ComponentIDs, componentIDs},
			{gate.CapabilityIDs, capabilityIDs},
			{gate.InvariantIDs, invariantIDs},
		} {
			for _, id := range selector.values {
				if _, ok := selector.known[id]; !ok {
					return newError(KindIntegrity, "UNKNOWN_GATE_SELECTOR", fmt.Sprintf("gate %q references unknown selector id %q", gate.ID, id), nil)
				}
			}
		}
	}

	validText := func(value string, max int) bool {
		return value == strings.TrimSpace(value) && value != "" && len(value) <= max && !strings.ContainsRune(value, '\x00')
	}
	validRefs := func(values []string, known map[string]struct{}, label string) error {
		normalized, err := normalizeTextList(values, false)
		if err != nil || !reflect.DeepEqual(normalized, values) {
			return newError(KindIntegrity, "INVALID_TRUTH_REFERENCES", label+" references are duplicated or non-canonical", err)
		}
		for _, id := range values {
			if _, ok := known[id]; !ok {
				return newError(KindIntegrity, "UNKNOWN_TRUTH_REFERENCE", fmt.Sprintf("%s references unknown id %q", label, id), nil)
			}
		}
		return nil
	}
	for _, item := range truth.Capabilities {
		if !validText(item.Name, 300) || !validText(item.Description, 4000) || (item.Status != "active" && item.Status != "planned" && item.Status != "deprecated") {
			return newError(KindIntegrity, "INVALID_TRUTH_CAPABILITY", fmt.Sprintf("capability %q is incomplete or invalid", item.ID), nil)
		}
		if err := validRefs(item.ComponentIDs, componentIDs, "capability "+item.ID+" component_ids"); err != nil {
			return err
		}
		if err := validRefs(item.InvariantIDs, invariantIDs, "capability "+item.ID+" invariant_ids"); err != nil {
			return err
		}
	}
	validInvariantCategory := map[string]struct{}{"business": {}, "data": {}, "architecture": {}, "security": {}, "privacy": {}, "compatibility": {}, "operations": {}}
	for _, item := range truth.Invariants {
		if !validText(item.Name, 300) || !validText(item.Statement, 4000) || item.Risk.Rank() == 0 {
			return newError(KindIntegrity, "INVALID_TRUTH_INVARIANT", fmt.Sprintf("invariant %q is incomplete or invalid", item.ID), nil)
		}
		if _, ok := validInvariantCategory[item.Category]; !ok {
			return newError(KindIntegrity, "INVALID_TRUTH_INVARIANT", fmt.Sprintf("invariant %q has an invalid category", item.ID), nil)
		}
		if err := validRefs(item.GateIDs, gateIDs, "invariant "+item.ID+" gate_ids"); err != nil {
			return err
		}
		normalizedSources, err := normalizeTextList(item.SourceRefs, false)
		if err != nil || !reflect.DeepEqual(normalizedSources, item.SourceRefs) {
			return newError(KindIntegrity, "INVALID_TRUTH_SOURCE_REFS", fmt.Sprintf("invariant %q source_refs are duplicated or non-canonical", item.ID), err)
		}
	}
	for _, item := range truth.Components {
		if !validText(item.Name, 300) || !validText(item.Responsibility, 4000) {
			return newError(KindIntegrity, "INVALID_TRUTH_COMPONENT", fmt.Sprintf("component %q is incomplete", item.ID), nil)
		}
		roots, err := normalizeUniquePathRoots(item.PathRoots)
		unsafeControlPath := false
		for _, root := range roots {
			if containsPath(".ecp", root) {
				unsafeControlPath = true
				break
			}
		}
		if err != nil || len(roots) == 0 || !reflect.DeepEqual(roots, item.PathRoots) || unsafeControlPath {
			return newError(KindIntegrity, "INVALID_TRUTH_COMPONENT_PATHS", fmt.Sprintf("component %q path_roots are empty, unsafe, or non-canonical", item.ID), err)
		}
		if err := validRefs(item.DependsOn, componentIDs, "component "+item.ID+" depends_on"); err != nil {
			return err
		}
		if containsStringValue(item.DependsOn, item.ID) {
			return newError(KindIntegrity, "SELF_REFERENTIAL_COMPONENT", fmt.Sprintf("component %q depends on itself", item.ID), nil)
		}
	}
	for _, item := range truth.Decisions {
		if !validText(item.Title, 300) || !validText(item.Decision, 4000) || !validText(item.Rationale, 4000) || (item.Status != "accepted" && item.Status != "superseded" && item.Status != "proposed") {
			return newError(KindIntegrity, "INVALID_TRUTH_DECISION", fmt.Sprintf("decision %q is incomplete or invalid", item.ID), nil)
		}
		if err := validRefs(item.Supersedes, decisionIDs, "decision "+item.ID+" supersedes"); err != nil {
			return err
		}
		if err := validRefs(item.AffectedRefs, allIDsAsSet(allIDs), "decision "+item.ID+" affected_refs"); err != nil {
			return err
		}
	}
	validContractKind := map[string]struct{}{"boundary": {}, "api": {}, "data": {}, "event": {}, "permission": {}, "release": {}, "operations": {}}
	for _, item := range truth.Contracts {
		if !validText(item.Description, 4000) {
			return newError(KindIntegrity, "INVALID_TRUTH_CONTRACT", fmt.Sprintf("contract %q has no valid description", item.ID), nil)
		}
		if _, ok := validContractKind[item.Kind]; !ok {
			return newError(KindIntegrity, "INVALID_TRUTH_CONTRACT", fmt.Sprintf("contract %q has an invalid kind", item.ID), nil)
		}
		normalized, err := normalizePathRoot(item.Path)
		if err != nil || normalized != item.Path || (item.Path != "contracts" && !strings.HasPrefix(item.Path, "contracts/")) {
			return newError(KindIntegrity, "INVALID_TRUTH_CONTRACT_PATH", fmt.Sprintf("contract %q path must be canonical and under contracts/", item.ID), err)
		}
	}
	for _, item := range truth.Unknowns {
		if !validText(item.Statement, 4000) || !validText(item.ResolutionCondition, 4000) || item.Risk.Rank() == 0 {
			return newError(KindIntegrity, "INVALID_TRUTH_UNKNOWN", fmt.Sprintf("unknown %q is incomplete or invalid", item.ID), nil)
		}
	}
	return nil
}

func allIDsAsSet(values map[string]string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for id := range values {
		result[id] = struct{}{}
	}
	return result
}

func validateConfig(project ProjectConfig, policy PolicyConfig, gates GatesConfig) error {
	if project.SchemaVersion != SchemaVersion || policy.SchemaVersion != SchemaVersion || gates.SchemaVersion != SchemaVersion {
		return newError(KindIntegrity, "UNSUPPORTED_SCHEMA", "all config files must use the supported schema_version", nil)
	}
	if err := validateIdentifier(project.ProjectID, "project_id"); err != nil {
		return err
	}
	if strings.TrimSpace(project.Name) == "" || len(project.Name) > 200 {
		return newError(KindIntegrity, "INVALID_PROJECT_NAME", "project name is empty or too long", nil)
	}
	if policy.DefaultRisk.Rank() == 0 {
		return newError(KindIntegrity, "INVALID_DEFAULT_RISK", "policy default_risk is invalid", nil)
	}
	for _, risk := range policy.AcknowledgementRisks {
		if risk.Rank() == 0 {
			return newError(KindIntegrity, "INVALID_ACK_RISK", "acknowledgement risk is invalid", nil)
		}
	}
	for i, root := range policy.DeniedPathRoots {
		normalized, err := normalizePathRoot(root)
		if err != nil || normalized != root {
			return newError(KindIntegrity, "INVALID_DENIED_PATH", fmt.Sprintf("denied_path_roots[%d] is not canonical", i), err)
		}
	}
	if !containsStringValue(policy.DeniedPathRoots, ".ecp") {
		return newError(KindIntegrity, "MISSING_CORE_DENY", "denied_path_roots must retain the Core-required .ecp boundary", nil)
	}
	for i, rule := range policy.RiskRules {
		normalized, err := normalizePathRoot(rule.PathRoot)
		if err != nil || normalized != rule.PathRoot || rule.Risk.Rank() == 0 {
			return newError(KindIntegrity, "INVALID_RISK_RULE", fmt.Sprintf("risk_rules[%d] is invalid", i), err)
		}
	}
	if policy.MaxGateOutputBytes < 1024 || policy.MaxGateOutputBytes > 64*1024*1024 {
		return newError(KindIntegrity, "INVALID_OUTPUT_LIMIT", "max_gate_output_bytes must be between 1 KiB and 64 MiB", nil)
	}
	if policy.MaxSourceFiles < 1 || policy.MaxSourceFiles > 2_000_000 {
		return newError(KindIntegrity, "INVALID_SOURCE_FILE_LIMIT", "max_source_files is outside the supported range", nil)
	}
	if policy.MaxSourceBytes < 1 || policy.MaxSourceBytes > 1<<43 {
		return newError(KindIntegrity, "INVALID_SOURCE_BYTE_LIMIT", "max_source_bytes is outside the supported range", nil)
	}
	for _, name := range policy.InheritedEnvironment {
		if err := validateEnvironmentName(name); err != nil {
			return err
		}
		if isSensitiveEnvironmentName(name) {
			return newError(KindIntegrity, "SENSITIVE_ENVIRONMENT_DENIED", fmt.Sprintf("policy environment variable %q is denied by the v0.3 core", name), nil)
		}
	}
	seen := make(map[string]struct{})
	for i, gate := range gates.Gates {
		if err := validateIdentifier(gate.ID, "gate id"); err != nil {
			return err
		}
		if _, ok := seen[gate.ID]; ok {
			return newError(KindIntegrity, "DUPLICATE_GATE", fmt.Sprintf("gate id %q is duplicated", gate.ID), nil)
		}
		seen[gate.ID] = struct{}{}
		if gate.Tier != "" && gate.Tier != GateTierFast && gate.Tier != GateTierAffected && gate.Tier != GateTierFull {
			return newError(KindIntegrity, "INVALID_GATE_TIER", fmt.Sprintf("gate %q has an invalid tier", gate.ID), nil)
		}
		if len(gate.Command) == 0 || strings.TrimSpace(gate.Command[0]) == "" {
			return newError(KindIntegrity, "EMPTY_GATE_COMMAND", fmt.Sprintf("gates[%d] has no executable", i), nil)
		}
		if strings.ContainsRune(gate.Command[0], '\x00') {
			return newError(KindIntegrity, "INVALID_GATE_COMMAND", fmt.Sprintf("gates[%d] executable contains NUL", i), nil)
		}
		for _, arg := range gate.Command[1:] {
			if strings.ContainsRune(arg, '\x00') {
				return newError(KindIntegrity, "INVALID_GATE_ARGUMENT", fmt.Sprintf("gate %q contains a NUL argument", gate.ID), nil)
			}
		}
		for _, name := range gate.InheritEnvironment {
			if err := validateEnvironmentName(name); err != nil {
				return err
			}
			if isSensitiveEnvironmentName(name) {
				return newError(KindIntegrity, "SENSITIVE_ENVIRONMENT_DENIED", fmt.Sprintf("gate %q environment variable %q is denied by the v0.3 core", gate.ID, name), nil)
			}
		}
		for name, value := range gate.Environment {
			if err := validateEnvironmentName(name); err != nil {
				return err
			}
			if isSensitiveEnvironmentName(name) {
				return newError(KindIntegrity, "SENSITIVE_ENVIRONMENT_DENIED", fmt.Sprintf("gate %q explicit environment variable %q is denied by the v0.3 core", gate.ID, name), nil)
			}
			if strings.ContainsRune(value, '\x00') {
				return newError(KindIntegrity, "INVALID_ENVIRONMENT_VALUE", fmt.Sprintf("gate %q environment variable %q contains NUL", gate.ID, name), nil)
			}
		}
		cwd, err := normalizePathRoot(gate.WorkingDirectory)
		if err != nil || cwd != gate.WorkingDirectory {
			return newError(KindIntegrity, "INVALID_GATE_CWD", fmt.Sprintf("gate %q working_directory is not canonical", gate.ID), err)
		}
		if gate.TimeoutSeconds < 1 || gate.TimeoutSeconds > 86400 {
			return newError(KindIntegrity, "INVALID_GATE_TIMEOUT", fmt.Sprintf("gate %q timeout is outside the supported range", gate.ID), nil)
		}
		if len(gate.AllowedExitCodes) == 0 {
			return newError(KindIntegrity, "EMPTY_ALLOWED_EXIT_CODES", fmt.Sprintf("gate %q must declare allowed_exit_codes", gate.ID), nil)
		}
		for _, risk := range gate.RequiredFor {
			if risk.Rank() == 0 {
				return newError(KindIntegrity, "INVALID_GATE_RISK", fmt.Sprintf("gate %q has an invalid required_for risk", gate.ID), nil)
			}
		}
		roots, err := normalizeUniquePathRoots(gate.PathRoots)
		if err != nil || !slices.Equal(roots, gate.PathRoots) {
			return newError(KindIntegrity, "INVALID_GATE_PATHS", fmt.Sprintf("gate %q path_roots are duplicated or non-canonical", gate.ID), err)
		}
		for label, values := range map[string][]string{
			"component_ids":  gate.ComponentIDs,
			"capability_ids": gate.CapabilityIDs,
			"invariant_ids":  gate.InvariantIDs,
		} {
			normalized, err := normalizeTextList(values, false)
			if err != nil || !slices.Equal(normalized, values) {
				return newError(KindIntegrity, "INVALID_GATE_SELECTORS", fmt.Sprintf("gate %q %s are duplicated or non-canonical", gate.ID, label), err)
			}
		}
		seenExitCodes := make(map[int]struct{}, len(gate.AllowedExitCodes))
		for _, exitCode := range gate.AllowedExitCodes {
			if exitCode < 0 || exitCode > 255 {
				return newError(KindIntegrity, "INVALID_ALLOWED_EXIT_CODE", fmt.Sprintf("gate %q has an allowed exit code outside 0...255", gate.ID), nil)
			}
			if _, exists := seenExitCodes[exitCode]; exists {
				return newError(KindIntegrity, "DUPLICATE_ALLOWED_EXIT_CODE", fmt.Sprintf("gate %q has a duplicate allowed exit code", gate.ID), nil)
			}
			seenExitCodes[exitCode] = struct{}{}
		}
		if gate.MaxOutputBytes < 0 || gate.MaxOutputBytes > 64*1024*1024 {
			return newError(KindIntegrity, "INVALID_GATE_OUTPUT_LIMIT", fmt.Sprintf("gate %q max_output_bytes is invalid", gate.ID), nil)
		}
		if gate.ProducesSideEffects {
			return newError(KindIntegrity, "SIDE_EFFECT_GATE_UNSUPPORTED", fmt.Sprintf("gate %q declares external side effects, which v0.3 refuses", gate.ID), nil)
		}
		if gate.RequiresNetwork {
			return newError(KindIntegrity, "NETWORK_GATE_UNSUPPORTED", fmt.Sprintf("gate %q requires network access, but v0.3 has no trusted network authorization boundary", gate.ID), nil)
		}
	}
	return nil
}

func containsStringValue(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func writePrettyJSON(path string, value any, mode fs.FileMode) error {
	b, err := prettyJSONBytes(value)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, mode); err != nil {
		return newError(KindRuntime, "CONFIG_WRITE_FAILED", "could not write configuration", err)
	}
	return nil
}

func prettyJSONBytes(value any) ([]byte, error) {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, newError(KindRuntime, "JSON_ENCODE_FAILED", "could not encode configuration", err)
	}
	b = append(b, '\n')
	return b, nil
}

func decodeStrictJSON(data []byte, target any) error {
	if err := rejectDuplicateJSONKeys(data); err != nil {
		return err
	}
	if err := rejectInexactJSONFields(data, target); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func rejectInexactJSONFields(data []byte, target any) error {
	targetType := reflect.TypeOf(target)
	if targetType == nil || targetType.Kind() != reflect.Pointer || targetType.Elem().Kind() == reflect.Invalid {
		return fmt.Errorf("strict JSON target must be a non-nil pointer")
	}
	value := reflect.ValueOf(target)
	if value.IsNil() {
		return fmt.Errorf("strict JSON target must be a non-nil pointer")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return validateExactJSONShape(document, targetType.Elem(), "$")
}

var rawMessageType = reflect.TypeOf(json.RawMessage{})

func validateExactJSONShape(document any, targetType reflect.Type, path string) error {
	for targetType.Kind() == reflect.Pointer {
		targetType = targetType.Elem()
	}
	if targetType == rawMessageType || targetType.Kind() == reflect.Interface {
		return nil
	}
	switch value := document.(type) {
	case map[string]any:
		switch targetType.Kind() {
		case reflect.Struct:
			fields := exactJSONFields(targetType)
			for key, child := range value {
				fieldType, ok := fields[key]
				if !ok {
					return fmt.Errorf("unknown or inexact JSON field %q at %s", key, path)
				}
				if err := validateExactJSONShape(child, fieldType, path+"."+key); err != nil {
					return err
				}
			}
		case reflect.Map:
			for key, child := range value {
				if err := validateExactJSONShape(child, targetType.Elem(), path+"."+key); err != nil {
					return err
				}
			}
		}
	case []any:
		if targetType.Kind() == reflect.Slice || targetType.Kind() == reflect.Array {
			for index, child := range value {
				if err := validateExactJSONShape(child, targetType.Elem(), fmt.Sprintf("%s[%d]", path, index)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func exactJSONFields(targetType reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type)
	for index := 0; index < targetType.NumField(); index++ {
		field := targetType.Field(index)
		if field.PkgPath != "" {
			continue
		}
		tag := field.Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name == "-" {
			continue
		}
		if name == "" {
			if field.Anonymous {
				embedded := field.Type
				for embedded.Kind() == reflect.Pointer {
					embedded = embedded.Elem()
				}
				if embedded.Kind() == reflect.Struct {
					for embeddedName, embeddedType := range exactJSONFields(embedded) {
						fields[embeddedName] = embeddedType
					}
					continue
				}
			}
			name = field.Name
		}
		fields[name] = field.Type
	}
	return fields
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, isDelim := token.(json.Delim)
		if !isDelim {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return fmt.Errorf("object key is not a string")
				}
				if _, exists := seen[key]; exists {
					return fmt.Errorf("duplicate JSON key %q", key)
				}
				seen[key] = struct{}{}
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return fmt.Errorf("unexpected delimiter %q", delim)
		}
	}
	if err := walk(); err != nil {
		return err
	}
	if decoder.More() {
		return fmt.Errorf("trailing JSON data")
	}
	return nil
}

type configTreeEntry struct {
	Path    string `json:"path"`
	Content []byte `json:"content"`
}

type configTreeSnapshot struct {
	Files        map[string][]byte
	ConfigDigest string
	TruthDigest  string
	BundleDigest string
	TruthFiles   []TruthFileDigest
	TruthContent []TruthFileContent
}

func readConfigTree(ecpDir string) (configTreeSnapshot, error) {
	rootBefore, err := os.Lstat(ecpDir)
	if err != nil {
		if os.IsNotExist(err) {
			return configTreeSnapshot{}, newError(KindNotFound, "ECP_CONFIG_NOT_FOUND", "no .ecp directory was found", err)
		}
		return configTreeSnapshot{}, newError(KindRuntime, "CONFIG_STAT_FAILED", "could not inspect .ecp", err)
	}
	if rootBefore.Mode()&os.ModeSymlink != 0 || !rootBefore.IsDir() {
		return configTreeSnapshot{}, newError(KindIntegrity, "UNSAFE_CONFIG_ROOT", ".ecp must be a real directory, not a symlink", nil)
	}

	entries := make([]configTreeEntry, 0)
	files := make(map[string][]byte)
	var total int64
	err = filepath.WalkDir(ecpDir, func(path string, dirEntry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == ecpDir {
			return nil
		}
		rel, err := filepath.Rel(ecpDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if dirEntry.Type()&os.ModeSymlink != 0 {
			return newError(KindIntegrity, "CONFIG_SYMLINK", fmt.Sprintf("configuration path %q is a symlink", rel), nil)
		}
		if dirEntry.IsDir() {
			if rel != "contracts" && !strings.HasPrefix(rel, "contracts/") {
				return newError(KindIntegrity, "UNKNOWN_CONFIG_DIRECTORY", fmt.Sprintf("unsupported configuration directory %q", rel), nil)
			}
			return nil
		}
		if !dirEntry.Type().IsRegular() {
			return newError(KindIntegrity, "UNSAFE_CONFIG_ENTRY", fmt.Sprintf("configuration path %q is not a regular file", rel), nil)
		}
		if rel != "project.json" && rel != "policy.json" && rel != "gates.json" && rel != "truth.json" && !strings.HasPrefix(rel, "contracts/") {
			return newError(KindIntegrity, "UNKNOWN_CONFIG_FILE", fmt.Sprintf("unsupported configuration file %q", rel), nil)
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return newError(KindIntegrity, "UNSAFE_CONFIG_ENTRY", fmt.Sprintf("configuration path %q changed type while being read", rel), nil)
		}
		if info.Size() > maxConfigFileBytes {
			return newError(KindIntegrity, "CONFIG_FILE_TOO_LARGE", fmt.Sprintf("configuration file %q exceeds the size limit", rel), nil)
		}
		if len(entries)+1 > maxConfigTreeFiles {
			return newError(KindIntegrity, "CONFIG_TREE_TOO_LARGE", "configuration tree exceeds safety limits", nil)
		}
		content, err := readStableRegularFile(path, info, maxConfigFileBytes)
		if err != nil {
			return err
		}
		total += int64(len(content))
		if total > maxConfigTreeBytes {
			return newError(KindIntegrity, "CONFIG_TREE_TOO_LARGE", "configuration tree exceeds safety limits", nil)
		}
		entries = append(entries, configTreeEntry{Path: rel, Content: content})
		files[rel] = content
		return nil
	})
	if err != nil {
		if _, ok := err.(*ECPError); ok {
			return configTreeSnapshot{}, err
		}
		return configTreeSnapshot{}, newError(KindRuntime, "CONFIG_WALK_FAILED", "could not inspect configuration tree", err)
	}
	rootAfter, err := os.Lstat(ecpDir)
	if err != nil || !os.SameFile(rootBefore, rootAfter) {
		return configTreeSnapshot{}, newError(KindConflict, "CONFIG_ROOT_CHANGED_DURING_READ", ".ecp was replaced while Core was reading it", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	configDigest, err := digestJSON(controlConfigEntries(entries))
	if err != nil {
		return configTreeSnapshot{}, newError(KindRuntime, "CONFIG_DIGEST_FAILED", "could not digest configuration tree", err)
	}
	truthEntries := projectTruthEntries(entries)
	truthDigest, err := digestJSON(truthEntries)
	if err != nil {
		return configTreeSnapshot{}, newError(KindRuntime, "TRUTH_DIGEST_FAILED", "could not digest Project Truth", err)
	}
	bundleDigest, err := digestJSON(entries)
	if err != nil {
		return configTreeSnapshot{}, newError(KindRuntime, "CONFIG_DIGEST_FAILED", "could not digest configuration tree", err)
	}
	truthFiles := make([]TruthFileDigest, 0, len(truthEntries))
	truthContent := make([]TruthFileContent, 0, len(truthEntries))
	for _, entry := range truthEntries {
		if !utf8.Valid(entry.Content) || bytes.IndexByte(entry.Content, 0) >= 0 {
			return configTreeSnapshot{}, newError(KindIntegrity, "TRUTH_FILE_NOT_TEXT", fmt.Sprintf("Project Truth file %q must be valid UTF-8 text without NUL", entry.Path), nil)
		}
		digest := digestBytes(entry.Content)
		truthFiles = append(truthFiles, TruthFileDigest{Path: entry.Path, Digest: digest})
		truthContent = append(truthContent, TruthFileContent{Path: entry.Path, Digest: digest, Content: append([]byte(nil), entry.Content...)})
	}
	return configTreeSnapshot{Files: files, ConfigDigest: configDigest, TruthDigest: truthDigest, BundleDigest: bundleDigest, TruthFiles: truthFiles, TruthContent: truthContent}, nil
}

func controlConfigEntries(entries []configTreeEntry) []configTreeEntry {
	result := make([]configTreeEntry, 0, 3)
	for _, entry := range entries {
		if entry.Path == "project.json" || entry.Path == "policy.json" || entry.Path == "gates.json" {
			result = append(result, entry)
		}
	}
	return result
}

func projectTruthEntries(entries []configTreeEntry) []configTreeEntry {
	result := make([]configTreeEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Path == "truth.json" || strings.HasPrefix(entry.Path, "contracts/") {
			result = append(result, entry)
		}
	}
	return result
}

func readStableRegularFile(path string, before os.FileInfo, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, newError(KindRuntime, "CONFIG_READ_FAILED", "could not open ECP config file", err)
	}
	defer file.Close()
	afterOpen, err := file.Stat()
	if err != nil || !afterOpen.Mode().IsRegular() || !sameFileObservation(before, afterOpen) {
		return nil, newError(KindConflict, "CONFIG_FILE_CHANGED_DURING_READ", "an ECP config file was replaced while being opened", err)
	}
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, newError(KindRuntime, "CONFIG_READ_FAILED", "could not read ECP config file", err)
	}
	if int64(len(content)) > limit {
		return nil, newError(KindIntegrity, "CONFIG_FILE_TOO_LARGE", "ECP config file exceeds the size limit", nil)
	}
	afterRead, err := file.Stat()
	if err != nil || !sameFileObservation(afterOpen, afterRead) {
		return nil, newError(KindConflict, "CONFIG_FILE_CHANGED_DURING_READ", "an ECP config file was replaced while being read", err)
	}
	return content, nil
}

func sameFileObservation(before, after os.FileInfo) bool {
	return os.SameFile(before, after) &&
		before.Size() == after.Size() &&
		before.Mode() == after.Mode() &&
		before.ModTime().Equal(after.ModTime())
}
