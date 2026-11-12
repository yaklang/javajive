package javaclassparser

import (
	"crypto/sha256"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/jdecenv"
)

// rewriteRuleExpectedOutput applies the same contract shield sourceRewrite
// applies, without consulting the scheduler.
func rewriteRuleExpectedOutput(id string, rule func(string) string, source string) string {
	if wrapped := wrapContractedSourceRule(id, rule); wrapped != nil {
		return wrapped(source)
	}
	return rule(source)
}

// TestRewriteSchedulerOrderMatchesDumperCallSites locks the immutable ordered
// registry to the actual inline call sites, including every repeated entry and
// the method-local phase order. A reordering of dumper.go must update the
// registry deliberately.
func TestRewriteSchedulerOrderMatchesDumperCallSites(t *testing.T) {
	data, err := os.ReadFile("dumper.go")
	if err != nil {
		t.Fatal(err)
	}
	callRe := regexp.MustCompile(`c\.sourceRewrite\("([^"]+)", "(class_source|method_source)"`)
	var gotClass, gotMethod []string
	for _, match := range callRe.FindAllStringSubmatch(string(data), -1) {
		switch match[2] {
		case "class_source":
			gotClass = append(gotClass, match[1])
		case "method_source":
			gotMethod = append(gotMethod, match[1])
		}
	}
	if !slices.Equal(gotClass, classSourceRewriteOrder) {
		t.Fatalf("class-source order drifted:\n got=%v\nwant=%v", gotClass, classSourceRewriteOrder)
	}
	if !slices.Equal(gotMethod, methodSourceRewriteOrder) {
		t.Fatalf("method-source order drifted:\n got=%v\nwant=%v", gotMethod, methodSourceRewriteOrder)
	}
	if len(classSourceRewriteOrder) != 103 {
		t.Fatalf("class-source registry length = %d, want 103", len(classSourceRewriteOrder))
	}
	if len(methodSourceRewriteOrder) != 3 {
		t.Fatalf("method-source registry length = %d, want 3", len(methodSourceRewriteOrder))
	}
	classCount := map[string]int{}
	for _, id := range classSourceRewriteOrder {
		classCount[id]++
	}
	for key := range rewriteRuleSpecs {
		if key.Phase != "class_source" {
			t.Errorf("prefilter spec %q has unexpected phase %q", key.ID, key.Phase)
		}
		if classCount[key.ID] == 0 {
			t.Errorf("prefilter spec %q is absent from the ordered class-source registry", key.ID)
		}
	}
}

func TestRewriteSchedulerNecessaryConditionDNF(t *testing.T) {
	spec := rewriteRuleSpec{Alternatives: []rewriteNeedleSet{
		{"alpha", "beta"},
		{"gamma"},
	}}
	indexFor := func(source string) *rewriteApplicabilityIndex {
		return &rewriteApplicabilityIndex{source: source, presence: map[string]bool{}}
	}
	if !rewriteSpecApplicable(spec, indexFor("alpha beta")) {
		t.Fatal("first alternative with both needles was rejected")
	}
	if rewriteSpecApplicable(spec, indexFor("alpha only")) {
		t.Fatal("incomplete first alternative was accepted")
	}
	if !rewriteSpecApplicable(spec, indexFor("gamma")) {
		t.Fatal("second alternative was rejected")
	}
	if rewriteSpecApplicable(spec, indexFor("delta")) {
		t.Fatal("source with no alternative was accepted")
	}
	if !rewriteSpecApplicable(rewriteRuleSpec{}, indexFor("")) {
		t.Fatal("Always rule was rejected")
	}
	if !rewriteRuleApplicable("unregistered-rule", "class_source", "") {
		t.Fatal("unregistered rule must remain Always")
	}
	if !rewriteRuleApplicable("fixIntCmpBoolMaterializedLiteral", "method_source", "") {
		t.Fatal("phase mismatch must remain Always")
	}
}

type schedulerRuleFixture struct {
	id       string
	phase    string
	offFlag  string
	positive string
	nearMiss string
	rule     func(string) string
}

