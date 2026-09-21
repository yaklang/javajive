package javaclassparser

import (
	"crypto/sha256"
	"fmt"
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
	Mode DecompileMode
	// MaxAnalysisUpdates bounds method-local reaching-definition work; zero uses the default.
	MaxAnalysisUpdates int
	Resolve            func(internalName string) ([]byte, bool)
	// EnvSnapshot is an optional request-local JDEC_* map (T31). Nil means
	// kill-switches still read os.Getenv. Resources owner fills this at request entry.
	EnvSnapshot map[string]string
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
	Source       string                `json:"source"`
	Mode         DecompileMode         `json:"mode"`
	InputHash    string                `json:"input_hash"`
	Status       string                `json:"status"`
	StubMethods  []string              `json:"stub_methods"`
	RulesApplied []RewriteRecord       `json:"rules_applied"`
	Diagnostics  []DecompileDiagnostic `json:"diagnostics"`
}

// DecompileWithOptions provides an explicit policy and degradation evidence.
// Compilation, JVM verification and behavioral equivalence are intentionally
// not success fields here: only an external execution harness can measure them.
func DecompileWithOptions(data []byte, options DecompileOptions) (result DecompileResult, err error) {
	if options.Mode == "" {
		options.Mode = Precision
	}
	result = DecompileResult{Mode: options.Mode, InputHash: fmt.Sprintf("%x", sha256.Sum256(data)), Status: "invalid_input"}
	if options.Mode != Precision && options.Mode != Compatibility {
		return result, fmt.Errorf("unknown decompile mode %q", options.Mode)
	}
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("decompile: %v", v)
			if result.Status != "invalid_input" {
				result.Status = "unsupported"
			}
		}
	}()
	if options.MaxAnalysisUpdates < 0 {
		return result, fmt.Errorf("negative analysis budget")
	}
	obj, err := Parse(data)
	if err != nil {
		return result, err
	}
	result.Status = "unsupported"
	d := NewClassObjectDumper(obj)
	d.options = options
	d.report = &result
	d.foldSiblingResolver = options.Resolve
	result.Source, err = d.DumpClass()
	if err != nil {
		return result, err
	}
	result.Status = "complete"
	if len(result.StubMethods) > 0 {
		result.Status = "partial"
	}
	return result, nil
}

func (c *ClassObjectDumper) sourceRewrite(id, phase, source string, rule func(string) string) string {
	if c.options.Mode == Precision {
		return source
	}
	changed := rule(source)
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
				c.report.Diagnostics = append(c.report.Diagnostics, DecompileDiagnostic{Code: "rewrite_cycle", Rule: id, Message: "Rejected source rewrite that restores an earlier class state."})
				return source
			}
			c.rewriteSeen[after] = true
		}
		c.report.RulesApplied = append(c.report.RulesApplied, RewriteRecord{Rule: id, Phase: phase, BeforeHash: fmt.Sprintf("%x", sha256.Sum256([]byte(source))), AfterHash: fmt.Sprintf("%x", sha256.Sum256([]byte(changed)))})
		c.report.Diagnostics = append(c.report.Diagnostics, DecompileDiagnostic{Code: "compatibility_rewrite", Rule: id, Message: "Legacy source recovery changed output without bytecode-level proof; validate behavior separately."})
	}
	return changed
}
