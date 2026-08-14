package ecp

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

func ParseChangeRequirementJSON(value string) (ChangeRequirement, error) {
	var requirement ChangeRequirement
	if err := decodeStrictJSON([]byte(value), &requirement); err != nil {
		return ChangeRequirement{}, newError(KindUsage, "INVALID_REQUIREMENT_JSON", "--requirement must be one exact ChangeRequirement JSON object", err)
	}
	return requirement, nil
}

func ParseRequirementAssessmentJSON(value string) (RequirementAssessment, error) {
	var assessment RequirementAssessment
	if err := decodeStrictJSON([]byte(value), &assessment); err != nil {
		return RequirementAssessment{}, newError(KindUsage, "INVALID_REQUIREMENT_ASSESSMENT_JSON", "--requirement-result must be one exact RequirementAssessment JSON object", err)
	}
	return assessment, nil
}

func ParseUnknownDispositionJSON(value string) (UnknownDisposition, error) {
	var disposition UnknownDisposition
	if err := decodeStrictJSON([]byte(value), &disposition); err != nil {
		return UnknownDisposition{}, newError(KindUsage, "INVALID_UNKNOWN_DISPOSITION_JSON", "--unknown-disposition must be one exact UnknownDisposition JSON object", err)
	}
	return disposition, nil
}

func validateChangeImpact(impact ChangeImpact, truth ProjectTruthConfig) error {
	capabilities := truthIDSetCapabilities(truth.Capabilities)
	invariants := truthIDSetInvariants(truth.Invariants)
	components := truthIDSetComponents(truth.Components)
	decisions := truthIDSetDecisions(truth.Decisions)
	contracts := truthIDSetContracts(truth.Contracts)
	unknownIDs := truthIDSetUnknowns(truth.Unknowns)
	if err := validateImpactRefs("capability_ids", impact.CapabilityIDs, capabilities); err != nil {
		return err
	}
	if err := validateImpactRefs("invariant_ids", impact.InvariantIDs, invariants); err != nil {
		return err
	}
	if err := validateImpactRefs("component_ids", impact.ComponentIDs, components); err != nil {
		return err
	}
	if err := validateImpactRefs("decision_ids", impact.DecisionIDs, decisions); err != nil {
		return err
	}
	if err := validateImpactRefs("contract_ids", impact.ContractIDs, contracts); err != nil {
		return err
	}
	if err := validateImpactRefs("unknown_ids", impact.UnknownIDs, unknownIDs); err != nil {
		return err
	}
	for label, values := range map[string][]string{
		"user_journeys":          impact.UserJourneys,
		"data_effects":           impact.DataEffects,
		"operational_effects":    impact.OperationalEffects,
		"expected_changes":       impact.ExpectedChanges,
		"expected_preservations": impact.ExpectedPreservations,
		"unknowns":               impact.Unknowns,
	} {
		normalized, err := normalizeTextList(values, false)
		if err != nil || !slices.Equal(normalized, values) {
			return newError(KindUsage, "INVALID_CHANGE_IMPACT", label+" must be unique, trimmed, and within length limits", err)
		}
	}
	if !impact.ProjectPurpose && len(impact.CapabilityIDs)+len(impact.InvariantIDs)+len(impact.ComponentIDs)+len(impact.DecisionIDs)+len(impact.ContractIDs)+len(impact.UnknownIDs)+
		len(impact.UserJourneys)+len(impact.DataEffects)+len(impact.OperationalEffects)+len(impact.ExpectedChanges)+len(impact.ExpectedPreservations)+len(impact.Unknowns) == 0 {
		return newError(KindUsage, "EMPTY_CHANGE_IMPACT", "Change impact must reference Project Truth or explicitly describe an affected journey, data/operational effect, or unknown", nil)
	}
	return nil
}

