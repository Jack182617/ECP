package ecp

// ContractSchema returns the closed enums and current Change contract version
// accepted by this Core build. Keeping this in Core gives adapters one
// authoritative preflight instead of duplicating values in prompts.
func ContractSchema() ContractSchemaView {
	return ContractSchemaView{
		SchemaVersion:         SchemaVersion,
		CoreVersion:           CoreVersion,
		ChangeContractVersion: CurrentChangeContractVersion,
		Risks:                 []Risk{RiskLow, RiskModerate, RiskHigh, RiskCritical},
		GateTiers:             []GateTier{GateTierFast, GateTierAffected, GateTierFull},
		RequirementStatuses: []ChangeRequirementStatus{
			RequirementDecided,
			RequirementNotApplicable,
			RequirementDeferredSafe,
			RequirementBlockingUnknown,
		},
		RequirementVerifications: []RequirementVerification{
			RequirementVerificationAutomated,
			RequirementVerificationReview,
			RequirementVerificationExternal,
		},
		RequirementOutcomes: []RequirementOutcome{
			RequirementOutcomeVerified,
			RequirementOutcomeNotApplicable,
			RequirementOutcomeDeferredSafe,
			RequirementOutcomeExternalPending,
		},
		SemanticBehaviors: []SemanticBehavior{
			SemanticBehaviorPreserved,
			SemanticBehaviorChanged,
			SemanticBehaviorUnknown,
		},
		SemanticCategories: []string{
			"architecture",
			"behavior",
			"business",
			"compatibility",
			"data",
			"interface",
			"operations",
			"performance",
			"permission",
			"privacy",
			"project-truth",
			"security",
			"unknown",
		},
		UnknownDispositionOutcomes: []UnknownDispositionOutcome{
			UnknownDispositionPreserved,
			UnknownDispositionResolved,
			UnknownDispositionRefined,
		},
	}
}
