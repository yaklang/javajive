package javaclassparser

import (
	"context"
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/yaklang/javajive/classparser/decompiler/core"

	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
)

// DecompileMode selects a source-recovery policy. Precision excludes the legacy
// class-text repair pipeline; it does not claim to prove arbitrary JVM programs
// equivalent. Compatibility retains that pipeline and reports each applied rule.
type DecompileMode string

// SourceCompilerProfile names the compiler lowering whose anonymous-class and
// access-bridge metadata must be regenerated. A language --release alone does
// not distinguish a native javac8 compiler from modern javac --release 8.
type SourceCompilerProfile string

const (
	ModernJavac  SourceCompilerProfile = "modern-javac"
	NativeJavac8 SourceCompilerProfile = "javac8"
)

func (p SourceCompilerProfile) effective() SourceCompilerProfile {
	if p == "" {
		return ModernJavac
	}
	return p
}

func (p SourceCompilerProfile) validate(sourceVersion int) error {
	switch p {
	case "", ModernJavac:
		return nil
	case NativeJavac8:
		if sourceVersion == 8 {
			return nil
		}
		return fmt.Errorf("javac8 compiler profile requires explicit source version 8")
	default:
		return fmt.Errorf("unknown source compiler profile %q", p)
	}
}

const (
	Precision     DecompileMode = "precision"
	Compatibility DecompileMode = "compatibility"
)

// DecompileOptions is copied at request entry. No process environment is changed.
type DecompileOptions struct {
	// ValidateSyntax is an optional request-local syntax-only validator.
	ValidateSyntax func(context.Context, string) error
	Mode           DecompileMode
	// MaxAnalysisUpdates bounds method-local reaching-definition work; zero uses the default.
	MaxAnalysisUpdates int
	Resolve            func(internalName string) ([]byte, bool)

	// Context is request-scoped cancellation. Nil means no cancel.
	Context context.Context

	// Limits are request-scoped. Zero fields = unlimited.
	// Shared across all methods of the request; not reset per method.
	Limits Limits

	// TargetRelease is the explicit Java release for multi-release JAR
	// selection (JEP 238). Zero means the base tree only (compatible default).
	TargetRelease int

	// EnvSnapshot is a read-only copy of process JDEC_* values taken at
	// request entry. Nil on input means “snapshot now”. After snapshot the
	// request must not re-read os.Getenv for policy. Never os.Setenv.
	EnvSnapshot map[string]string

	// Work, if non-nil, is the request-scoped budget shared with archive IO.
	// Nil means DecompileWithOptions constructs a new budget from Limits.
	Work *WorkBudget

	// TargetSourceVersion is the Java language level of reconstructed source (8, 11, 17, 21, ...).
	// Zero means derive from the input class-file major (major-44, minimum 8). T17 uses this
	// for lossless-vs-unsupported capability checks. Future majors are not auto-interpreted
	// as the latest known Java semantics.
	TargetSourceVersion int

	// SourceCompiler selects metadata regeneration, not a runtime or a
	// Multi-Release namespace. Empty retains modern javac behavior. javac8
	// requires TargetSourceVersion=8 and an actual javac8 rebuild oracle.
	SourceCompiler SourceCompilerProfile
	// Only a JarSourceArtifact can grant postcompile field metadata recovery.
	// This capability is never constructed by a plain-source public request.
	sourceFieldMetadata       *sourceFieldMetadataAuthority
	sourceLambdaMetadata      *sourceLambdaMetadataAuthority
	sourceConstructorLowering *sourceConstructorLoweringAuthority
	// Header policy for one physical source root. Native ownership readers
	// may retain this exact root's ClassFile access without borrowing any
	// executable lowering authority. Copied lexical child options do not match.
	reconstructionPhysicalRoot *ClassObject

	// RetainUnprovedSource keeps the original member when a javac source
	// certificate fails but the bytecode still has a body or a unique target.
	// Analyzers that only parse the text use it. It does not make the text
	// recompilable and it does not replace a refused body with a compiler factory.
	// The default remains an explicit refusal.
	RetainUnprovedSource bool

	// EnableShadowIR builds a read-only MethodIR snapshot after SemanticCFG.
	// The old printer remains the default source path. Default false.
	EnableShadowIR bool

	// EnableRecoveryEvidence records bounded, request-local observations of
	// existing recovery rules. It grants no additional rewrite permission.
	EnableRecoveryEvidence bool
}