func validateUnknownDispositions(impact ChangeImpact, truth ProjectTruthConfig, required bool) error {
	if !required && len(impact.UnknownDispositions) == 0 {
		return nil
	}
	known := truthIDSetUnknowns(truth.Unknowns)
	impacted := truthStringSet(impact.UnknownIDs)
	seen := make(map[string]struct{}, len(impact.UnknownDispositions))
	previousID := ""
	for _, disposition := range impact.UnknownDispositions {
		if err := validateIdentifier(disposition.UnknownID, "unknown disposition id"); err != nil {
			return newError(KindUsage, "INVALID_UNKNOWN_DISPOSITION", "unknown disposition contains an invalid unknown_id", err)
		}
		if previousID != "" && disposition.UnknownID <= previousID {
			return newError(KindUsage, "INVALID_UNKNOWN_DISPOSITION", "unknown dispositions must be unique and sorted by unknown_id", nil)
		}
		previousID = disposition.UnknownID
		if _, ok := known[disposition.UnknownID]; !ok {
			return newError(KindUsage, "UNKNOWN_TRUTH_REFERENCE", fmt.Sprintf("unknown disposition references unknown Project Truth ID %q", disposition.UnknownID), nil)
		}
		if _, ok := impacted[disposition.UnknownID]; !ok {
			return newError(KindUsage, "UNKNOWN_DISPOSITION_OUTSIDE_IMPACT", fmt.Sprintf("unknown disposition %q is not declared by impact.unknown_ids", disposition.UnknownID), nil)
		}
		switch disposition.Outcome {
		case UnknownDispositionPreserved, UnknownDispositionResolved, UnknownDispositionRefined:
		default:
			return newError(KindUsage, "INVALID_UNKNOWN_DISPOSITION", fmt.Sprintf("unknown disposition %q must be PRESERVED, RESOLVED, or REFINED", disposition.UnknownID), nil)
		}
		seen[disposition.UnknownID] = struct{}{}
	}
	if required {
		for _, unknownID := range impact.UnknownIDs {
			if _, ok := seen[unknownID]; !ok {
				return newError(KindBlocked, "UNKNOWN_DISPOSITION_REQUIRED", fmt.Sprintf("accepted Project Truth unknown %q requires an explicit PRESERVED, RESOLVED, or REFINED disposition", unknownID), nil)
			}
		}
	}
	if len(seen) != len(impacted) {
		return newError(KindUsage, "UNKNOWN_DISPOSITION_MISMATCH", "unknown dispositions must map exactly to impact.unknown_ids", nil)
	}
	return nil
}

func validateUnknownDispositionOutcomes(impact ChangeImpact, starting, candidate ProjectTruthConfig) error {
	startingByID := make(map[string]TruthUnknown, len(starting.Unknowns))
	for _, unknown := range starting.Unknowns {
		startingByID[unknown.ID] = unknown
	}
	candidateByID := make(map[string]TruthUnknown, len(candidate.Unknowns))
	for _, unknown := range candidate.Unknowns {
		candidateByID[unknown.ID] = unknown
	}
	for _, disposition := range impact.UnknownDispositions {
		before, beforeOK := startingByID[disposition.UnknownID]
		after, afterOK := candidateByID[disposition.UnknownID]
		if !beforeOK {
			return newError(KindIntegrity, "CHANGE_START_UNKNOWN_MISSING", fmt.Sprintf("starting Project Truth no longer contains declared unknown %q", disposition.UnknownID), nil)
		}
		switch disposition.Outcome {
		case UnknownDispositionPreserved:
			if !afterOK || truthItemDigest(before) != truthItemDigest(after) {
				return newError(KindBlocked, "UNKNOWN_DISPOSITION_UNSATISFIED", fmt.Sprintf("Project Truth unknown %q was declared PRESERVED but is absent or changed", disposition.UnknownID), nil)
			}
		case UnknownDispositionResolved:
			if afterOK {
				return newError(KindBlocked, "UNKNOWN_DISPOSITION_UNSATISFIED", fmt.Sprintf("Project Truth unknown %q was declared RESOLVED but remains present", disposition.UnknownID), nil)
			}
		case UnknownDispositionRefined:
			if !afterOK || truthItemDigest(before) == truthItemDigest(after) {
				return newError(KindBlocked, "UNKNOWN_DISPOSITION_UNSATISFIED", fmt.Sprintf("Project Truth unknown %q was declared REFINED but is absent or unchanged", disposition.UnknownID), nil)
			}
		default:
			return newError(KindIntegrity, "INVALID_UNKNOWN_DISPOSITION", fmt.Sprintf("Change contains unsupported unknown disposition %q", disposition.Outcome), nil)
		}
	}
	return nil
}

func unknownDispositionExpectsChange(impact ChangeImpact) bool {
	for _, disposition := range impact.UnknownDispositions {
		if disposition.Outcome == UnknownDispositionResolved || disposition.Outcome == UnknownDispositionRefined {
			return true
		}
	}
	return false
}

