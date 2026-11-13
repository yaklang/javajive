package javaclassparser

import (
	"crypto/sha256"
	"fmt"
	"strings"
)

// Real sourceRewrite rule IDs contracted for T27. Names match dumper.go.
const (
	RewriteFixShadowFieldSuperAssign = "fixShadowFieldSuperAssign"
	RewriteFixTryBreakReturn         = "fixTryBreakReturn"
)

// Deprecated: empty source labels do not prove an exceptional method exit.
// Retained as an identifier for readers of historical rewrite reports only.
const RewriteFixEmptySwitchDefault = "fixEmptySwitchDefault"

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
	var out strings.Builder
	for i, id := range chain {
		if i > 0 {
			out.WriteString("->")
		}
		out.WriteString(id)
	}
	return out.String()
}

// The Compatibility class-source pipeline is one ordered sequence. This list
// mirrors the call sites in dumper.go byte for byte, including every repeated
// rule entry. It is metadata only: production still calls the rules inline in
// this exact order. Never append, sort or deduplicate this list.
var classSourceRewriteOrder = []string{
	"fixLambdaLoopCapture",
	"removeUnreachableSwitchBreak",
	"fixTryBreakReturn",
	"fixThrowTypeVarCast",
	"fixLoopCatchThrow",
	"fixTryCatchExceptionPlacement",
	"wrapUncaughtThrowingCall",
	"wrapReflectionCallInSwitchCase",
	"wrapFieldInitializerReflection",
	"wrapUncaughtGetConstructor",
	"fixAlreadyCaughtDuplicateCatch",
	"fixShadowFieldSuperAssign",
	"fixImmutableBuilderWitness",
	"fixSelectMethodsAmbiguous",
	"fixAsMapFunctionRawCast",
	"fixThrowClassCastDropsThrowable",
	"fixReplaceAllListStreamCast",
	"fixClassMapEntryPutCasts",
	"fixVisitAnnotationConsumerCast",
	"fixGetUninterruptiblyGetInTry",
	"fixDrainUninterruptiblyPollInTry",
	"fixTransposeCellFunctionCast",
	"fixPreferringStringsAsListCast",
	"fixSyncLazyNavigableReturn",
	"fixCatchingFutureFinally",
	"fixAbstractServiceFinally",
	"fixSyncHelperCasReturn",
	"fixRescheduleUnlockFinally",
	"fixDupThrowableCatchFinally",
	"fixAppEngineCNFECatch",
	"fixRateLimiterTryAcquireReturn",
	"fixConvertClassValuesObjectArray",
	"fixToAnnotationArrayFinisher",
	"fixDataBufferLambdaCast",
	"fixLinkedWithNextRawCast",
	"fixJacksonFeatureUpcast",
	"fixAnnotatedAndMetadataLambda",
	"fixJacksonRemainingReconstructs",
	"fixCloseResourceThrows",
	"fixTransmitterEmptySync",
	"fixExchangeFinderEmptySync",
	"fixRealConnectionConnect",
	"fixConnectionPoolCleanup",
	"fixHttp2NewStream",
	"fixHttp2StreamEmptySync",
	"fixEmptySyncMissingReturn",
	"fixSkipAsciiWhitespaceContinue",
	"fixDiskLruIteratorHasNext",
	"fixPublicSuffixDatabase",
	"fixCollections4RemainingReconstructs",
	"fixNettyRemainingReconstructs",
	"fixLog4jRemainingReconstructs",
	"fixOrig14RemainderReconstructs",
	"fixProtobufRemainingReconstructs",
	"fixJedisRemainingReconstructs",
	"fixLogbackRemainingReconstructs",
	"fixHikaricpRemainingReconstructs",
	"fixPool2RemainingReconstructs",
	"fixPicocliRemainingReconstructs",
	"fixHttpclientRemainingReconstructs",
	"fixJavassistRemainingReconstructs",
	"fixCommonsIoRemainingReconstructs",
	"fixCompressRemainingReconstructs",
	"fixCaffeineRemainingReconstructs",
	"fixRxjavaRemainingReconstructs",
	"fixMath3RemainingReconstructs",
	"fixAssertjRemainingReconstructs",
	"fixZxingRemainingReconstructs",
	"fixEnumNoArgCtorThisAfterLocals",
	"fixCtorNPECheckBeforeThis",
	"fixCtorDelegationArgumentSpills",
	"fixEnumClinitIllegalNew",
	"fixKotlinEnumEntries",
	"fixBareNestedImports",
	"fixHardjarShapes",
	"fixBoolUsedAsArithOperand",
	"fixBooleanZeroLiteral",
	"fixIntCmpBoolMaterializedLiteral",
	"fixBoundedStreamWiden",
	"fixObjectUsedAsInt",
	"fixIntUsedAsInstanceof",
	"fixXstreamRemainingReconstructs",
	"fixFreemarkerRemainingReconstructs",
	"fixThrowObjectAsThrowable",
	"fixClassDollarForwardRef",
	"fixThrowInitCauseCast",
	"fixMutinyPublisherCast",
	"fixSpringMethodRefCasts",
	"fixFluxCreateDataBufferWitness",
	"fixUrlResourceURISyntax",
	"fixPercNSMECatch",
	"fixFutureAdapterReturn",
	"fixLeakedExceptionSentinel",
	"fixIdentSelfCast",
	"wrapThrowTargetException",
	"fixXstreamRemainingReconstructs",
	"fixJavassistRemainingReconstructs",
	"fixOrig14RemainderReconstructs",
	"fixJacksonRemainingReconstructs",
	"fixNettyRemainingReconstructs",
	"fixLog4jRemainingReconstructs",
	"fixIdentAsTypeDecl",
	"fixObjectInitCastType",
}

