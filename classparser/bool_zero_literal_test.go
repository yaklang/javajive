package javaclassparser

import (
	"regexp"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func TestBooleanZeroLiteralSnippet(t *testing.T) {
	in := "boolean var10 = this.excludeField(var9,true);\n\t\t\t\tif (((var10) == (0)) && (!(var11))){\n\t\t\t\t\tvar10 = 0;\n\t\t\t\t\tvar10 = 1;\n"
	out := fixBooleanZeroLiteral(in)
	if !strings.Contains(out, "(var10) == (false)") {
		t.Fatalf("== 0 not rewritten:\n%s", out)
	}
	if !strings.Contains(out, "var10 = false;") {
		t.Fatalf("= 0 not rewritten:\n%s", out)
	}
	if !strings.Contains(out, "var10 = true;") {
		t.Fatalf("= 1 not rewritten:\n%s", out)
	}
}

func TestBooleanExprCmpZeroSnippet(t *testing.T) {
	in := "if (((Entities.isBaseNamedEntity(var4_1)) || ((Entities.isNamedEntity(var4_1)) && (var5_1))) == (0)){\n"
	out := fixBooleanExprCmpZero(in)
	if !strings.Contains(out, ") == (false)){") {
		t.Fatalf("expr == 0 not rewritten:\n%s", out)
	}
}

func TestBooleanExprCmpZeroNeSnippet(t *testing.T) {
	in := "if ((((this.ch) == (45)) || ((this.ch) == (43))) != (0)){\n"
	out := fixBooleanExprCmpZero(in)
	if !strings.Contains(out, ") != (false)){") {
		t.Fatalf("expr != 0 not rewritten:\n%s", out)
	}
}

func TestIntCmpBoolMaterializedLiteralSnippet(t *testing.T) {
	in := "if ((!(var5)) && ((var7_1) == ((2) != (0)))){\n"
	out := fixIntCmpBoolMaterializedLiteral(in)
	if !strings.Contains(out, "(var7_1) == (2)") {
		t.Fatalf("literal-2 wrap not collapsed:\n%s", out)
	}
	if strings.Contains(out, "(2) != (0)") {
		t.Fatalf("still wrapped:\n%s", out)
	}
}

func TestJodaTwoDigitYearIntCmpIsLoadBearing(t *testing.T) {
	raw, code, _ := reviewedFixtureMethod(t, "testdata/regression/TwoDigitYear.class", "parseInto", "")
	assertReviewedOpcode(t, code, 104, core.OP_IINC, 7, 1)
	assertReviewedOpcode(t, code, 158, core.OP_ICONST_2)
	assertReviewedOpcode(t, code, 159, core.OP_IF_ICMPEQ)
	assertReviewedSources(t, raw, "JDEC_INT_CMP_BOOL_LIT_OFF", func(source string) {
		body := reviewedSourceMethod(t, source, `public int parseInto\(`)
		comparison := requireReviewedPattern(t, body, `\((\w+)\)\s*==\s*\(2\)`)
		requireReviewedPattern(t, body, `int\s+`+regexp.QuoteMeta(comparison[1])+`\s*=`)
		requireReviewedPattern(t, body, regexp.QuoteMeta(comparison[1])+`\+\+`)
		if strings.Contains(body, "(2) != (0)") {
			t.Fatalf("numeric equality became a boolean-materialized literal:\n%s", body)
		}
	})
}

func TestJsoupTokeniserBoolExprCmpZeroIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/Tokeniser.class", "JDEC_BOOL_EXPR_CMP_ZERO_OFF",
		") == (false)){",
		"((Entities.isBaseNamedEntity(var4_1)) || ((Entities.isNamedEntity(var4_1)) && (var5_1))) == (0)){")
}

func TestAsmFrameBoolAssignOneIsLoadBearing(t *testing.T) {
	assertKillSwitchDecompile(t, "testdata/regression/AsmFrame.class", "JDEC_BOOL_ZERO_LITERAL_OFF",
		"var4 = true;",
		"var4 = 1;")
}