func validateChangeRequirements(requirements []ChangeRequirement, acceptance []string, impact ChangeImpact, gates GatesConfig, required bool) error {
	if len(requirements) == 0 {
		if required {
			return newError(KindUsage, "CHANGE_REQUIREMENTS_REQUIRED", "at least one structured requirement is required and every contract item must be covered", nil)
		}
		return nil
	}
	gateIDs := make(map[string]struct{}, len(gates.Gates))
	for _, gate := range gates.Gates {
		gateIDs[gate.ID] = struct{}{}
	}
	type coverageSection struct {
		label  string
		values []string
	}
	targets := []coverageSection{
		{label: "acceptance_criteria", values: acceptance},
		{label: "user_journeys", values: impact.UserJourneys},
		{label: "data_effects", values: impact.DataEffects},
		{label: "operational_effects", values: impact.OperationalEffects},
		{label: "expected_changes", values: impact.ExpectedChanges},
		{label: "expected_preservations", values: impact.ExpectedPreservations},
		{label: "unknowns", values: impact.Unknowns},
	}
	known := make(map[string]map[string]struct{}, len(targets))
	covered := make(map[string]map[string]struct{}, len(targets))
	for _, target := range targets {
		known[target.label] = truthStringSet(target.values)
		covered[target.label] = make(map[string]struct{})
	}
	previousID := ""
	for _, requirement := range requirements {
		if err := validateIdentifier(requirement.ID, "requirement id"); err != nil {
			return newError(KindUsage, "INVALID_CHANGE_REQUIREMENT", "requirement contains an invalid ID", err)
		}
		if previousID != "" && requirement.ID <= previousID {
			return newError(KindUsage, "INVALID_CHANGE_REQUIREMENT", "requirements must be unique and sorted by id", nil)
		}
		previousID = requirement.ID
		if strings.TrimSpace(requirement.Statement) == "" || requirement.Statement != strings.TrimSpace(requirement.Statement) || len(requirement.Statement) > 4000 ||
			strings.TrimSpace(requirement.Rationale) == "" || requirement.Rationale != strings.TrimSpace(requirement.Rationale) || len(requirement.Rationale) > 4000 ||
			strings.TrimSpace(requirement.DecisionSource) == "" || requirement.DecisionSource != strings.TrimSpace(requirement.DecisionSource) || len(requirement.DecisionSource) > 1000 {
			return newError(KindUsage, "INVALID_CHANGE_REQUIREMENT", fmt.Sprintf("requirement %q has empty, non-canonical, or oversized decision fields", requirement.ID), nil)
		}
		if requirement.RevisitCondition != strings.TrimSpace(requirement.RevisitCondition) || len(requirement.RevisitCondition) > 4000 {
			return newError(KindUsage, "INVALID_CHANGE_REQUIREMENT", fmt.Sprintf("requirement %q has an invalid revisit_condition", requirement.ID), nil)
		}
		switch requirement.Status {
		case RequirementDecided, RequirementNotApplicable:
			if requirement.RevisitCondition != "" {
				return newError(KindUsage, "INVALID_CHANGE_REQUIREMENT", fmt.Sprintf("requirement %q may not declare revisit_condition for status %s", requirement.ID, requirement.Status), nil)
			}
		case RequirementDeferredSafe:
			if requirement.RevisitCondition == "" {
				return newError(KindUsage, "INVALID_CHANGE_REQUIREMENT", fmt.Sprintf("requirement %q requires a concrete revisit_condition", requirement.ID), nil)
			}
		case RequirementBlockingUnknown:
			return newError(KindBlocked, "UNRESOLVED_CHANGE_REQUIREMENT", fmt.Sprintf("requirement %q is still BLOCKING_UNKNOWN; resolve it before starting implementation", requirement.ID), nil)
		default:
			return newError(KindUsage, "INVALID_CHANGE_REQUIREMENT", fmt.Sprintf("requirement %q has an invalid status", requirement.ID), nil)
		}
		normalizedGates, err := normalizeTextList(requirement.RequiredGateIDs, false)
		if err != nil || !slices.Equal(normalizedGates, requirement.RequiredGateIDs) {
			return newError(KindUsage, "INVALID_CHANGE_REQUIREMENT", fmt.Sprintf("requirement %q required_gate_ids must be unique and canonical", requirement.ID), err)
		}
		for _, gateID := range requirement.RequiredGateIDs {
			if _, ok := gateIDs[gateID]; !ok {
				return newError(KindUsage, "UNKNOWN_REQUIREMENT_GATE", fmt.Sprintf("requirement %q references unknown Gate %q", requirement.ID, gateID), nil)
			}
		}
		switch requirement.Verification {
		case RequirementVerificationAutomated:
			if requirement.Status != RequirementDecided || len(requirement.RequiredGateIDs) == 0 {
				return newError(KindUsage, "INVALID_REQUIREMENT_VERIFICATION", fmt.Sprintf("requirement %q AUTOMATED verification requires DECIDED status and at least one Gate", requirement.ID), nil)
			}
		case RequirementVerificationReview:
			if len(requirement.RequiredGateIDs) != 0 {
				return newError(KindUsage, "INVALID_REQUIREMENT_VERIFICATION", fmt.Sprintf("requirement %q REVIEW verification cannot claim automated Gate Evidence", requirement.ID), nil)
			}
		case RequirementVerificationExternal:
			if requirement.Status != RequirementDecided || len(requirement.RequiredGateIDs) != 0 {
				return newError(KindUsage, "INVALID_REQUIREMENT_VERIFICATION", fmt.Sprintf("requirement %q EXTERNAL verification requires DECIDED status and no local Gate IDs", requirement.ID), nil)
			}
		default:
			return newError(KindUsage, "INVALID_REQUIREMENT_VERIFICATION", fmt.Sprintf("requirement %q has an invalid verification mode", requirement.ID), nil)
		}

		coverage := []coverageSection{
			{label: "acceptance_criteria", values: requirement.Covers.AcceptanceCriteria},
			{label: "user_journeys", values: requirement.Covers.UserJourneys},
			{label: "data_effects", values: requirement.Covers.DataEffects},
			{label: "operational_effects", values: requirement.Covers.OperationalEffects},
			{label: "expected_changes", values: requirement.Covers.ExpectedChanges},
			{label: "expected_preservations", values: requirement.Covers.ExpectedPreservations},
			{label: "unknowns", values: requirement.Covers.Unknowns},
		}
		coverageCount := 0
		for _, item := range coverage {
			normalized, err := normalizeTextList(item.values, false)
			if err != nil || !slices.Equal(normalized, item.values) {
				return newError(KindUsage, "INVALID_REQUIREMENT_COVERAGE", fmt.Sprintf("requirement %q %s coverage is duplicated or non-canonical", requirement.ID, item.label), err)
			}
			for _, value := range item.values {
				if _, ok := known[item.label][value]; !ok {
					return newError(KindUsage, "UNKNOWN_REQUIREMENT_COVERAGE", fmt.Sprintf("requirement %q covers a %s value that is not in the Change contract", requirement.ID, item.label), nil)
				}
				covered[item.label][value] = struct{}{}
				coverageCount++
			}
		}
		if coverageCount == 0 {
			return newError(KindUsage, "EMPTY_REQUIREMENT_COVERAGE", fmt.Sprintf("requirement %q does not cover any exact Change contract item", requirement.ID), nil)
		}
	}
	for _, target := range targets {
		for _, value := range target.values {
			if _, ok := covered[target.label][value]; !ok {
				return newError(KindBlocked, "UNCOVERED_CHANGE_CONTRACT", fmt.Sprintf("%s item %q has no structured requirement decision", target.label, value), nil)
			}
		}
	}
	return nil
}

