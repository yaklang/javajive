package javaclassparser

import (
	"crypto/sha256"
	"fmt"
)

// Real sourceRewrite rule IDs contracted for T27. Names match dumper.go.
const (
	RewriteFixShadowFieldSuperAssign = "fixShadowFieldSuperAssign"
	RewriteFixEmptySwitchDefault     = "fixEmptySwitchDefault"
	RewriteFixTryBreakReturn         = "fixTryBreakReturn"
)

// RewriteContract describes a narrow production rule: predicates, phase, and
// whether the Java-lexical shield must wrap the string transform.
type RewriteContract struct {
	ID            string
	Phase         string
	Preconditions string
	Shield        bool
	Apply         func(string) string
	Matches       func(string) bool
}

var rewriteContracts = map[string]RewriteContract{
	RewriteFixShadowFieldSuperAssign: {
		ID:            RewriteFixShadowFieldSuperAssign,
		Phase:         "class_source",
		Preconditions: "identifier this.F = this.F; or this.F = (T) (this.F);",
		Shield:        true,
		Apply:         fixShadowFieldSuperAssign,
		Matches: func(src string) bool {
			return fixShadowFieldSuperAssign(src) != src
		},
	},
	RewriteFixEmptySwitchDefault: {
		ID:            RewriteFixEmptySwitchDefault,
		Phase:         "class_source",
		Preconditions: "tab-indented `default:` whose next non-blank line is `}` at the same indent",
		Shield:        true,
		Apply:         fixEmptySwitchDefault,
		Matches: func(src string) bool {
			return fixEmptySwitchDefault(src) != src
		},
	},
	RewriteFixTryBreakReturn: {
		ID:            RewriteFixTryBreakReturn,
		Phase:         "class_source",
		Preconditions: "`try { break; }` inside `} while (true);` followed by a hoisted `return <expr>;`",
		Shield:        true,
		Apply:         fixTryBreakReturn,
		Matches: func(src string) bool {
			return fixTryBreakReturn(src) != src
		},
	},
}

// ContractedRewriteIDs returns the production rule IDs under T27 contracts.
func ContractedRewriteIDs() []string {
	return []string{
		RewriteFixShadowFieldSuperAssign,
		RewriteFixEmptySwitchDefault,
		RewriteFixTryBreakReturn,
	}
}

func lookupRewriteContract(id string) (RewriteContract, bool) {
	c, ok := rewriteContracts[id]
	return c, ok
}

// wrapContractedSourceRule is the sourceRewrite hook: contracted string rules
// run behind rewriteJavaCode so literals/comments/text-blocks are not mutated.
func wrapContractedSourceRule(id string, rule func(string) string) func(string) string {
	spec, ok := rewriteContracts[id]
	if !ok || !spec.Shield || rule == nil {
		return nil
	}
	return func(src string) string {
		return rewriteJavaCode(src, rule)
	}
}

// ApplyContractedRule applies a named production rule with lexical shielding.
func ApplyContractedRule(id, source string) string {
	spec, ok := rewriteContracts[id]
	if !ok || spec.Apply == nil {
		return source
	}
	if spec.Shield {
		return rewriteJavaCode(source, spec.Apply)
	}
	return spec.Apply(source)
}

const defaultRewriteBudget = 64

// RewriteContractEngine is a Mode-isolated registry used by T27 oscillation
// tests and as the contract view of sourceRewrite provenance.
type RewriteContractEngine struct {
	Mode        DecompileMode
	Budget      int
	Seen        map[string]bool
	Chain       []string
	Records     []RewriteRecord
	Diagnostics []DecompileDiagnostic
	applied     int
}

func NewRewriteContractEngine(mode DecompileMode) *RewriteContractEngine {
	return &RewriteContractEngine{
		Mode:   mode,
		Budget: defaultRewriteBudget,
		Seen:   map[string]bool{},
	}
}

func hashSource(s string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(s)))
}

// Apply runs one rule. Precision is a no-op. A→B→A or budget exhaustion
// rejects deterministically and records rewrite_cycle plus the rule chain.
func (e *RewriteContractEngine) Apply(id, phase, source string, rule func(string) string) string {
	if e == nil {
		return source
	}
	if e.Mode == Precision || e.Mode == "" {
		return source
	}
	if rule == nil {
		return source
	}
	if spec, ok := rewriteContracts[id]; ok && spec.Shield {
		rule = wrapContractedSourceRule(id, rule)
	}
	if e.Budget <= 0 {
		e.Budget = defaultRewriteBudget
	}
	if e.applied >= e.Budget {
		e.Diagnostics = append(e.Diagnostics, DecompileDiagnostic{
			Code:    "rewrite_budget",
			Rule:    id,
			Message: "rewrite budget exhausted; chain=" + joinRuleChain(e.Chain),
		})
		return source
	}
	e.applied++
	changed := rule(source)
	if changed == source {
		return source
	}
	before, after := hashSource(source), hashSource(changed)
	if e.Seen == nil {
		e.Seen = map[string]bool{}
	}
	e.Seen[before] = true
	if phase == "class_source" && e.Seen[after] {
		e.Chain = append(e.Chain, id)
		e.Diagnostics = append(e.Diagnostics, DecompileDiagnostic{
			Code:    "rewrite_cycle",
			Rule:    id,
			Message: "Rejected source rewrite that restores an earlier class state. chain=" + joinRuleChain(e.Chain),
		})
		return source
	}
	e.Seen[after] = true
	e.Chain = append(e.Chain, id)
	e.Records = append(e.Records, RewriteRecord{Rule: id, Phase: phase, BeforeHash: before, AfterHash: after})
	e.Diagnostics = append(e.Diagnostics, DecompileDiagnostic{
		Code:    "compatibility_rewrite",
		Rule:    id,
		Message: "Legacy source recovery changed output without bytecode-level proof; validate behavior separately.",
	})
	return changed
}

func joinRuleChain(chain []string) string {
	out := ""
	for i, id := range chain {
		if i > 0 {
			out += "->"
		}
		out += id
	}
	return out
}
