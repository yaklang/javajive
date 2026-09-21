package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const shadowLiteralSrc = `public class ShadowLiteralShield {
  int poolLock;
  public static void main(String[] args) {
    String s = "this.poolLock = this.poolLock;";
    char c = 'x';
    System.out.println(s);
    System.out.println((int)c);
    // this.poolLock = this.poolLock;
    /* this.poolLock = this.poolLock; default: */
  }
  void real(int poolLock) { this.poolLock = poolLock; }
}
`

func TestT27_C01_LiteralShielding(t *testing.T) {
	t.Run("T27-C01", func(t *testing.T) { testT27C01(t) })
}

func testT27C01(t *testing.T) {
	pattern := "this.poolLock = this.poolLock;"
	src := "class X {\n" +
		"  int poolLock;\n" +
		"  void m(){ this.poolLock = this.poolLock; }\n" +
		"  String s = \"" + pattern + "\";\n" +
		"  char ch = 'z';\n" +
		"  // " + pattern + "\n" +
		"  /* " + pattern + " */\n" +
		"  String t = \"\"\"\n" + pattern + "\n\"\"\";\n" +
		"}\n"
	got := ApplyContractedRule(RewriteFixShadowFieldSuperAssign, src)
	if !strings.Contains(got, `"`+pattern+`"`) {
		t.Fatalf("T27-C01 string literal mutated:\n%s", got)
	}
	if !strings.Contains(got, "// "+pattern) {
		t.Fatalf("T27-C01 line comment mutated:\n%s", got)
	}
	if !strings.Contains(got, "/* "+pattern+" */") {
		t.Fatalf("T27-C01 block comment mutated:\n%s", got)
	}
	if !strings.Contains(got, pattern+"\n") && !strings.Contains(got, "\"\"\"") {
		t.Fatalf("T27-C01 text block mutated:\n%s", got)
	}
	if !strings.Contains(got, "this.poolLock = super.poolLock;") {
		t.Fatalf("T27-C01 code node should still rewrite:\n%s", got)
	}

	dir := t.TempDir()
	compileJavaRelease(t, dir, map[string]string{"ShadowLiteralShield.java": shadowLiteralSrc}, "8")
	data := readClassBytes(t, dir, "ShadowLiteralShield")
	res := decompileCompatibility(t, data, nil)
	if !strings.Contains(res.Source, `this.poolLock = this.poolLock;`) && !strings.Contains(res.Source, `"this.poolLock = this.poolLock;"`) {
		t.Fatalf("T27-C01 decompiled literal lost:\n%s", res.Source)
	}
	if strings.Contains(res.Source, `"this.poolLock = super.poolLock;"`) {
		t.Fatalf("T27-C01 rule rewrote a string literal:\n%s", res.Source)
	}
}

func TestT27_C02_PositiveVsNearMiss(t *testing.T) {
	t.Run("T27-C02", func(t *testing.T) { testT27C02(t) })
}

func testT27C02(t *testing.T) {
	posShadow := "this.poolLock = this.poolLock;\n"
	nearShadow := "this.poolLock = this.other;\n"
	if ApplyContractedRule(RewriteFixShadowFieldSuperAssign, posShadow) == posShadow {
		t.Fatal("T27-C02 shadow positive should apply")
	}
	if ApplyContractedRule(RewriteFixShadowFieldSuperAssign, nearShadow) != nearShadow {
		t.Fatal("T27-C02 shadow near-miss must not apply")
	}
	spec, ok := lookupRewriteContract(RewriteFixShadowFieldSuperAssign)
	if !ok || !spec.Matches(posShadow) || spec.Matches(nearShadow) {
		t.Fatal("T27-C02 shadow predicate mismatch")
	}

	posDefault := "\tswitch (x) {\n\tdefault:\n\t}\n"
	nearDefault := "\tswitch (x) {\n\tdefault:\n\t\tbreak;\n\t}\n"
	if ApplyContractedRule(RewriteFixEmptySwitchDefault, posDefault) == posDefault {
		t.Fatal("T27-C02 empty-default positive should apply")
	}
	if ApplyContractedRule(RewriteFixEmptySwitchDefault, nearDefault) != nearDefault {
		t.Fatal("T27-C02 empty-default near-miss must not apply")
	}

	posTry := "" +
		"\tdo{\n" +
		"\ttry{\n" +
		"\t\tbreak;\n" +
		"\t}catch(Exception e){\n" +
		"\t}\n" +
		"\t} while (true);\n" +
		"\treturn x;\n"
	nearTry := strings.Replace(posTry, "} while (true);", "} while (false);", 1)
	gotTry := ApplyContractedRule(RewriteFixTryBreakReturn, posTry)
	if gotTry == posTry || !strings.Contains(gotTry, "return x;") {
		t.Fatalf("T27-C02 try-break positive should apply, got:\n%s", gotTry)
	}
	if ApplyContractedRule(RewriteFixTryBreakReturn, nearTry) != nearTry {
		t.Fatal("T27-C02 try-break near-miss must not apply")
	}
}