func validateRequirementAssessments(change Change, assessments []RequirementAssessment) error {
	if len(change.Requirements) == 0 {
		if len(assessments) != 0 {
			return newError(KindIntegrity, "UNEXPECTED_REQUIREMENT_ASSESSMENT", "legacy Change has no structured requirements", nil)
		}
		return nil
	}
	if len(assessments) != len(change.Requirements) {
		return newError(KindUsage, "REQUIREMENT_ASSESSMENTS_INCOMPLETE", "semantic reconciliation must assess every structured requirement exactly once", nil)
	}
	for index, requirement := range change.Requirements {
		assessment := assessments[index]
		if assessment.RequirementID != requirement.ID {
			return newError(KindUsage, "REQUIREMENT_ASSESSMENTS_INCOMPLETE", "requirement assessments must be unique, sorted, and match the Change requirements", nil)
		}
		if strings.TrimSpace(assessment.Summary) == "" || assessment.Summary != strings.TrimSpace(assessment.Summary) || len(assessment.Summary) > 4000 {
			return newError(KindUsage, "INVALID_REQUIREMENT_ASSESSMENT", fmt.Sprintf("requirement %q assessment summary is empty or invalid", requirement.ID), nil)
		}
		normalizedGates, err := normalizeTextList(assessment.EvidenceGateIDs, false)
		if err != nil || !slices.Equal(normalizedGates, assessment.EvidenceGateIDs) {
			return newError(KindUsage, "INVALID_REQUIREMENT_ASSESSMENT", fmt.Sprintf("requirement %q Evidence Gate IDs are duplicated or non-canonical", requirement.ID), err)
		}
		switch {
		case requirement.Status == RequirementNotApplicable:
			if assessment.Outcome != RequirementOutcomeNotApplicable || len(assessment.EvidenceGateIDs) != 0 {
				return newError(KindUsage, "REQUIREMENT_OUTCOME_MISMATCH", fmt.Sprintf("requirement %q must reconcile as NOT_APPLICABLE", requirement.ID), nil)
			}
		case requirement.Status == RequirementDeferredSafe:
			if assessment.Outcome != RequirementOutcomeDeferredSafe || len(assessment.EvidenceGateIDs) != 0 {
				return newError(KindUsage, "REQUIREMENT_OUTCOME_MISMATCH", fmt.Sprintf("requirement %q must reconcile as DEFERRED_SAFE", requirement.ID), nil)
			}
		case requirement.Verification == RequirementVerificationExternal:
			if assessment.Outcome != RequirementOutcomeExternalPending || len(assessment.EvidenceGateIDs) != 0 {
				return newError(KindUsage, "REQUIREMENT_OUTCOME_MISMATCH", fmt.Sprintf("requirement %q must remain EXTERNAL_PENDING until an external Evidence class exists", requirement.ID), nil)
			}
		case requirement.Verification == RequirementVerificationAutomated:
			if assessment.Outcome != RequirementOutcomeVerified || !slices.Equal(assessment.EvidenceGateIDs, requirement.RequiredGateIDs) {
				return newError(KindUsage, "REQUIREMENT_OUTCOME_MISMATCH", fmt.Sprintf("requirement %q must map VERIFIED to its exact required Gate IDs", requirement.ID), nil)
			}
		default:
			if assessment.Outcome != RequirementOutcomeVerified || len(assessment.EvidenceGateIDs) != 0 {
				return newError(KindUsage, "REQUIREMENT_OUTCOME_MISMATCH", fmt.Sprintf("requirement %q must reconcile as a reviewed VERIFIED decision", requirement.ID), nil)
			}
		}
	}
	return nil
}