// methodSourceRewriteOrder mirrors the three method-local passes. They run per
// method and therefore have no single global position relative to the class
// sequence above; this list locks their order within each method only.
var methodSourceRewriteOrder = []string{
	"hoistSameTypeEscapedLocals",
	"hoistCastGuardedEscapedLocals",
	"addMissingGeneratedLocalDecls",
}

// rewriteRuleKey names one rule in one phase. A rule may carry different
// necessary conditions in different phases, so the phase is part of the key.
type rewriteRuleKey struct {
	ID    string
	Phase string
}

// rewriteNeedleSet is one DNF alternative: every literal must be present in
// the source before the rule can change it. An alternative is not a sufficient
// condition; it only permits invocation.
type rewriteNeedleSet []string

// rewriteRuleSpec is immutable applicability metadata. An empty Alternatives
// list means the rule is Always applicable and must execute conservatively.
type rewriteRuleSpec struct {
	Alternatives []rewriteNeedleSet
}

// rewriteRuleSpecs holds necessary-condition metadata for the small, audited
// prefilter set. Rules absent from this map are Always applicable. The map is
// initialized once and never mutated. Every needle below is a literal that the
// rule implementation itself requires (either an explicit early guard or an
// exact pattern it must find), so absence proves the rule cannot change source.
//
// Shaded-library rules intentionally remain Always: canonical package names
// (for example com.google.zxing or freemarker.) can be relocated by shading,
// and using them here would make a necessary condition unreliable.
var rewriteRuleSpecs = map[rewriteRuleKey]rewriteRuleSpec{
	{ID: "removeUnreachableSwitchBreak", Phase: "class_source"}: {
		// The rule only removes a line matching ^(\t+)break;$.
		Alternatives: []rewriteNeedleSet{{"break;"}},
	},
	{ID: "fixTryBreakReturn", Phase: "class_source"}: {
		// Requires `} while (true);`, an in-try `break;`, and a hoisted return.
		Alternatives: []rewriteNeedleSet{{"} while (true);", "break;", "return "}},
	},
	{ID: "fixThrowTypeVarCast", Phase: "class_source"}: {
		// Requires a `throws T {` signature and a `throw <var>;` statement.
		// The keywords (not the following whitespace) are the necessary
		// condition: the implementation accepts any \s+ separator.
		Alternatives: []rewriteNeedleSet{{"throws", "throw"}},
	},
	{ID: "fixLoopCatchThrow", Phase: "class_source"}: {
		// Requires a catch with `throw new RuntimeException(...)` followed by a
		// loop transfer (continue or break).
		Alternatives: []rewriteNeedleSet{
			{"catch(", "throw new RuntimeException(", "continue"},
			{"catch(", "throw new RuntimeException(", "break"},
		},
	},
	{ID: "wrapUncaughtThrowingCall", Phase: "class_source"}: {
		// Matches return lines containing UNSAFE.allocateInstance(...).
		Alternatives: []rewriteNeedleSet{{"allocateInstance("}},
	},
	{ID: "wrapFieldInitializerReflection", Phase: "class_source"}: {
		// The field-initializer pattern requires one of these method names.
		Alternatives: []rewriteNeedleSet{
			{"getConstructor("}, {"getDeclaredConstructor("},
			{"getMethod("}, {"getDeclaredMethod("},
			{"getDeclaredField("}, {"getField("},
		},
	},
	{ID: "wrapReflectionCallInSwitchCase", Phase: "class_source"}: {
		// The return-statement pattern requires one of these method names.
		Alternatives: []rewriteNeedleSet{
			{"getConstructor("}, {"getDeclaredConstructor("},
			{"getMethod("}, {"getDeclaredMethod("},
			{"invokeExact("}, {"metafactory("},
			{"findStatic("}, {"findVirtual("},
		},
	},
	{ID: "fixTryCatchExceptionPlacement", Phase: "class_source"}: {
		// The throwing-call regex requires a dotted call to one of these names.
		Alternatives: []rewriteNeedleSet{
			{".getDeclaredConstructor("}, {".newInstance("},
			{".getMethod("}, {".getConstructor("},
		},
	},
	{ID: "fixImmutableBuilderWitness", Phase: "class_source"}: {
		// The regex requires an Immutable* type and its .builder() call.
		Alternatives: []rewriteNeedleSet{{"Immutable", ".builder("}},
	},
	{ID: "fixIntCmpBoolMaterializedLiteral", Phase: "class_source"}: {
		// Both exact replacements contain `!= (0))`.
		Alternatives: []rewriteNeedleSet{{"!= (0))"}},
	},
	{ID: "fixBoolUsedAsArithOperand", Phase: "class_source"}: {
		// Every changed boolean local/field is declared with `boolean `.
		Alternatives: []rewriteNeedleSet{{"boolean "}},
	},
	{ID: "fixBooleanZeroLiteral", Phase: "class_source"}: {
		// The rule only rewrites chunks belonging to a `boolean varN` local.
		Alternatives: []rewriteNeedleSet{{"boolean var"}},
	},
	{ID: "fixClassDollarForwardRef", Phase: "class_source"}: {
		// The rule walks from this exact declaration prefix.
		Alternatives: []rewriteNeedleSet{{"static Class class$"}},
	},
	{ID: "fixThrowInitCauseCast", Phase: "class_source"}: {
		// The constructor chain must start with the exact `().initCause(` suffix.
		Alternatives: []rewriteNeedleSet{{"().initCause("}},
	},
	{ID: "fixShadowFieldSuperAssign", Phase: "class_source"}: {
		// Form 1 needs ` = this.`; form 2 needs `) (this.`.
		Alternatives: []rewriteNeedleSet{
			{" = this."},
			{") (this."},
		},
	},
	{ID: "fixLeakedExceptionSentinel", Phase: "class_source"}: {
		// The rule returns before scanning unless the shielded code contains one
		// of these exact diagnostic sentinels.
		Alternatives: []rewriteNeedleSet{
			{"= Exception;"},
			{"= Exception\n"},
			{"return Exception;"},
			{"throw Exception;"},
		},
	},
	{ID: "fixSpringMethodRefCasts", Phase: "class_source"}: {
		// Every exact replacement contains a method-reference `::`.
		Alternatives: []rewriteNeedleSet{{"::"}},
	},
}