type DecompileDiagnostic struct {
	Code    string `json:"code"`
	Method  string `json:"method,omitempty"`
	Rule    string `json:"rule,omitempty"`
	Message string `json:"message"`
}
type RewriteRecord struct {
	Rule       string `json:"rule"`
	Phase      string `json:"phase"`
	BeforeHash string `json:"before_hash"`
	AfterHash  string `json:"after_hash"`
}
type DecompileResult struct {
	Members                   []MemberRecord                  `json:"members"`
	Shadow                    []core.ShadowObservation        `json:"shadow"`
	Syntax                    ValidationObservation           `json:"syntax"`
	Source                    string                          `json:"source"`
	Mode                      DecompileMode                   `json:"mode"`
	InputHash                 string                          `json:"input_hash"`
	Status                    string                          `json:"status"`
	StubMethods               []string                        `json:"stub_methods"`
	RulesApplied              []RewriteRecord                 `json:"rules_applied"`
	Diagnostics               []DecompileDiagnostic           `json:"diagnostics"`
	EffectiveConfig           *EffectiveConfig                `json:"effective_config,omitempty"`
	ShadowIRHash              string                          `json:"shadow_ir_hash,omitempty"`
	ShadowIRVersion           uint64                          `json:"shadow_ir_version,omitempty"`
	RecoveryEvidence          []RecoveryObligationObservation `json:"recovery_evidence,omitempty"`
	RecoveryEvidenceEnabled   bool                            `json:"recovery_evidence_enabled,omitempty"`
	RecoveryEvidenceTruncated bool                            `json:"recovery_evidence_truncated,omitempty"`
}

// EffectiveConfig is the request-local snapshot of policy actually used.
// complete does not mean compile, JVM verify, or behavioral equivalence.
type EffectiveConfig struct {
	Mode               DecompileMode         `json:"mode"`
	MaxAnalysisUpdates int                   `json:"max_analysis_updates"`
	TargetRelease      int                   `json:"target_release"`
	Limits             Limits                `json:"limits"`
	Env                []string              `json:"env,omitempty"`
	Resolver           bool                  `json:"resolver"`
	SourceCompiler     SourceCompilerProfile `json:"source_compiler,omitempty"`
}

// DecompileWithOptions provides an explicit policy and degradation evidence.
// Compilation, JVM verification and behavioral equivalence are intentionally
// not success fields here: only an external execution harness can measure them.
func DecompileWithOptions(data []byte, options DecompileOptions) (result DecompileResult, err error) {
	result, _, err = decompileWithBudget(data, options)
	return result, err
}