func validateImpactRefs(label string, values []string, known map[string]struct{}) error {
	normalized, err := normalizeTextList(values, false)
	if err != nil || !slices.Equal(normalized, values) {
		return newError(KindUsage, "INVALID_CHANGE_IMPACT", label+" must be unique and canonical", err)
	}
	for _, id := range values {
		if _, ok := known[id]; !ok {
			return newError(KindUsage, "UNKNOWN_IMPACT_REFERENCE", fmt.Sprintf("%s references unknown Project Truth id %q", label, id), nil)
		}
	}
	return nil
}

func validateTruthAcceptance(acceptance ProjectTruthAcceptance, config *ConfigAcceptance) error {
	if config == nil {
		return newError(KindIntegrity, "TRUTH_ACCEPTANCE_WITHOUT_CONFIG", "Project Truth acceptance requires an accepted control config", nil)
	}
	if acceptance.SchemaVersion != SchemaVersion || !isSHA256Digest(acceptance.TruthDigest) || acceptance.AcceptedAt.IsZero() ||
		strings.TrimSpace(acceptance.Actor) == "" || acceptance.Actor != strings.TrimSpace(acceptance.Actor) ||
		strings.TrimSpace(acceptance.Reason) == "" || acceptance.Reason != strings.TrimSpace(acceptance.Reason) ||
		acceptance.Trust == "" || acceptance.CoreIdentity == "" {
		return newError(KindIntegrity, "TRUTH_ACCEPTANCE_INVALID", "Project Truth acceptance is incomplete or invalid", nil)
	}
	if acceptance.PreviousTruthDigest != "" && !isSHA256Digest(acceptance.PreviousTruthDigest) {
		return newError(KindIntegrity, "TRUTH_ACCEPTANCE_INVALID", "previous Project Truth digest is invalid", nil)
	}
	if acceptance.ChangeID != "" {
		if err := validateIdentifier(acceptance.ChangeID, "change_id"); err != nil {
			return newError(KindIntegrity, "TRUTH_ACCEPTANCE_INVALID", "Project Truth acceptance contains an invalid Change ID", err)
		}
	}
	if err := validateProjectTruth(acceptance.Truth, config.Gates); err != nil {
		return newError(KindIntegrity, "EFFECTIVE_TRUTH_INVALID", "accepted Project Truth is invalid", err)
	}
	if len(acceptance.Files) == 0 {
		return newError(KindIntegrity, "TRUTH_FILE_MANIFEST_EMPTY", "accepted Project Truth has no file manifest", nil)
	}
	seen := make(map[string]struct{}, len(acceptance.Files))
	foundTruth := false
	previousPath := ""
	for _, file := range acceptance.Files {
		normalized, err := normalizePathRoot(file.Path)
		if err != nil || normalized != file.Path || (file.Path != "truth.json" && !strings.HasPrefix(file.Path, "contracts/")) || !isSHA256Digest(file.Digest) {
			return newError(KindIntegrity, "TRUTH_FILE_MANIFEST_INVALID", fmt.Sprintf("accepted Project Truth file %q is invalid", file.Path), err)
		}
		if _, duplicate := seen[file.Path]; duplicate || (previousPath != "" && file.Path < previousPath) {
			return newError(KindIntegrity, "TRUTH_FILE_MANIFEST_INVALID", "accepted Project Truth file manifest is duplicated or unsorted", nil)
		}
		seen[file.Path] = struct{}{}
		previousPath = file.Path
		if file.Path == "truth.json" {
			foundTruth = true
		}
	}
	if !foundTruth {
		return newError(KindIntegrity, "TRUTH_FILE_MANIFEST_INVALID", "accepted Project Truth file manifest is missing truth.json", nil)
	}
	return validateTruthFileReferences(acceptance.Truth, acceptance.Files)
}