func schedulerRuleFixtures() []schedulerRuleFixture {
	return []schedulerRuleFixture{
		{
			id: "removeUnreachableSwitchBreak", phase: "class_source", offFlag: "JDEC_RM_UNREACHABLE_BREAK_OFF",
			rule: removeUnreachableSwitchBreak,
			positive: "class T {\n" +
				"\tvoid f() {\n" +
				"\t\tint var0 = 0;\n" +
				"\t\tswitch (var0) {\n" +
				"\t\t\tcase 1:\n" +
				"\t\t\t\tswitch (var0) {\n" +
				"\t\t\t\t\tdefault:\n" +
				"\t\t\t\t\t\treturn;\n" +
				"\t\t\t\t}\n" +
				"\t\t\t\tbreak;\n" +
				"\t\t}\n" +
				"\t}\n" +
				"}\n",
			nearMiss: "class T {\n" +
				"\tvoid f() {\n" +
				"\t\tint var0 = 0;\n" +
				"\t\tswitch (var0) {\n" +
				"\t\t\tcase 1:\n" +
				"\t\t\t\tswitch (var0) {\n" +
				"\t\t\t\t\tdefault:\n" +
				"\t\t\t\t\t\treturn;\n" +
				"\t\t\t\t}\n" +
				"\t\t\t\tcontinue;\n" +
				"\t\t}\n" +
				"\t}\n" +
				"}\n",
		},
		{
			id: "fixTryBreakReturn", phase: "class_source", offFlag: "JDEC_FIX_TRY_BREAK_RETURN_OFF",
			rule: fixTryBreakReturn,
			positive: "\t\tdo{\n" +
				"\t\t\ttry{\n" +
				"\t\t\t\tbreak;\n" +
				"\t\t\t}catch(Exception var0){\n" +
				"\t\t\t}\n" +
				"\t\t} while (true);\n" +
				"\t\treturn var0;\n",
			nearMiss: "\t\tdo{\n" +
				"\t\t\ttry{\n" +
				"\t\t\t\tbreak;\n" +
				"\t\t\t}catch(Exception var0){\n" +
				"\t\t\t}\n" +
				"\t\t} while (false);\n" +
				"\t\treturn var0;\n",
		},
		{
			id: "fixThrowTypeVarCast", phase: "class_source", offFlag: "JDEC_FIX_THROW_TYPEVAR_CAST_OFF",
			rule: fixThrowTypeVarCast,
			positive: "\tpublic <T extends Throwable> void f(Throwable var0) throws T {\n" +
				"\t\tthrow var0;\n" +
				"\t}\n",
			nearMiss: "\tpublic <T extends Throwable> void f(Throwable var0) {\n" +
				"\t\tthrow var0;\n" +
				"\t}\n",
		},
		{
			id: "fixLoopCatchThrow", phase: "class_source", offFlag: "JDEC_FIX_LOOP_CATCH_THROW_OFF",
			rule: fixLoopCatchThrow,
			positive: "\tvoid f() {\n" +
				"\t\tfor (;;) {\n" +
				"\t\t\ttry{\n" +
				"\t\t\t\tthis.g();\n" +
				"\t\t\t}catch(Exception var0){\n" +
				"\t\t\t\tthrow new RuntimeException(var0);\n" +
				"\t\t\t}\n" +
				"\t\t\tcontinue;\n" +
				"\t\t}\n" +
				"\t}\n",
			nearMiss: "\tvoid f() {\n" +
				"\t\tfor (;;) {\n" +
				"\t\t\ttry{\n" +
				"\t\t\t\tthis.g();\n" +
				"\t\t\t}catch(Exception var0){\n" +
				"\t\t\t\tthrow new RuntimeException(var0);\n" +
				"\t\t\t}\n" +
				"\t\t\treturn;\n" +
				"\t\t}\n" +
				"\t}\n",
		},
		{
			id: "wrapUncaughtThrowingCall", phase: "class_source", offFlag: "JDEC_WRAP_ALLOCATE_OFF",
			rule: wrapUncaughtThrowingCall,
			positive: "\tvoid f() {\n" +
				"\t\treturn UNSAFE.allocateInstance(var0);\n" +
				"\t}\n",
			nearMiss: "\tvoid f() {\n" +
				"\t\treturn UNSAFE.getInstance(var0);\n" +
				"\t}\n",
		},
		{
			id: "wrapFieldInitializerReflection", phase: "class_source", offFlag: "JDEC_WRAP_FIELD_INIT_OFF",
			rule:     wrapFieldInitializerReflection,
			positive: "\tprivate static final long valueOffset = UNSAFE.objectFieldOffset(Class.class.getDeclaredField(\"value\"));\n",
			nearMiss: "\tprivate static final long valueOffset = UNSAFE.objectFieldOffset(Class.class.findField(\"value\"));\n",
		},
		{
			id: "wrapReflectionCallInSwitchCase", phase: "class_source", offFlag: "JDEC_WRAP_REFLECTION_CASE_OFF",
			rule: wrapReflectionCallInSwitchCase,
			positive: "\tvoid f() {\n" +
				"\t\tswitch (var0) {\n" +
				"\t\t\tcase 1:\n" +
				"\t\t\t\treturn Class.class.getMethod(\"x\");\n" +
				"\t\t}\n" +
				"\t}\n",
			nearMiss: "\tvoid f() {\n" +
				"\t\tswitch (var0) {\n" +
				"\t\t\tcase 1:\n" +
				"\t\t\t\treturn Class.class.lookupMethod(\"x\");\n" +
				"\t\t}\n" +
				"\t}\n",
		},
		{
			id: "fixTryCatchExceptionPlacement", phase: "class_source", offFlag: "JDEC_FIX_TRYCATCH_OFF",
			rule: fixTryCatchExceptionPlacement,
			positive: "if (cond){\n" +
				"\tctor = type.getDeclaredConstructor();\n" +
				"}\n" +
				"try{\n" +
				"\ttry{\n" +
				"\t\tready();\n" +
				"\t}catch(Exception ex){\n" +
				"\t\tthrow new RuntimeException(ex);\n" +
				"\t}\n" +
				"}catch(Throwable t){ }\n",
			nearMiss: "if (cond){\n" +
				"\tctor = type.create();\n" +
				"}\n" +
				"try{\n" +
				"\ttry{\n" +
				"\t\tready();\n" +
				"\t}catch(Exception ex){\n" +
				"\t\tthrow new RuntimeException(ex);\n" +
				"\t}\n" +
				"}catch(Throwable t){ }\n",
		},
		{
			id: "fixImmutableBuilderWitness", phase: "class_source", offFlag: "JDEC_IMMUTABLE_BUILDER_WITNESS_OFF",
			rule:     fixImmutableBuilderWitness,
			positive: "\tprivate static final ImmutableMap<String, Foo> VALUE_PARSERS = ImmutableMap.builder().put(\"a\", new Foo()).build();\n",
			nearMiss: "\tprivate static final HashMap<String, Foo> VALUE_PARSERS = HashMap.builder().put(\"a\", new Foo()).build();\n",
		},
		{
			id: "fixIntCmpBoolMaterializedLiteral", phase: "class_source", offFlag: "JDEC_INT_CMP_BOOL_LIT_OFF",
			rule:     fixIntCmpBoolMaterializedLiteral,
			positive: "if ((!(var5)) && ((var7_1) == ((2) != (0)))){\n",
			nearMiss: "if ((!(var5)) && ((var7_1) == ((2) != (1)))){\n",
		},
		{
			id: "fixBoolUsedAsArithOperand", phase: "class_source", offFlag: "JDEC_BOOL_ARITH_OPERAND_OFF",
			rule:     fixBoolUsedAsArithOperand,
			positive: "\t\tboolean var8_1 = false;\n\t\tvar9_1 = (((141) + (var6_1)) + ((2) * (var7_1))) + ((4) * (var8_1));\n",
			nearMiss: "\t\tint var8_1 = 0;\n\t\tvar9_1 = (((141) + (var6_1)) + ((2) * (var7_1))) + ((4) * (var8_1));\n",
		},
		{
			id: "fixBooleanZeroLiteral", phase: "class_source", offFlag: "JDEC_BOOL_ZERO_LITERAL_OFF",
			rule:     fixBooleanZeroLiteral,
			positive: "boolean var10 = this.excludeField(var9,true);\n\t\t\t\tif (((var10) == (0)) && (!(var11))){\n\t\t\t\t\tvar10 = 0;\n\t\t\t\t\tvar10 = 1;\n",
			nearMiss: "int var10 = this.excludeField(var9,true);\n\t\t\t\tif (((var10) == (0)) && (!(var11))){\n\t\t\t\t\tvar10 = 0;\n\t\t\t\t\tvar10 = 1;\n",
		},
		{
			id: "fixClassDollarForwardRef", phase: "class_source", offFlag: "JDEC_CLASSDOLLAR_FORWARD_OFF",
			rule:     fixClassDollarForwardRef,
			positive: "\tstatic final String DATA_HOLDER_KEY = class$Foo.getName();\n\tstatic Class class$Foo;\n",
			nearMiss: "\tstatic final String DATA_HOLDER_KEY = class$Foo.getName();\n",
		},
		{
			id: "fixThrowInitCauseCast", phase: "class_source", offFlag: "JDEC_THROW_INITCAUSE_OFF",
			rule:     fixThrowInitCauseCast,
			positive: "\t\t\tthrow new NoClassDefFoundError().initCause((Throwable)(var1));\n",
			nearMiss: "\t\t\tthrow new NoClassDefFoundError().cause((Throwable)(var1));\n",
		},
		{
			id: "fixShadowFieldSuperAssign", phase: "class_source", offFlag: "JDEC_SHADOW_FIELD_SUPER_ASSIGN_OFF",
			rule:     fixShadowFieldSuperAssign,
			positive: "this.poolLock = this.poolLock;\nthis.leasedConnections = (Set) (this.leasedConnections);",
			nearMiss: "this.poolLock = poolLock;\n",
		},
		{
			id: "fixLeakedExceptionSentinel", phase: "class_source", offFlag: "JDEC_LEAKED_EXCEPTION_SENTINEL_OFF",
			rule:     fixLeakedExceptionSentinel,
			positive: "catch(Exception var0_1){\n\tObject var2 = Exception;\n\tthrow var0;\n}\n",
			nearMiss: "catch(Exception var0_1){\n\tObject var2 = null;\n\tthrow var0;\n}\n",
		},
		{
			id: "fixSpringMethodRefCasts", phase: "class_source", offFlag: "JDEC_SPRING_METHODREF_CAST_OFF",
			rule:     fixSpringMethodRefCasts,
			positive: "\t\treturn helper.synthesize(MergedAnnotation::isPresent);\n",
			nearMiss: "\t\treturn helper.synthesize(MergedAnnotation.isPresent);\n",
		},
	}
}