func decompileWithBudget(data []byte, options DecompileOptions) (result DecompileResult, budget *workbudget.Budget, err error) {
	options = copyDecompileOptions(options)
	if options.Mode == "" {
		options.Mode = Precision
	}
	result = DecompileResult{
		Mode:                    options.Mode,
		Status:                  "invalid_input",
		EffectiveConfig:         effectiveConfigOf(options),
		RecoveryEvidenceEnabled: options.EnableRecoveryEvidence,
	}
	if options.Mode != Precision && options.Mode != Compatibility {
		return result, nil, fmt.Errorf("unknown decompile mode %q", options.Mode)
	}
	if err := options.SourceCompiler.validate(options.TargetSourceVersion); err != nil {
		return result, nil, err
	}
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("decompile: %v", v)
			if budget != nil && budget.Err() != nil {
				err = budget.Err()
				result.Status = statusForDecompileError(err)
				result.Source = ""
				return
			}
			if result.Status != "invalid_input" {
				result.Status = "unsupported"
			}
		}
	}()
	if options.MaxAnalysisUpdates < 0 {
		return result, nil, fmt.Errorf("negative analysis budget")
	}
	if err := options.Limits.Validate(); err != nil {
		return result, nil, err
	}
	budget = options.Work
	if budget == nil {
		budget = workbudget.New(options.Context, options.Limits)
	}
	if err := budget.CheckContext(options.Context); err != nil {
		result.Status = statusForDecompileError(err)
		return result, budget, err
	}
	// Cancellation and input admission precede hashing and all parser allocations.
	reader, err := NewClassReaderWithBudget(data, budget)
	if err != nil {
		result.Status = statusForDecompileError(err)
		return result, budget, err
	}
	reader.context = options.Context
	result.InputHash = fmt.Sprintf("%x", sha256.Sum256(data))
	obj, err := parseWithReader(reader)
	if err != nil {
		result.Status = StatusClassifies(err)
		if workbudget.Is(err) {
			result.Status = statusForDecompileError(err)
		}
		return result, budget, err
	}
	// Parse remains permissive for unknown versions so structural callers can
	// inspect them. A decompile request, however, must not silently apply
	// Java-21 source semantics to a classfile outside the supported range.
	if err := ValidateSupportedClassMajor(obj.MajorVersion); err != nil {
		result.Status = "unsupported"
		return result, budget, err
	}
	if err := ValidateClassMinor(obj.MajorVersion, obj.MinorVersion); err != nil {
		if strings.Contains(err.Error(), "unsupported_classfile_preview") {
			result.Status = "unsupported"
		} else {
			result.Status = "invalid_input"
		}
		return result, budget, err
	}
	result.Members = newMemberLedger(obj)
	result.Syntax = ValidationObservation{Status: "not_run"}
	defer func() {
		finalizeMemberStatus(&result)
		if err == nil {
			observeSyntax(&result, options)
		}
	}()
	result.Status = "unsupported"
	d := NewClassObjectDumper(obj)
	d.options = options
	d.report = &result
	d.Work = budget
	d.foldSiblingResolver = wrapResolve(options.Context, budget, options.EnvSnapshot, options.Resolve)
	runErr := jdecenv.Run(options.EnvSnapshot, func() error {
		var inner error
		result.Source, inner = d.DumpClass()
		return inner
	})
	err = runErr
	if err == nil {
		if budget != nil && budget.Err() != nil {
			err = budget.Err()
		} else if options.Context != nil && options.Context.Err() != nil {
			err = options.Context.Err()
		}
	}
	if err != nil {
		result.Status = statusForDecompileError(err)
		if result.Status == "resource_limit" || result.Status == "canceled" {
			result.Source = ""
		}
		if result.Status == "complete" || result.Status == "" {
			result.Status = "unsupported"
		}
		applyBootstrapCapabilities(&result, d)
		noteUnsupportedBootstraps(&result, obj)
		return result, budget, err
	}
	// Pattern-switch reconstruction replaces a whole method body after the
	// legacy structurer has already recorded a deconstruction-handler stub.
	// Apply the final source rewrite and prune that superseded stub before the
	// result status and member ledger are finalized.
	applyPatternSwitchSource(d, &result)
	result.Status = "complete"
	if d.overloadFamilyUnproven || (d.FuncCtx != nil && d.FuncCtx.OverloadFamilyUnproven) {
		result.Status = "unsupported"
		d.appendDiagnostic(DecompileDiagnostic{
			Code:    "overload_family_unknown",
			Message: "external overload family was not fully resolved; reconstructed binding is unverified",
		})
	}
	if len(result.StubMethods) > 0 || d.typeAnnosUnsupported || d.enumParameterMetadataUnsupported {
		if result.Status == "complete" {
			result.Status = "partial"
		}
	}
	applyBootstrapCapabilities(&result, d)
	noteUnsupportedBootstraps(&result, obj)
	if err := budget.CheckContext(options.Context); err != nil {
		result.Status = statusForDecompileError(err)
		result.Source = ""
		return result, budget, err
	}
	return result, budget, nil
}