func validateTruthFileReferences(truth ProjectTruthConfig, files []TruthFileDigest) error {
	referenced := map[string]string{"truth.json": "truth"}
	for _, contract := range truth.Contracts {
		referenced[contract.Path] = contract.ID
	}
	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		if _, ok := referenced[file.Path]; !ok {
			return newError(KindIntegrity, "UNREFERENCED_TRUTH_FILE", fmt.Sprintf("Project Truth file %q is not referenced by truth.json", file.Path), nil)
		}
		seen[file.Path] = struct{}{}
	}
	for path, id := range referenced {
		if _, ok := seen[path]; !ok {
			return newError(KindIntegrity, "TRUTH_CONTRACT_FILE_MISSING", fmt.Sprintf("Project Truth reference %q has no file %q", id, path), nil)
		}
	}
	return nil
}

func validateSemanticAssessment(assessment SemanticAssessment, acceptedTruthDigest string) error {
	if assessment.SchemaVersion != SchemaVersion || assessment.RecordedAt.IsZero() || assessment.Trust == "" ||
		!isSHA256Digest(assessment.PreviousTruthDigest) || !isSHA256Digest(assessment.CurrentTruthDigest) ||
		assessment.PreviousTruthDigest != acceptedTruthDigest || !isSHA256Digest(assessment.SourceFingerprint) ||
		strings.TrimSpace(assessment.Summary) == "" || assessment.Summary != strings.TrimSpace(assessment.Summary) || len(assessment.Summary) > 4000 ||
		strings.TrimSpace(assessment.Actor) == "" || assessment.Actor != strings.TrimSpace(assessment.Actor) || len(assessment.Actor) > 200 ||
		strings.TrimSpace(assessment.Reason) == "" || assessment.Reason != strings.TrimSpace(assessment.Reason) || len(assessment.Reason) > 4000 {
		return newError(KindIntegrity, "SEMANTIC_ASSESSMENT_INVALID", "semantic assessment is incomplete or not bound to the accepted Project Truth", nil)
	}
	if err := validateIdentifier(assessment.AssessmentID, "assessment_id"); err != nil {
		return newError(KindIntegrity, "SEMANTIC_ASSESSMENT_INVALID", "semantic assessment ID is invalid", err)
	}
	normalizedCategories, err := normalizeTextList(assessment.Categories, true)
	if err != nil || !slices.Equal(normalizedCategories, assessment.Categories) {
		return newError(KindIntegrity, "SEMANTIC_CATEGORIES_INVALID", "semantic categories must be non-empty, unique, and canonical", err)
	}
	validCategories := map[string]struct{}{
		"behavior": {}, "business": {}, "architecture": {}, "data": {}, "interface": {}, "permission": {},
		"security": {}, "privacy": {}, "compatibility": {}, "operations": {}, "performance": {},
		"project-truth": {}, "unknown": {},
	}
	for _, category := range assessment.Categories {
		if _, ok := validCategories[category]; !ok {
			return newError(KindIntegrity, "SEMANTIC_CATEGORIES_INVALID", fmt.Sprintf("semantic category %q is not supported", category), nil)
		}
	}
	changed := assessment.CurrentTruthDigest != assessment.PreviousTruthDigest
	if assessment.Behavior != SemanticBehaviorPreserved && assessment.Behavior != SemanticBehaviorChanged && assessment.Behavior != SemanticBehaviorUnknown {
		return newError(KindIntegrity, "SEMANTIC_BEHAVIOR_INVALID", "semantic behavior must be PRESERVED, CHANGED, or UNKNOWN", nil)
	}
	if assessment.Behavior == SemanticBehaviorPreserved && changed {
		return newError(KindIntegrity, "SEMANTIC_BEHAVIOR_INVALID", "PRESERVED cannot accept changed Project Truth", nil)
	}
	if changed && !slices.Contains(assessment.Categories, "project-truth") {
		return newError(KindIntegrity, "SEMANTIC_CATEGORIES_INVALID", "a changed Project Truth assessment must include the project-truth category", nil)
	}
	if assessment.Behavior == SemanticBehaviorUnknown && !slices.Contains(assessment.Categories, "unknown") {
		return newError(KindIntegrity, "SEMANTIC_CATEGORIES_INVALID", "an UNKNOWN assessment must include the unknown category", nil)
	}
	if changed != (len(assessment.TruthDelta) > 0) {
		return newError(KindIntegrity, "SEMANTIC_DELTA_INVALID", "truth delta does not match the declared truth digest transition", nil)
	}
	protected := false
	previousKey := ""
	for _, delta := range assessment.TruthDelta {
		if strings.TrimSpace(delta.Section) == "" || strings.TrimSpace(delta.ID) == "" || (delta.Operation != "added" && delta.Operation != "modified" && delta.Operation != "removed") {
			return newError(KindIntegrity, "SEMANTIC_DELTA_INVALID", "truth delta contains an invalid item", nil)
		}
		key := delta.Section + "\x00" + delta.ID
		if previousKey != "" && key <= previousKey {
			return newError(KindIntegrity, "SEMANTIC_DELTA_INVALID", "truth delta must be unique and sorted", nil)
		}
		previousKey = key
		protected = protected || delta.Protected
	}
	if assessment.ProtectedChange != protected {
		return newError(KindIntegrity, "SEMANTIC_PROTECTION_INVALID", "protected_change does not match the truth delta", nil)
	}
	if assessment.ProtectedConfirmed != (protected && assessment.Behavior == SemanticBehaviorChanged) {
		return newError(KindIntegrity, "SEMANTIC_CONFIRMATION_INVALID", "protected Project Truth changes require an explicit confirmation bound to a CHANGED assessment", nil)
	}
	return nil
}

