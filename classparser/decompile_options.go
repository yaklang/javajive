package javaclassparser

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"

	"github.com/yaklang/javajive/internal/jdecenv"
	"github.com/yaklang/javajive/internal/workbudget"
)

// DecompileMode selects a source-recovery policy. Precision excludes the legacy
// class-text repair pipeline; it does not claim to prove arbitrary JVM programs
// equivalent. Compatibility retains that pipeline and reports each applied rule.
type DecompileMode string

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

	// EnableShadowIR builds a read-only MethodIR snapshot after SemanticCFG.
	// The old printer remains the default source path. Default false.
	EnableShadowIR bool
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
	Members         []MemberRecord           `json:"members"`
	Shadow          []core.ShadowObservation `json:"shadow"`
	Syntax          ValidationObservation    `json:"syntax"`
	Source          string                   `json:"source"`
	Mode            DecompileMode            `json:"mode"`
	InputHash       string                   `json:"input_hash"`
	Status          string                   `json:"status"`
	StubMethods     []string                 `json:"stub_methods"`
	RulesApplied    []RewriteRecord          `json:"rules_applied"`
	Diagnostics     []DecompileDiagnostic    `json:"diagnostics"`
	EffectiveConfig *EffectiveConfig         `json:"effective_config,omitempty"`
	ShadowIRHash    string                   `json:"shadow_ir_hash,omitempty"`
	ShadowIRVersion uint64                   `json:"shadow_ir_version,omitempty"`
}

// EffectiveConfig is the request-local snapshot of policy actually used.
// complete does not mean compile, JVM verify, or behavioral equivalence.
type EffectiveConfig struct {
	Mode               DecompileMode `json:"mode"`
	MaxAnalysisUpdates int           `json:"max_analysis_updates"`
	TargetRelease      int           `json:"target_release"`
	Limits             Limits        `json:"limits"`
	Env                []string      `json:"env,omitempty"`
	Resolver           bool          `json:"resolver"`
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
		Mode:            options.Mode,
		Status:          "invalid_input",
		EffectiveConfig: effectiveConfigOf(options),
	}
	if options.Mode != Precision && options.Mode != Compatibility {
		return result, nil, fmt.Errorf("unknown decompile mode %q", options.Mode)
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
	result.Status = "complete"
	if d.overloadFamilyUnproven || (d.FuncCtx != nil && d.FuncCtx.OverloadFamilyUnproven) {
		result.Status = "unsupported"
		d.appendDiagnostic(DecompileDiagnostic{
			Code:    "overload_family_unknown",
			Message: "external overload family was not fully resolved; reconstructed binding is unverified",
		})
	}
	if len(result.StubMethods) > 0 || d.typeAnnosUnsupported {
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

func (c *ClassObjectDumper) sourceRewrite(id, phase, source string, rule func(string) string) string {
	if c.options.Mode == Precision {
		return source
	}
	if wrapped := wrapContractedSourceRule(id, rule); wrapped != nil {
		rule = wrapped
	}
	changed := rule(source)
	if c.Work != nil {
		if err := c.Work.CheckAlloc(int64(len(changed))); err != nil {
			return source
		}
	}
	if changed != source && c.report != nil {
		before, after := fmt.Sprintf("%x", sha256.Sum256([]byte(source))), fmt.Sprintf("%x", sha256.Sum256([]byte(changed)))
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
		c.report.RulesApplied = append(c.report.RulesApplied, RewriteRecord{Rule: id, Phase: phase, BeforeHash: fmt.Sprintf("%x", sha256.Sum256([]byte(source))), AfterHash: fmt.Sprintf("%x", sha256.Sum256([]byte(changed)))})
		c.appendDiagnostic(DecompileDiagnostic{Code: "compatibility_rewrite", Rule: id, Message: "Legacy source recovery changed output without bytecode-level proof; validate behavior separately."})
	}
	return changed
}