func TestRewriteSchedulerPositiveAndNearMissExactOutput(t *testing.T) {
	t.Setenv("JDEC_REWRITE_SCHEDULER_OFF", "")
	fixtures := schedulerRuleFixtures()
	if len(fixtures) != len(rewriteRuleSpecs) {
		t.Fatalf("fixture count = %d, prefilter spec count = %d; every prefilttered rule needs a fixture", len(fixtures), len(rewriteRuleSpecs))
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.id, func(t *testing.T) {
			t.Setenv(fixture.offFlag, "")
			direct := rewriteRuleExpectedOutput(fixture.id, fixture.rule, fixture.positive)
			if direct == fixture.positive {
				t.Fatalf("positive fixture did not exercise the rule:\n%s", fixture.positive)
			}
			on, stats := runScheduledRewrite(fixture.id, fixture.phase, fixture.positive, fixture.rule, false)
			if on != direct {
				t.Fatalf("scheduler output differs from direct rule output:\n got=%q\nwant=%q", on, direct)
			}
			if want := (rewriteSchedulerMetrics{Considered: 1, Candidates: 1, Invoked: 1}); stats != want {
				t.Fatalf("positive stats = %+v, want %+v", stats, want)
			}
			off, offStats := runScheduledRewrite(fixture.id, fixture.phase, fixture.positive, fixture.rule, true)
			if off != direct {
				t.Fatalf("kill-switch output differs from direct rule output:\n got=%q\nwant=%q", off, direct)
			}
			if want := (rewriteSchedulerMetrics{Considered: 1, Candidates: 1, Invoked: 1}); offStats != want {
				t.Fatalf("kill-switch stats = %+v, want %+v", offStats, want)
			}

			if rewriteRuleApplicable(fixture.id, fixture.phase, fixture.nearMiss) {
				t.Fatalf("near miss is still considered applicable:\n%s", fixture.nearMiss)
			}
			if nearDirect := rewriteRuleExpectedOutput(fixture.id, fixture.rule, fixture.nearMiss); nearDirect != fixture.nearMiss {
				t.Fatalf("necessary condition is unsound: rule changed a near miss:\n got=%q", nearDirect)
			}
			nearOn, nearStats := runScheduledRewrite(fixture.id, fixture.phase, fixture.nearMiss, fixture.rule, false)
			if nearOn != fixture.nearMiss {
				t.Fatalf("scheduler changed a near miss:\n got=%q", nearOn)
			}
			if want := (rewriteSchedulerMetrics{Considered: 1, Skipped: 1}); nearStats != want {
				t.Fatalf("near-miss stats = %+v, want %+v", nearStats, want)
			}
			nearOff, _ := runScheduledRewrite(fixture.id, fixture.phase, fixture.nearMiss, fixture.rule, true)
			if nearOff != fixture.nearMiss {
				t.Fatalf("kill-switch rule changed a near miss:\n got=%q", nearOff)
			}
		})
	}
}

