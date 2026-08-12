# ECP v0.3 Verification and Product-Readiness Matrix

This matrix maps the canonical acceptance contract to concrete evidence. It is
not a release certificate and it does not replace rerunning the named checks.
Its purpose is to prevent a green local suite from being generalized into
fresh-desktop, real-project, CI, device, release, production, or multi-year
proof.

## Evidence classes

| Mark | Meaning |
| --- | --- |
| `A` | Automated behavior test runs on the supported local Unix development host. |
| `S` | Static contract validation of the Skill/Plugin or canonical documents; this proves packaged instructions, not Agent infallibility or non-bypassability. |
| `B` | Build/policy evidence for a platform that is not runtime-tested here. |
| `X` | External evidence is required and is not supplied by this repository. |

Test names below are Go test functions under `internal/ecp` or `internal/cli`.
The authoritative scenario wording remains in `SPEC.md` section 12.

## SPEC section 12 traceability

| # | Contract focus | Evidence | Class |
| ---: | --- | --- | :---: |
| 1 | Disabled bootstrap, explicit enable, Change → Gate → PASS → completion/history | `TestProjectStatusDefaultsDisabledWithoutCreatingControlPlaneState`; `TestWorkflowPassStaleRerunAndComplete`; `TestCompletedAndCancelledHistoryRemainQueryable` | A |
| 2 | Source change makes old Evidence stale | `TestWorkflowPassStaleRerunAndComplete` | A |
| 3 | Exact Gate rerun can restore PASS | `TestWorkflowPassStaleRerunAndComplete` | A |
| 4 | Gate-time source mutation cannot PASS | `TestGateMutationCannotPass` | A |
| 5 | Exit, timeout, and spawn failures cannot PASS | `TestGateFailureModesFailClosed`; `TestRunnerTimeoutKillsDescendantsAndCapsOutput` | A |
| 6 | Config drift requires acceptance and cannot create vacuous PASS | `TestConfigDriftCannotCreateVacuousPass` | A |
| 7 | Out-of-scope add/change/delete/rename/mode changes block | `TestOutOfScopePathBlocksVerdict`; `TestOutOfScopeRenameAndModeRemainVisible` | A |
| 8 | Mid-Change commit remains visible against baseline | `TestSnapshotDetectsCommitAndModeChanges` | A |
| 9 | High-risk acknowledgement binds the exact subject | `TestHighRiskAcknowledgementBindsExactSubject` | A |
| 10 | Artifact corruption is indeterminate; unsafe binding/events/state fail integrity | `TestEvidenceArtifactCorruptionIsIndeterminate`; `TestAuthorityStateAndArtifactsMustRemainPrivate`; `TestGateFailureAndIntegrityUseStableJSONExitCodes` | A |
| 11 | One ACTIVE Change per Workspace | `TestSingleActiveChangeAndScopePrefixBoundary` | A |
| 12 | Shell metacharacters remain literal argv | `TestGateArgvMetacharactersRemainLiteral` | A |
| 13 | Traversal, outside cwd, and followed source symlinks are rejected | `TestPathAndSymlinkBoundaries` | A |
| 14 | Skill never sets Verdict, only forwards exact confirmed preconditions, and does not equate local PASS with release | `TestBundledSkillEncodesAdapterSafetyBoundaries`; `plugins/ecp-codex/skills/ecp-change/SKILL.md` | A+S |
| 15 | Existing Project Pack is not implicitly accepted; Draft Project ID cannot replace binding | `TestExistingConfigRequiresExplicitRegistration`; `TestDraftProjectIDCannotReplaceAuthorityIdentity` | A |
| 16 | Config semantics and digest come from one stable byte epoch | `TestConfigReadRejectsMixedEpoch` | A |
| 17 | Dirty submodule checkout changes stale Evidence | `TestSubmoduleCheckoutChangesSourceFingerprint` | A |
| 18 | `core.worktree`, trailing spaces, and in-repo state path cannot redirect authority | `TestLocalCoreWorktreeCannotRedirectInitialization`; `TestGitPathWithTrailingSpacePreservesWorkspaceIdentity`; `TestStateDirectoryInsideRepositoryIsRejectedBeforeConfigWrite`; `TestStateDirectoryAlternateFilesystemSpellingIsRejected` | A |
| 19 | Reachable risk without a Required Gate cannot create ACTIVE | `TestStartChangeRefusesEmptyOrUnsupportedGatePlan`; `TestStoreRejectsEnablementAndChangeWithoutRequiredGateCoverage` | A |
| 20 | Default environment omits HOME; sensitive/NUL/invalid execution context fails before ACTIVE | `TestDefaultGateEnvironmentOmitsHomeAndRejectsCommonSecretCapabilities`; `TestGatePreflightCancellationAndAuthorityHistoryRecovery` | A |
| 21 | Authority-only history and exact cancellation survive malformed/missing Draft | `TestGatePreflightCancellationAndAuthorityHistoryRecovery`; `TestCompletedAndCancelledHistoryRemainQueryable` | A |
| 22 | Cancellation preserves Evidence/history and stale Change ID cannot cancel a later Change | `TestDisableAtomicallyCancelsObservedChangeAndPreservesEvidenceAndSource`; `TestActiveChangeMutationsRequireExactIdentity` | A |
| 23 | Invalid indexed submodules fail closed and recursive source budgets are global | `TestSubmoduleCheckoutChangesSourceFingerprint` | A |
| 24 | Advisory lease releases on holder exit and conflicts while held | `TestEventTamperRevisionCASAndGateLease`; `TestAdvisoryLockIsReleasedWhenHolderProcessExits` | A |
| 25 | Partial Gate sequence error exposes committed Evidence; ordinary errors omit zero partial result | `TestGateSequenceErrorReturnsPartialEvidence`; `TestGateFailureAndIntegrityUseStableJSONExitCodes` | A |
| 26 | Stale authority/Workspace/config/Change/subject targets cannot retarget mutations | `TestProjectInspectBindsExactRegistrationDigest`; `TestActiveChangeMutationsRequireExactIdentity`; `TestWorkflowPassStaleRerunAndComplete` | A |
| 27 | Change start requires exact observed authority/Workspace/activation/config/truth/source | `TestStartChangeRequiresExactObservedAuthorityWorkspaceConfigAndSource` | A |
| 28 | Stale plan stops before first command or before the next command with partial Evidence | `TestStalePlanDoesNotCreateGateRun`; `TestConfigDriftStopsRemainingGateSequence`; `TestGateSequenceErrorReturnsPartialEvidence` | A |
| 29 | Evidence binds exact activation/plan and cannot cross epoch or rebuilt plan | `TestEvidenceStalesOnExecutableEnvironmentAndCoreIdentity`; `TestProjectActivationIsWorkspaceScopedPersistentAndCloneLocal` | A |
| 30 | Ambient Git is ignored; missing trusted Git and Git byte drift fail closed/stale | `TestCoreGitIgnoresAmbientPath`; `TestEvidenceStalesOnExecutableEnvironmentAndCoreIdentity` | A |
| 31 | Non-Unix authority policy rejects before writes; version path remains authority-independent | `TestAuthorityPlatformPolicyFailsClosedForUnsupportedHost`; `TestVersionUsesStableEnvelope`; Windows cross-build command in the validation protocol below | A+B |
| 32 | Never-registered status is side-effect-free disabled; config/Plugin/other clone cannot enable | `TestProjectStatusDefaultsDisabledWithoutCreatingControlPlaneState`; `TestProjectActivationIsWorkspaceScopedPersistentAndCloneLocal`; `TestStatusAndEnableNeverExecuteConfiguredGate` | A |
| 33 | Enable consumes exact status and only preflights a covered Gate | `TestStatusAndEnableNeverExecuteConfiguredGate`; `TestStoreRejectsEnablementAndChangeWithoutRequiredGateCoverage`; `TestProjectModeCommandsDefaultDisabledAndFailClosedOnDrift` | A |
| 34 | Enabled drift remains enabled+BLOCKED/INDETERMINATE and can disable authority-only | `TestEnabledProjectNeverFallsBackOnConfigDriftAndCanStillDisable`; `TestProjectModeCommandsDefaultDisabledAndFailClosedOnDrift` | A |
| 35 | Disable token is stale after mutation; active cancellation+disable is atomic; repeated disable is idempotent | `TestDisableAtomicallyCancelsObservedChangeAndPreservesEvidenceAndSource`; `TestEnabledProjectNeverFallsBackOnConfigDriftAndCanStillDisable` | A |
| 36 | Re-enable creates a new activation and old context/Evidence/ack/subject cannot cross it | `TestProjectActivationIsWorkspaceScopedPersistentAndCloneLocal`; `TestActiveChangeMutationsRequireExactIdentity` | A |
| 37 | Supported Skill probes status before mutation, routes disabled normally, governs enabled, and has no task bypass | `TestBundledSkillEncodesAdapterSafetyBoundaries`; bundled `SKILL.md` | A+S |
| 38 | Normal user flow hides CLI commands and opaque protocol values | `TestBundledSkillEncodesAdapterSafetyBoundaries`; bundled `SKILL.md` | A+S |
| 39 | Enabled BLOCKED/INDETERMINATE status is `ok:true` stdout with exit 3/4 | `TestProjectModeCommandsDefaultDisabledAndFailClosedOnDrift` | A |
| 40 | Skill bypass limitations are explicit and never described as enforcement | `TestBundledSkillEncodesAdapterSafetyBoundaries`; `docs/security-model.md`; bundled `SKILL.md` | A+S |
| 41 | Init creates honest seed truth/contract and independent control/truth digests | `TestRepositoryProjectPackLoadsAsEstablishedTruth`; `TestCLIChangeStartAndTruthReconcileMachineContract` | A |
| 42 | Change binds accepted truth and rejects empty/unknown/stale Impact references | `TestCLIChangeStartAndTruthReconcileMachineContract`; `TestStartChangeRequiresExactObservedAuthorityWorkspaceConfigAndSource` | A |
| 43 | PASS Gates without final semantic assessment remain blocked; UNKNOWN never passes | `TestSemanticReconciliationIsRequiredForPass`; `TestUnknownSemanticOutcomeCannotPass` | A |
| 44 | Core computes truth delta; protected confirmation and atomic acceptance are exact; zero-delta CHANGED creates no truth epoch | `TestProtectedProjectTruthEvolutionRequiresExactConfirmation`; `TestSemanticChangeWithoutProjectTruthDeltaCanPassButCannotBeMarkedPreserved` | A |
| 45 | Assessment, plan, Evidence, Verdict, and completion bind final source/truth | `TestSemanticReconciliationIsRequiredForPass`; `TestWorkflowPassStaleRerunAndComplete`; `TestProtectedProjectTruthEvolutionRequiresExactConfirmation` | A |
| 46 | Impact preserves product expectations and semantic categories are versioned | `TestSemanticCategoryContractRejectsUnversionedValues`; `TestSemanticChangeWithoutProjectTruthDeltaCanPassButCannotBeMarkedPreserved` | A |
| 47 | Every contract file is referenced; generic unknown cannot change existing protected truth | `TestUnreferencedProjectTruthContractFileIsRejected`; `TestImpactUnknownCannotModifyExistingProtectedFact` | A |
| 48 | Latest and exact historical accepted Truth/contracts survive candidate drift and detect bad blobs | `TestProtectedProjectTruthEvolutionRequiresExactConfirmation`; `TestAcceptedProjectTruthContentSurvivesCandidateDriftAndDetectsCorruption` | A |
| 49 | Latest and exact historical accepted Project/Policy/Gates survive candidate drift | `TestAcceptedControlConfigSurvivesMalformedCandidate`; `TestAcceptedControlConfigRecoversAnExactHistoricalEpoch` | A |
| 50 | Product CHANGED and truth delta are independent; PRESERVED cannot satisfy expected change | `TestSemanticChangeWithoutProjectTruthDeltaCanPassButCannotBeMarkedPreserved` | A |
| 51 | Impacted invariant raises risk and injects its exact Gate | `TestProtectedProjectTruthEvolutionRequiresExactConfirmation` | A |
| 52 | Starting/final truth risk and Gate requirements form a conservative union | `TestTruthEvolutionCannotLowerItsOwnRiskOrDropItsStartingGate` | A |
| 53 | Launcher is path-independent and never resolves ambient/repository Core | `TestBundledPluginRuntimeIsCompleteAndPathIndependent` | A |
| 54 | Four runtime targets are version/size/digest/sidecar bound and corruption is rejected pre-Core | `TestBundledPluginRuntimeIsCompleteAndPathIndependent` | A |
| 55 | Legacy root loads; threshold rotates; new Store replays exact projection | `TestLegacySingleEventFileRemainsCompatible`; `TestSegmentedEventHistoryRotatesAndReloadsAcrossStoreInstances` | A |
| 56 | Segment tamper/gap/name/empty/limit/type/permission fails; safe temp is ignored | `TestSegmentedEventHistoryDetectsCrossSegmentTampering`; `TestSegmentedEventHistoryRejectsGapsAndNonCanonicalLayout`; `TestSegmentedEventHistoryHandlesOnlySafeAtomicWriteRemnants` | A |
| 57 | Oversized event batch fails before partial mutation | `TestEventBatchMustFitOneSegmentWithoutPartialMutation` | A |
| 58 | Aggregate bytes/segment count/terminal reserve remain bounded | `TestEventStoreReserveRemainsUntilProjectDisablement`; `TestAuthorityHealthReportsSegmentedCapacityAndThresholds` | A |
| 59 | Unchanged authority exports deterministically, read-only, private, and offline-verifiable | `TestAuthorityExportIsDeterministicPrivateAndOfflineVerifiable` | A |
| 60 | Export includes all segments/history references and excludes temp/orphans despite malformed candidate | `TestAuthorityExportPreservesSegmentedHistory`; `TestAuthorityExportIgnoresOrphansAndSurvivesMalformedCandidate` | A |
| 61 | Missing/extra/unsafe/tampered bundle and recomputed outer manifest attacks fail | `TestAuthorityExportVerificationDetectsFileTampering`; `TestAuthorityExportEventChainSurvivesRecomputedManifestAttack`; `TestAuthorityExportVerificationRejectsMissingAndExtraFiles` | A |
| 62 | Unsafe/existing/internal export targets fail; verify never implies restore or publication | `TestAuthorityExportRejectsUnsafeTargetsAndBundleRoots`; static export contract | A+S |
| 63 | GateRun starts before code, has exact Evidence, valid terminal states, and rejects impossible replay | `TestGateRunLifecycleCompletesAndBindsEvidence`; `TestGateRunPartialFailureIsTerminalAndPreservesEvidence`; `TestCancelledGateRunIsTerminalAndLiveLeaseIsNotRecovered`; `TestGateRunTerminalReplayRejectsImpossibleStates` | A |
| 64 | Live holder cannot be interrupted; killed holder remains open until the next acquired lease records INTERRUPTED | `TestReleasedGateRunIsRecoveredAsInterruptedBeforeNextRun`; `TestKilledGateRunHolderIsRecoveredAfterAdvisoryLeaseRelease` | A |
| 65 | Open run makes status/context/Verdict indeterminate and history remains authority-only | `TestReleasedGateRunIsRecoveredAsInterruptedBeforeNextRun`; `TestGateRunHistoryRemainsAuthorityOnlyWhenCandidateIsMalformed` | A |
| 66 | Health is candidate-independent, lock-consistent, and cannot misclassify a live lease | `TestAuthorityHealthIsAuthorityOnlyExactAndHealthy`; `TestAuthorityHealthCannotMisclassifyLiveGateLeaseOwner` | A |
| 67 | Health verifies all historical references and reports exact capacity without event mutation | `TestAuthorityHealthIsAuthorityOnlyExactAndHealthy`; `TestAuthorityHealthReportsSegmentedCapacityAndThresholds` | A |
| 68 | Bad references return structured INDETERMINATE/4; bad projection returns typed integrity error | `TestAuthorityHealthReturnsIndeterminateReportForCorruptReferences`; `TestAuthorityHealthRejectsUntrustedEventProjectionAndReportsUnsafeOrphan`; `TestAuthorityHealthUsesStructuredStatusExitCodes` | A |
| 69 | Safe remnants/orphans are ATTENTION and preserved; unsafe layout is INDETERMINATE, not repaired or followed | `TestAuthorityHealthReportsOrphansAndTemporaryRemnantsWithoutDeletingThem`; `TestAuthorityHealthDoesNotRepairUnsafeAuthorityDirectory`; `TestAuthorityHealthDoesNotFollowObjectStoreSymlinks` | A |
| 70 | Health reports but does not recover an open released run and applies stable capacity thresholds | `TestAuthorityHealthExposesButDoesNotRecoverReleasedGateRun`; `TestAuthorityHealthReportsSegmentedCapacityAndThresholds` | A |
| 71 | Silent ambiguity, blocking unknowns, and uncovered Change items are rejected before ACTIVE | `TestChangeRequirementsRejectSilentAmbiguityAndUncoveredContract` | A |
| 72 | Requirement reconciliation is exact; mapped local Evidence is required and external proof remains pending | `TestAutomatedRequirementNeedsMappedCurrentEvidence`; `TestExternalRequirementRemainsPendingInsteadOfBecomingLocalPass` | A |
| 73 | Cancelled edits require explicit supersession and original baseline/lineage carry-forward | `TestCancelledChangeMustCarryOriginalBaseline` | A |
| 74 | Paths infer component/capability/invariant closure and expose under-declaration | `TestInferredImpactSelectsAffectedGatesAndBlocksUnderDeclaration` | A |
| 75 | Gate selectors omit unrelated affected Gates while universal and explicit relation Gates remain required | `TestInferredImpactSelectsAffectedGatesAndBlocksUnderDeclaration` | A |