func lookupRewriteRuleSpec(id, phase string) (rewriteRuleSpec, bool) {
	spec, ok := rewriteRuleSpecs[rewriteRuleKey{ID: id, Phase: phase}]
	return spec, ok
}

// rewriteApplicabilityIndex caches literal presence for one immutable source
// state. Callers must rebuild it whenever the source changes; the dumper also
// stamps it with a mutation epoch so a stale index is never consulted.
type rewriteApplicabilityIndex struct {
	source   string
	epoch    uint64
	presence map[string]bool
}

func (i *rewriteApplicabilityIndex) contains(needle string) bool {
	if present, ok := i.presence[needle]; ok {
		return present
	}
	present := strings.Contains(i.source, needle)
	i.presence[needle] = present
	return present
}

// rewriteSpecApplicable evaluates DNF: at least one alternative must have every
// one of its literal needles present. It never proves applicability; it only
// rejects rules whose required literals are provably absent.
func rewriteSpecApplicable(spec rewriteRuleSpec, index *rewriteApplicabilityIndex) bool {
	if len(spec.Alternatives) == 0 {
		return true
	}
	for _, alternative := range spec.Alternatives {
		if len(alternative) == 0 {
			return true
		}
		present := true
		for _, needle := range alternative {
			if !index.contains(needle) {
				present = false
				break
			}
		}
		if present {
			return true
		}
	}
	return false
}

// rewriteRuleApplicable is the index-free form used by focused tests.
func rewriteRuleApplicable(id, phase, source string) bool {
	spec, ok := lookupRewriteRuleSpec(id, phase)
	if !ok {
		return true
	}
	return rewriteSpecApplicable(spec, &rewriteApplicabilityIndex{
		source:   source,
		presence: make(map[string]bool),
	})
}