func runScheduledRewrite(id, phase, source string, rule func(string) string, disabled bool) (string, rewriteSchedulerMetrics) {
	d := &ClassObjectDumper{
		options:                  DecompileOptions{Mode: Compatibility},
		rewriteSchedulerDisabled: disabled,
	}
	out := d.sourceRewrite(id, phase, source, rule)
	return out, d.rewriteSchedulerStats
}

func schedulerTestHash(source string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
}

func TestRewriteSchedulerPreservesRecordsDiagnosticsAndHashes(t *testing.T) {
	t.Setenv("JDEC_REWRITE_SCHEDULER_OFF", "")
	t.Setenv("JDEC_BOOL_ZERO_LITERAL_OFF", "")
	t.Setenv("JDEC_THROW_INITCAUSE_OFF", "")
	initial := "class T {\n" +
		"\tvoid f() {\n" +
		"\t\tboolean var0 = false;\n" +
		"\t\tvar0 = 0;\n" +
		"\t\tthrow new NoClassDefFoundError().initCause((Throwable)(var0));\n" +
		"\t}\n" +
		"}\n"

	run := func(disabled bool) (string, *DecompileResult, rewriteSchedulerMetrics) {
		report := &DecompileResult{}
		d := &ClassObjectDumper{
			options:                  DecompileOptions{Mode: Compatibility},
			report:                   report,
			rewriteSchedulerDisabled: disabled,
		}
		source := initial
		source = d.sourceRewrite("fixBooleanZeroLiteral", "class_source", source, fixBooleanZeroLiteral)
		source = d.sourceRewrite("fixThrowInitCauseCast", "class_source", source, fixThrowInitCauseCast)
		source = d.sourceRewrite("scheduler-always-rule", "class_source", source, func(src string) string {
			return strings.ReplaceAll(src, "class T", "class U")
		})
		// This registered rule has no exact literal trigger in the sequence.
		source = d.sourceRewrite("fixIntCmpBoolMaterializedLiteral", "class_source", source, fixIntCmpBoolMaterializedLiteral)
		return source, report, d.rewriteSchedulerStats
	}

	onSource, onReport, onStats := run(false)
	offSource, offReport, offStats := run(true)
	if onSource != offSource {
		t.Fatalf("scheduler changed final source:\n on=%q\noff=%q", onSource, offSource)
	}
	if !reflect.DeepEqual(onReport.RulesApplied, offReport.RulesApplied) {
		t.Fatalf("rewrite records diverged:\n on=%+v\noff=%+v", onReport.RulesApplied, offReport.RulesApplied)
	}
	if !reflect.DeepEqual(onReport.Diagnostics, offReport.Diagnostics) {
		t.Fatalf("diagnostics diverged:\n on=%+v\noff=%+v", onReport.Diagnostics, offReport.Diagnostics)
	}
	if onStats.Considered != 4 || offStats.Considered != 4 {
		t.Fatalf("considered counts = on:%d off:%d, want 4/4", onStats.Considered, offStats.Considered)
	}
	if onStats.Skipped != 1 {
		t.Fatalf("expected exactly one prefiltered rule: %+v", onStats)
	}
	if offStats.Skipped != 0 {
		t.Fatalf("kill switch skipped a rule: %+v", offStats)
	}
	if onStats.Candidates+onStats.Skipped != onStats.Considered {
		t.Fatalf("candidate/skip accounting = %+v", onStats)
	}

	s0 := initial
	s1 := fixBooleanZeroLiteral(s0)
	s2 := fixThrowInitCauseCast(s1)
	s3 := strings.ReplaceAll(s2, "class T", "class U")
	want := []RewriteRecord{
		{Rule: "fixBooleanZeroLiteral", Phase: "class_source", BeforeHash: schedulerTestHash(s0), AfterHash: schedulerTestHash(s1)},
		{Rule: "fixThrowInitCauseCast", Phase: "class_source", BeforeHash: schedulerTestHash(s1), AfterHash: schedulerTestHash(s2)},
		{Rule: "scheduler-always-rule", Phase: "class_source", BeforeHash: schedulerTestHash(s2), AfterHash: schedulerTestHash(s3)},
	}
	if !reflect.DeepEqual(onReport.RulesApplied, want) {
		t.Fatalf("hashes/records differ from historical values:\n got=%+v\nwant=%+v", onReport.RulesApplied, want)
	}
	if len(onReport.Diagnostics) != 3 {
		t.Fatalf("diagnostic count = %d, want 3: %+v", len(onReport.Diagnostics), onReport.Diagnostics)
	}
}