The deterministic Verdict clause following scenario 75 is covered separately by
`TestVerdictIsDeterministicApartFromObservationTime`: repeated evaluation of the
same exact subject must produce identical decision fields and subject digest;
only `evaluated_at` may differ.

## Required validation protocol

The narrowest relevant checks should run during implementation. Before calling
the v0.3 local candidate internally complete, run at least:

```text
gofmt on changed Go files
go vet ./...
go test ./... -count=1
```

When Plugin/Core source changes, also:

```text
plugin-creator update_plugin_cachebuster.py <plugin-root>
./scripts/package-plugin.sh
plugin-creator validate_plugin.py <plugin-root>
skill-creator quick_validate.py <skill-root>
```

Validate the launcher in a copied deep cache layout, all four binary formats,
manifest version/size/SHA-256/sidecars, and two-build byte-for-byte
reproducibility. The repository tests exercise the cache-layout and corruption
cases. The packaging process still is not signing or notarization.

For the non-Unix rejection/build boundary, the policy is executable on the
current host through `TestAuthorityPlatformPolicyFailsClosedForUnsupportedHost`.
Cross-compilation may additionally use:

```text
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -buildvcs=false -o /private/tmp/ecp-windows-amd64.exe ./cmd/ecp
```

That command proves compilation only. It must never be reported as Windows
runtime validation; Windows authority behavior requires an actual Windows host
if that unsupported-path contract is promoted to release evidence.