// rewriteHashMemoLimit bounds source states whose SHA-256 is retained. A
// class-source sequence normally changes only a handful of times; the cap
// keeps a pathological oscillation from retaining many full source strings.
const rewriteHashMemoLimit = 64

// rewriteSchedulerMetrics counts rule-level decisions for one dumper. It is
// request-local instrumentation and is never published as a diagnostic, so
// enabling the scheduler cannot change Observable output.
type rewriteSchedulerMetrics struct {
	Considered       int // every Compatibility sourceRewrite call
	Candidates       int // passed the necessary-literal prefilter (or Always)
	Invoked          int // the rule implementation was actually entered
	Skipped          int // rejected by the prefilter without invoking the rule
	HashComputations int // SHA-256 calls; repeated source states reuse the memo
}

// rewriteSchedulerOff is the bisection kill switch. The conservative prefilter
// is enabled by default; JDEC_REWRITE_SCHEDULER_OFF=1 restores unconditional
// invocation without changing order, hashing or diagnostics.
func (c *ClassObjectDumper) rewriteSchedulerOff() bool {
	if c.rewriteSchedulerDisabled {
		return true
	}
	if !c.rewriteSchedulerSwitchReady {
		c.rewriteSchedulerSwitchReady = true
		c.rewriteSchedulerSwitchOff = jdecenv.Get("JDEC_REWRITE_SCHEDULER_OFF") == "1"
	}
	return c.rewriteSchedulerSwitchOff
}

// applicabilityIndexFor returns the index for this exact source state. The
// epoch and source string are both checked: an accepted rewrite advances the
// epoch and clears the index, and a rollback/retry that restores an older
// source cannot reuse the newer state's applicability.
func (c *ClassObjectDumper) applicabilityIndexFor(source string) *rewriteApplicabilityIndex {
	if c.rewriteIndex == nil || c.rewriteIndex.epoch != c.rewriteEpoch || c.rewriteIndex.source != source {
		c.rewriteIndex = &rewriteApplicabilityIndex{
			source:   source,
			epoch:    c.rewriteEpoch,
			presence: make(map[string]bool, len(rewriteRuleSpecs)),
		}
	}
	return c.rewriteIndex
}

// rewriteRuleMayChange is a necessary-condition gate, never a proof of
// applicability. Rules without registered metadata (including every
// shaded-library family) are Always applicable.
func (c *ClassObjectDumper) rewriteRuleMayChange(id, phase, source string) bool {
	spec, ok := lookupRewriteRuleSpec(id, phase)
	if !ok {
		return true
	}
	return rewriteSpecApplicable(spec, c.applicabilityIndexFor(source))
}

// memoizedSourceHash hashes each exact source state at most once until the
// bounded memo is reset. It preserves the historical SHA-256 hex values.
func (c *ClassObjectDumper) memoizedSourceHash(source string) string {
	if hash, ok := c.rewriteHashes[source]; ok {
		return hash
	}
	hash := hashSource(source)
	c.rewriteSchedulerStats.HashComputations++
	if c.rewriteHashes == nil {
		c.rewriteHashes = make(map[string]string, 8)
	} else if len(c.rewriteHashes) >= rewriteHashMemoLimit {
		c.rewriteHashes = make(map[string]string, 8)
	}
	c.rewriteHashes[source] = hash
	return hash
}