func TestRewriteSchedulerHashesEachStateOnce(t *testing.T) {
	t.Setenv("JDEC_REWRITE_SCHEDULER_OFF", "")
	report := &DecompileResult{}
	d := &ClassObjectDumper{
		options: DecompileOptions{Mode: Compatibility},
		report:  report,
	}
	first := d.sourceRewrite("scheduler-hash-a", "class_source", "A", func(string) string { return "B" })
	if first != "B" {
		t.Fatalf("first rewrite = %q", first)
	}
	noop := d.sourceRewrite("scheduler-hash-noop", "class_source", first, func(source string) string { return source })
	if noop != first {
		t.Fatalf("no-op rewrite changed source: %q", noop)
	}
	cycle := d.sourceRewrite("scheduler-hash-b", "class_source", first, func(string) string { return "A" })
	if cycle != first || len(report.RulesApplied) != 1 {
		t.Fatalf("cycle was not rejected: %q %+v", cycle, report)
	}
	if got := d.rewriteSchedulerStats.HashComputations; got != 2 {
		t.Fatalf("SHA-256 computations = %d, want 2 (A and B each once)", got)
	}
	if got := len(d.rewriteHashes); got != 2 {
		t.Fatalf("hash memo entries = %d, want 2", got)
	}
	if last := report.Diagnostics[len(report.Diagnostics)-1]; last.Code != "rewrite_cycle" {
		t.Fatalf("cycle diagnostic = %+v", last)
	}
}