func latestSemanticAssessment(projection Projection, changeID string) *SemanticAssessment {
	items := projection.SemanticAssessments[changeID]
	if len(items) == 0 {
		return nil
	}
	copyOfAssessment := items[len(items)-1]
	return &copyOfAssessment
}

func computeTruthDelta(previous ProjectTruthAcceptance, candidate ConfigBundle) ([]TruthDelta, error) {
	deltas := make([]TruthDelta, 0)
	if previous.Truth.Maturity != candidate.Truth.Maturity || previous.Truth.Purpose != candidate.Truth.Purpose {
		deltas = append(deltas, TruthDelta{Section: "project", ID: "purpose", Operation: "modified", Protected: true})
	}
	sections := []struct {
		name      string
		previous  map[string]string
		candidate map[string]string
	}{
		{"capability", truthDigestsCapabilities(previous.Truth.Capabilities), truthDigestsCapabilities(candidate.Truth.Capabilities)},
		{"invariant", truthDigestsInvariants(previous.Truth.Invariants), truthDigestsInvariants(candidate.Truth.Invariants)},
		{"component", truthDigestsComponents(previous.Truth.Components), truthDigestsComponents(candidate.Truth.Components)},
		{"decision", truthDigestsDecisions(previous.Truth.Decisions), truthDigestsDecisions(candidate.Truth.Decisions)},
		{"contract", truthDigestsContracts(previous.Truth.Contracts), truthDigestsContracts(candidate.Truth.Contracts)},
		{"unknown", truthDigestsUnknowns(previous.Truth.Unknowns), truthDigestsUnknowns(candidate.Truth.Unknowns)},
		{"file", truthFileDigestMap(previous.Files), truthFileDigestMap(candidate.TruthFiles)},
	}
	for _, section := range sections {
		for id, oldDigest := range section.previous {
			newDigest, ok := section.candidate[id]
			if !ok {
				deltas = append(deltas, TruthDelta{Section: section.name, ID: id, Operation: "removed", Protected: true})
			} else if newDigest != oldDigest {
				deltas = append(deltas, TruthDelta{Section: section.name, ID: id, Operation: "modified", Protected: true})
			}
		}
		for id := range section.candidate {
			if _, ok := section.previous[id]; !ok {
				deltas = append(deltas, TruthDelta{Section: section.name, ID: id, Operation: "added", Protected: true})
			}
		}
	}
	sort.Slice(deltas, func(i, j int) bool {
		if deltas[i].Section != deltas[j].Section {
			return deltas[i].Section < deltas[j].Section
		}
		return deltas[i].ID < deltas[j].ID
	})
	return deltas, nil
}