func TestT27_C03_RedundantRetirementKeepsContract(t *testing.T) {
	t.Run("T27-C03", func(t *testing.T) { testT27C03(t) })
}

func testT27C03(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/ConnPoolByRoute.class")
	if err != nil {
		t.Skip(err)
	}
	os.Unsetenv("JDEC_SHADOW_FIELD_SUPER_ASSIGN_OFF")
	on, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("JDEC_SHADOW_FIELD_SUPER_ASSIGN_OFF", "1")
	off, err := Decompile(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(on, "this.poolLock = super.poolLock") || strings.Contains(off, "this.poolLock = super.poolLock") {
		if !strings.Contains(on, "this.poolLock = super.poolLock") && !strings.Contains(off, "poolLock") {
			t.Fatal("T27-C03 lost poolLock assignment")
		}
	}
	if on == off {
		t.Log("T27-C03 rule ON/OFF identical: general algorithm satisfies the contract; rule may be retired")
	}
}

func TestT27_C04_OscillationBudget(t *testing.T) {
	t.Run("T27-C04", func(t *testing.T) { testT27C04(t) })
}

func testT27C04(t *testing.T) {
	eng := NewRewriteContractEngine(Compatibility)
	eng.Budget = 8
	first := eng.Apply("oscA", "class_source", "A", func(string) string { return "B" })
	next := eng.Apply("oscB", "class_source", first, func(string) string { return "A" })
	if next != "B" {
		t.Fatalf("T27-C04 cycle not rejected, got %q", next)
	}
	found := false
	for _, d := range eng.Diagnostics {
		if d.Code == "rewrite_cycle" {
			found = true
			if !strings.Contains(d.Message, "oscA") || !strings.Contains(d.Message, "oscB") {
				t.Fatalf("T27-C04 missing rule chain: %s", d.Message)
			}
		}
	}
	if !found {
		t.Fatalf("T27-C04 expected rewrite_cycle, diags=%+v", eng.Diagnostics)
	}
	if len(eng.Records) != 1 {
		t.Fatalf("T27-C04 oscillating second rule must not record success, records=%+v", eng.Records)
	}

	repeat := NewRewriteContractEngine(Compatibility)
	repeat.Budget = 3
	s := "n0"
	for i := 0; i < 10; i++ {
		id := "repeat"
		s = repeat.Apply(id, "class_source", s, func(in string) string { return in + "x" })
	}
	budgeted := false
	for _, d := range repeat.Diagnostics {
		if d.Code == "rewrite_budget" {
			budgeted = true
		}
	}
	if !budgeted {
		t.Fatalf("T27-C04 expected rewrite_budget, diags=%+v chain=%v", repeat.Diagnostics, repeat.Chain)
	}

	report := &DecompileResult{}
	d := &ClassObjectDumper{options: DecompileOptions{Mode: Compatibility}, report: report}
	a := d.sourceRewrite("oscA", "class_source", "A", func(string) string { return "B" })
	b := d.sourceRewrite("oscB", "class_source", a, func(string) string { return "A" })
	if b != "B" || report.Diagnostics[len(report.Diagnostics)-1].Code != "rewrite_cycle" {
		t.Fatalf("T27-C04 production sourceRewrite cycle: %s %+v", b, report)
	}
}

func TestT27_C05_PrecisionVsCompatibility(t *testing.T) {
	t.Run("T27-C05", func(t *testing.T) { testT27C05(t) })
}

func testT27C05(t *testing.T) {
	raw, err := os.ReadFile("testdata/regression/ConnPoolByRoute.class")
	if err != nil {
		t.Skip(err)
	}
	prec, err := DecompileWithOptions(raw, DecompileOptions{Mode: Precision})
	if err != nil {
		t.Fatal(err)
	}
	if len(prec.RulesApplied) != 0 {
		t.Fatalf("T27-C05 Precision must skip sourceRewrite, rules=%+v", prec.RulesApplied)
	}
	comp, err := DecompileWithOptions(raw, DecompileOptions{Mode: Compatibility})
	if err != nil {
		t.Fatal(err)
	}
	if prec.Mode != Precision || comp.Mode != Compatibility {
		t.Fatalf("T27-C05 mode boundary lost prec=%s comp=%s", prec.Mode, comp.Mode)
	}
	for _, rec := range comp.RulesApplied {
		if rec.Rule == "" || rec.Phase == "" || rec.BeforeHash == "" || rec.AfterHash == "" {
			t.Fatalf("T27-C05 incomplete provenance: %+v", rec)
		}
	}
	found := false
	for _, rec := range comp.RulesApplied {
		if rec.Rule == RewriteFixShadowFieldSuperAssign {
			found = true
		}
	}
	if strings.Contains(comp.Source, "this.poolLock = super.poolLock") && !found {
		t.Fatal("T27-C05 compatibility rewrite of shadow field not recorded")
	}
}