func TestRewriteSchedulerEnvironmentKillSwitch(t *testing.T) {
	const source = "void f() {\n\tint var1 = 0;\n}\n"
	run := func(env map[string]string) rewriteSchedulerMetrics {
		t.Helper()
		var stats rewriteSchedulerMetrics
		if err := jdecenv.Run(env, func() error {
			d := &ClassObjectDumper{options: DecompileOptions{Mode: Compatibility}}
			_ = d.sourceRewrite("fixIntCmpBoolMaterializedLiteral", "class_source", source, fixIntCmpBoolMaterializedLiteral)
			stats = d.rewriteSchedulerStats
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return stats
	}
	if got := run(map[string]string{}); got != (rewriteSchedulerMetrics{Considered: 1, Skipped: 1}) {
		t.Fatalf("default scheduler stats = %+v, want one skip", got)
	}
	if got := run(map[string]string{"JDEC_REWRITE_SCHEDULER_OFF": "1"}); got != (rewriteSchedulerMetrics{Considered: 1, Candidates: 1, Invoked: 1}) {
		t.Fatalf("killed scheduler stats = %+v, want unconditional invocation", got)
	}
}

func TestRewriteSchedulerRebuildsApplicabilityAfterAcceptedRewrite(t *testing.T) {
	t.Setenv("JDEC_REWRITE_SCHEDULER_OFF", "")
	t.Setenv("JDEC_INT_CMP_BOOL_LIT_OFF", "")
	source := "void f() {\n\tint var1 = 0;\n}\n"
	d := &ClassObjectDumper{options: DecompileOptions{Mode: Compatibility}}

	// Prime the index with "!= (0)) absent".
	skipped := d.sourceRewrite("fixIntCmpBoolMaterializedLiteral", "class_source", source, fixIntCmpBoolMaterializedLiteral)
	if skipped != source {
		t.Fatalf("rule changed a source with no literal trigger: %q", skipped)
	}
	if d.rewriteSchedulerStats.Skipped != 1 || d.rewriteSchedulerStats.Invoked != 0 {
		t.Fatalf("expected one skip before mutation: %+v", d.rewriteSchedulerStats)
	}

	// An accepted rewrite must invalidate the old index.
	mutated := d.sourceRewrite("scheduler-inject-literal", "class_source", source, func(text string) string {
		return text + "if ((var1) == ((2) != (0))){\n}\n"
	})
	if mutated == source {
		t.Fatal("injection rewrite did not change source")
	}
	changed := d.sourceRewrite("fixIntCmpBoolMaterializedLiteral", "class_source", mutated, fixIntCmpBoolMaterializedLiteral)
	if changed == mutated {
		t.Fatal("stale applicability index skipped a now-applicable rule")
	}
	if !strings.Contains(changed, "(var1) == (2)") {
		t.Fatalf("rewrite did not collapse the materialized literal:\n%s", changed)
	}
	if d.rewriteSchedulerStats.Invoked != 2 {
		t.Fatalf("invocation count = %d, want 2 (injection + materialized literal)", d.rewriteSchedulerStats.Invoked)
	}
}

func TestRewriteSchedulerNearMissPipelineMetrics(t *testing.T) {
	t.Setenv("JDEC_REWRITE_SCHEDULER_OFF", "")
	fixtures := schedulerRuleFixtures()
	source := syntheticSchedulerNearMissSource()
	d := &ClassObjectDumper{options: DecompileOptions{Mode: Compatibility}}
	for _, fixture := range fixtures {
		t.Setenv(fixture.offFlag, "")
		if got := d.sourceRewrite(fixture.id, fixture.phase, source, fixture.rule); got != source {
			t.Fatalf("%s changed a near-miss pipeline source", fixture.id)
		}
	}
	want := rewriteSchedulerMetrics{Considered: len(fixtures), Skipped: len(fixtures)}
	if d.rewriteSchedulerStats != want {
		t.Fatalf("near-miss metrics = %+v, want %+v", d.rewriteSchedulerStats, want)
	}
}

func syntheticSchedulerNearMissSource() string {
	unit := "class Synthetic {\n" +
		"\tint field = 0;\n" +
		"\tvoid method(int var0) {\n" +
		"\t\tint var1 = var0;\n" +
		"\t\tvar1 = var1 + 1;\n" +
		"\t\tif ((var1) > (0)){\n" +
		"\t\t\tvar1 = var1 - 1;\n" +
		"\t\t}\n" +
		"\t}\n" +
		"}\n"
	return strings.Repeat(unit, 512)
}

func TestRewriteSchedulerDecompileEquivalence(t *testing.T) {
	paths := []string{
		"testdata/regression/SwitchThrowingDefaultBreakSeed.class",
		"testdata/regression/TryBreakReturnSeed.class",
		"testdata/regression/ThrowTypeVarCastSeed.class",
		"testdata/GenericMessageEndpoint.class",
		"testdata/regression/ConnPoolByRoute.class",
		"testdata/regression/XStream.class",
		"testdata/regression/CodecEncoding.class",
		"testdata/regression/TwoDigitYear.class",
		"testdata/regression/ExceptionSentinelSeed.class",
		"testdata/regression/FactoryReturnRawBridgeSeed.class",
		"testdata/regression/SpringDataBufferEncoder.class",
		"testdata/regression/Transmitter.class",
		"testdata/regression/ExchangeFinder.class",
		"testdata/regression/RealConnection.class",
		"testdata/regression/Http2Connection.class",
		"testdata/regression/Http2Stream.class",
		"testdata/regression/DiskLruCache$3.class",
		"testdata/regression/PublicSuffixDatabase.class",
	}
	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			on, onErr := DecompileWithOptions(raw, DecompileOptions{
				Mode:        Compatibility,
				EnvSnapshot: map[string]string{},
			})
			off, offErr := DecompileWithOptions(raw, DecompileOptions{
				Mode:        Compatibility,
				EnvSnapshot: map[string]string{"JDEC_REWRITE_SCHEDULER_OFF": "1"},
			})
			if (onErr == nil) != (offErr == nil) {
				t.Fatalf("error presence diverged: on=%v off=%v", onErr, offErr)
			}
			if onErr != nil && onErr.Error() != offErr.Error() {
				t.Fatalf("error text diverged: on=%v off=%v", onErr, offErr)
			}
			if on.Source != off.Source {
				t.Fatalf("source diverged:\n on=%q\noff=%q", on.Source, off.Source)
			}
			if on.Status != off.Status {
				t.Fatalf("status diverged: on=%q off=%q", on.Status, off.Status)
			}
			if !reflect.DeepEqual(on.RulesApplied, off.RulesApplied) {
				t.Fatalf("rewrite records diverged:\n on=%+v\noff=%+v", on.RulesApplied, off.RulesApplied)
			}
			if !reflect.DeepEqual(on.Diagnostics, off.Diagnostics) {
				t.Fatalf("diagnostics diverged:\n on=%+v\noff=%+v", on.Diagnostics, off.Diagnostics)
			}
			if !reflect.DeepEqual(on.StubMethods, off.StubMethods) {
				t.Fatalf("stub methods diverged:\n on=%+v\noff=%+v", on.StubMethods, off.StubMethods)
			}
		})
	}
}