func (c *ClassObjectDumper) sourceRewrite(id, phase, source string, rule func(string) string) string {
	if c.options.Mode == Precision {
		return source
	}
	c.rewriteSchedulerStats.Considered++
	// The prefilter runs before contract shielding, anonymous-scope masking,
	// hash computation and ordinary-constructor drafts. A missing literal
	// means the rule cannot change any slice of this source, so skipping the
	// whole invocation is output-equivalent.
	if !c.rewriteSchedulerOff() && !c.rewriteRuleMayChange(id, phase, source) {
		c.rewriteSchedulerStats.Skipped++
		return source
	}
	c.rewriteSchedulerStats.Candidates++
	if wrapped := wrapContractedSourceRule(id, rule); wrapped != nil {
		rule = wrapped
	}
	invoked := false
	scheduledRule := rule
	rule = func(input string) string {
		if !invoked {
			invoked = true
			c.rewriteSchedulerStats.Invoked++
		}
		return scheduledRule(input)
	}
	changed := source
	// Inline anonymous members have already completed recovery under their
	// original method context. Neither caller nor class recovery may borrow a
	// different method's declarations to reinterpret that lexical scope.
	lexicalPhase := phase == "method_source" || phase == "class_source"
	declarations := []string{}
	retained := int64(0)
	if lexicalPhase && c.nativeMemberRoot != nil {
		for _, local := range c.nativeMemberRoot.methodLocals {
			if local == nil || local.owner == nil || !nativeProofWork(c.Work, 1) {
				return source
			}
			if local.source == "" || local.object == c.obj {
				continue
			}
			if phase == "method_source" && (c.obj == nil || c.FuncCtx == nil || local.owner.owner != c.obj.GetClassName() || local.executionOwner().method != c.FuncCtx.FunctionName || local.executionOwner().descriptor != c.FuncCtx.CurrentMethodDesc) {
				continue
			}
			retained += int64(len(local.source))
			if retained > 1<<20 || len(declarations) >= nativeMemberLayoutNodeLimit {
				return source
			}
			declarations = append(declarations, local.source)
		}
	}
	recoverScope := func(input string) string {
		if lexicalPhase && (c.nativeAnonymousRoot != nil || c.nativeAnonymousForest != nil || len(declarations) > 0) {
			// Bound the scan before formatting. Protection never licenses a rewrite.
			scanWork, err := workbudget.CheckedProduct(int64(len(input)), int64(len(declarations)+1))
			if err == nil && nativeProofWork(c.Work, scanWork+retained+1) && (c.Work == nil || c.Work.CheckAlloc(int64(len(input))*4+retained+int64(len(declarations))*128) == nil) {
				return nativeRewriteEnclosingScope(input, rule, declarations...)
			}
			return input
		}
		return rule(input)
	}
	var ordinaryDraft *ordinaryConstructorClassSourceDraft
	if phase == "class_source" && c.ordinaryConstructorClassSource != nil {
		changed, ordinaryDraft = c.ordinaryConstructorClassSource.rewrite(source, recoverScope)
	} else {
		changed = recoverScope(source)
	}
	if c.Work != nil {
		if err := c.Work.CheckAlloc(int64(len(changed))); err != nil {
			return source
		}
	}
	var before, after string
	if changed != source && c.report != nil {
		before, after = c.memoizedSourceHash(source), c.memoizedSourceHash(changed)
		// The class phase is one bounded sequence. Reject oscillation between
		// rules; method phases have separate contexts and are not compared.
		if phase == "class_source" {
			if c.rewriteSeen == nil {
				c.rewriteSeen = map[string]bool{}
			}
			c.rewriteSeen[before] = true
			if c.rewriteSeen[after] {
				c.appendDiagnostic(DecompileDiagnostic{Code: "rewrite_cycle", Rule: id, Message: "Rejected source rewrite that restores an earlier class state."})
				return source
			}
			c.rewriteSeen[after] = true
		}
	}
	if ordinaryDraft != nil && !c.ordinaryConstructorClassSource.commit(ordinaryDraft) {
		return source
	}
	if changed != source {
		// An accepted rewrite invalidates every cached applicability decision.
		c.rewriteEpoch++
		c.rewriteIndex = nil
		if c.report != nil {
			c.report.RulesApplied = append(c.report.RulesApplied, RewriteRecord{Rule: id, Phase: phase, BeforeHash: before, AfterHash: after})
			c.appendDiagnostic(DecompileDiagnostic{Code: "compatibility_rewrite", Rule: id, Message: "Legacy source recovery changed output without bytecode-level proof; validate behavior separately."})
		}
	}
	return changed
}