func validateTruthDeltaAgainstImpact(delta []TruthDelta, impact ChangeImpact, previous, candidate ProjectTruthConfig) error {
	if len(delta) == 0 {
		return nil
	}
	allowsNewDiscovery := len(impact.Unknowns) > 0
	allowed := map[string]map[string]struct{}{
		"capability": truthStringSet(impact.CapabilityIDs),
		"invariant":  truthStringSet(impact.InvariantIDs),
		"component":  truthStringSet(impact.ComponentIDs),
		"decision":   truthStringSet(impact.DecisionIDs),
		"contract":   truthStringSet(impact.ContractIDs),
		"unknown":    truthStringSet(impact.UnknownIDs),
	}
	contractByPath := make(map[string]string)
	for _, contract := range previous.Contracts {
		contractByPath[contract.Path] = contract.ID
	}
	for _, contract := range candidate.Contracts {
		contractByPath[contract.Path] = contract.ID
	}
	for _, item := range delta {
		if item.Section == "file" && item.ID == "truth.json" {
			continue
		}
		if item.Section == "project" && impact.ProjectPurpose {
			continue
		}
		section := item.Section
		id := item.ID
		if section == "file" {
			section = "contract"
			id = contractByPath[item.ID]
		}
		if ids := allowed[section]; ids != nil {
			if _, ok := ids[id]; ok {
				continue
			}
		}
		// A concrete startup uncertainty can cover a newly discovered fact that
		// had no accepted ID at Change start. It is never a wildcard for
		// modifying or removing an existing protected fact.
		if allowsNewDiscovery && item.Operation == "added" {
			continue
		}
		return newError(KindBlocked, "TRUTH_DELTA_OUTSIDE_CHANGE_IMPACT", fmt.Sprintf("Project Truth delta %s/%s was not declared in the Change impact; update the Change contract in a new Change or explicitly declare the unresolved impact before implementation", item.Section, item.ID), nil)
	}
	return nil
}

func truthStringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func truthFileDigestMap(files []TruthFileDigest) map[string]string {
	result := make(map[string]string, len(files))
	for _, file := range files {
		result[file.Path] = file.Digest
	}
	return result
}

func truthItemDigest(value any) string {
	digest, _ := digestJSON(value)
	return digest
}

func truthDigestsCapabilities(values []TruthCapability) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		result[value.ID] = truthItemDigest(value)
	}
	return result
}

func truthDigestsInvariants(values []TruthInvariant) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		result[value.ID] = truthItemDigest(value)
	}
	return result
}

func truthDigestsComponents(values []TruthComponent) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		result[value.ID] = truthItemDigest(value)
	}
	return result
}

func truthDigestsDecisions(values []TruthDecision) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		result[value.ID] = truthItemDigest(value)
	}
	return result
}

func truthDigestsContracts(values []TruthContractRef) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		result[value.ID] = truthItemDigest(value)
	}
	return result
}

func truthDigestsUnknowns(values []TruthUnknown) map[string]string {
	result := make(map[string]string, len(values))
	for _, value := range values {
		result[value.ID] = truthItemDigest(value)
	}
	return result
}

func truthIDSetCapabilities(values []TruthCapability) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value.ID] = struct{}{}
	}
	return result
}

func truthIDSetInvariants(values []TruthInvariant) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value.ID] = struct{}{}
	}
	return result
}

func truthIDSetComponents(values []TruthComponent) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value.ID] = struct{}{}
	}
	return result
}

func truthIDSetDecisions(values []TruthDecision) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value.ID] = struct{}{}
	}
	return result
}

func truthIDSetContracts(values []TruthContractRef) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value.ID] = struct{}{}
	}
	return result
}

func truthIDSetUnknowns(values []TruthUnknown) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value.ID] = struct{}{}
	}
	return result
}