func TestRewriteSchedulerSkipsInapplicableRulesOnRealClass(t *testing.T) {
	t.Setenv("JDEC_REWRITE_SCHEDULER_OFF", "")
	raw, err := os.ReadFile("testdata/regression/SwitchThrowingDefaultBreakSeed.class")
	if err != nil {
		t.Fatal(err)
	}
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	d := NewClassObjectDumper(obj)
	d.options = DecompileOptions{Mode: Compatibility, TargetSourceVersion: 8, EnvSnapshot: map[string]string{}}
	d.report = &DecompileResult{}
	source, err := d.DumpClass()
	if err != nil {
		t.Fatal(err)
	}
	if source == "" {
		t.Fatal("empty class source")
	}
	stats := d.rewriteSchedulerStats
	t.Logf("rewrite scheduler stats for SwitchThrowingDefaultBreakSeed: %+v", stats)
	if stats.Considered < len(classSourceRewriteOrder) {
		t.Fatalf("considered = %d, want at least %d", stats.Considered, len(classSourceRewriteOrder))
	}
	if stats.Skipped == 0 {
		t.Fatalf("no rule was prefilttered on a class-source pipeline: %+v", stats)
	}
	if stats.Candidates+stats.Skipped != stats.Considered {
		t.Fatalf("candidate/skip accounting = %+v", stats)
	}
	if stats.Invoked > stats.Candidates {
		t.Fatalf("invoked exceeds candidates: %+v", stats)
	}
}