## Product and release exit evidence

| Requirement | Current repository evidence | Status |
| --- | --- | --- |
| Local v0.3 Core/CLI semantic loop | Implementation plus the 75-scenario mapping above | Locally automated |
| User does not handwrite product code or operate ECP CLI | Skill contract and CLI hiding rules | Statically specified; real-use proof missing |
| New task recovers without historical chat | Authority truth/policy/history tests | Mechanism proven; independent-project task proof missing |
| Independent real product, at least six ordinary/semantic Changes | `docs/real-project-pilot.md` protocol only | `X` — not performed |
| New maintainer/product person handoff | Pilot protocol only | `X` — not performed |
| Multi-year complexity/capacity/retention | Segmentation, health, and explicit bounds | `X` — not proven; retention/repair/GC absent |
| Team/cross-machine authority continuity | Project Pack is portable; local Evidence is Workspace-bound | `X` — no lineage/import/shared-authority protocol |
| Fresh Codex Desktop install, upgrade, uninstall, new-task pickup | Cache-layout test and local marketplace source | `X` — not performed in a fresh real task |
| Trusted Plugin distribution | Checksums and reproducible package | `X` — unsigned, unnotarized, unpublished |
| Protected CI/release enforcement | `docs/ci-consumer-contract.md` only | `X` — not implemented or deployed |
| Isolated/hermetic Gate execution | Timeout/process-group/bounded-output hardening | `X` — no sandbox/VM, network or same-user isolation |
| Device, production, release, or remote-service truth | Explicitly outside local Evidence | `X` — requires the relevant external systems |

## Honest completion verdict

Passing this repository's checks can establish that the current local v0.3
candidate implements its bounded Core/CLI/Skill contracts on the tested host.
It cannot establish the full North Star product claim. That claim remains
unproven until an independent real-project pilot, newcomer handoff, fresh
desktop distribution validation, and the chosen CI/isolation boundaries provide
their own evidence. Those missing results are product exit blockers, not reasons
to weaken or relabel the acceptance criteria.