// BenchmarkDecompileSchedulerPipeline measures the whole Compatibility pass on
// a real class. Both variants produce the same source and rewrite records.
func BenchmarkDecompileSchedulerPipeline(b *testing.B) {
	raw, err := os.ReadFile("testdata/regression/RealConnection.class")
	if err != nil {
		b.Fatal(err)
	}
	for _, disabled := range []bool{true, false} {
		name := "scheduler_on"
		env := map[string]string{}
		if disabled {
			name = "scheduler_off"
			env = map[string]string{"JDEC_REWRITE_SCHEDULER_OFF": "1"}
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if _, err := DecompileWithOptions(raw, DecompileOptions{
					Mode:        Compatibility,
					EnvSnapshot: env,
				}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkRewriteSchedulerNearMissPipeline(b *testing.B) {
	for _, fixture := range schedulerRuleFixtures() {
		b.Setenv(fixture.offFlag, "")
	}
	b.Setenv("JDEC_REWRITE_SCHEDULER_OFF", "")
	source := syntheticSchedulerNearMissSource()
	fixtures := schedulerRuleFixtures()
	for _, disabled := range []bool{true, false} {
		name := "scheduler_on"
		if disabled {
			name = "scheduler_off"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			var stats rewriteSchedulerMetrics
			for i := 0; i < b.N; i++ {
				d := &ClassObjectDumper{
					options:                  DecompileOptions{Mode: Compatibility},
					rewriteSchedulerDisabled: disabled,
				}
				for _, fixture := range fixtures {
					_ = d.sourceRewrite(fixture.id, fixture.phase, source, fixture.rule)
				}
				stats = d.rewriteSchedulerStats
			}
			b.ReportMetric(float64(stats.Considered), "considered")
			b.ReportMetric(float64(stats.Candidates), "candidates")
			b.ReportMetric(float64(stats.Invoked), "invoked")
			b.ReportMetric(float64(stats.Skipped), "skipped")
		})
	}
}
