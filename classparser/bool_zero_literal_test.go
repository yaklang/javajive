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
	for _, input := range []string{
		"if (((Entities.isBaseNamedEntity(name)) || (named && suffix)) == (0)){\n",
		"if ((((a) || (b)) ? (1) : (0)) == (0)){\n",
		"if (((a && b) ? (2) : (3)) == (0)){\n",
	} {
		if got := fixBooleanExprCmpZero(input); got != input {
			t.Fatalf("source syntax cannot prove the comparison operand type: %s", got)
		}
	}
}

func TestBooleanExprCmpZeroNeSnippet(t *testing.T) {
	for _, input := range []string{
		"if ((((this.ch) == (45)) || ((this.ch) == (43))) != (0)){\n",
		"if ((((a) || (b)) ? (1) : (0)) != (0)){\n",
		"if (((a && b) ? (-2) : (-1)) != (0)){\n",
	} {
		if got := fixBooleanExprCmpZero(input); got != input {
			t.Fatalf("a logical child is not a boolean enclosing operand: %s", got)
		}
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
	const path = "testdata/regression/Tokeniser.class"
	const desc = "(Ljava/lang/Character;Z)[I"
	raw, code, _ := reviewedFixtureMethod(t, path, "consumeCharacterReference", desc)
	assertReviewedTypeVarInvoke(t, path, "consumeCharacterReference", desc, 250, core.OP_INVOKESTATIC, "org/jsoup/nodes/Entities", "isBaseNamedEntity", "(Ljava/lang/String;)Z")
	assertReviewedTypeVarInvoke(t, path, "consumeCharacterReference", desc, 258, core.OP_INVOKESTATIC, "org/jsoup/nodes/Entities", "isNamedEntity", "(Ljava/lang/String;)Z")
	for _, op := range []struct {
		pc     uint16
		opcode int
		data   []byte
	}{
		{253, core.OP_IFNE, []byte{0, 16}}, {261, core.OP_IFEQ, []byte{0, 12}}, {264, core.OP_ILOAD, []byte{5}}, {266, core.OP_IFEQ, []byte{0, 7}},
		{269, core.OP_ICONST_1, nil}, {273, core.OP_ICONST_0, nil}, {274, core.OP_ISTORE, []byte{6}}, {276, core.OP_ILOAD, []byte{6}}, {278, core.OP_IFNE, []byte{0, 35}},
	} {
		assertReviewedOpcode(t, code, op.pc, op.opcode, op.data...)
	}
	// Original metadata proves the short-circuit expression is canonical 0/1.
	// TestAdversarialThreePredicateCanonicalWordTwoConsumersRoundTrip and
	// TestAdversarialNoncanonicalBooleanWordsRoundTrip independently exercise
	// ordered/effectful predicates and distinguish word consumers from Z sinks.
	assertReviewedSources(t, raw, "JDEC_BOOL_EXPR_CMP_ZERO_OFF", func(source string) {
		body := reviewedSourceMethod(t, source, `int\[\] consumeCharacterReference\(`)
		name := requireReviewedPattern(t, body, `String\s+(\w+)\s*=\s*this\.reader\.consumeLetterThenDigitSequence\(\);`)[1]
		suffix := requireReviewedPattern(t, body, `boolean\s+(\w+)\s*=\s*this\.reader\.matches\(\(char\)\(59\)\);`)[1]
		predicate := "Entities.isBaseNamedEntity(" + name + ")||Entities.isNamedEntity(" + name + ")&&" + suffix
		start, end, ok := reviewedCanonicalPredicateIf(body, predicate, false)
		if !ok {
			t.Fatalf("canonical predicate has an invalid consumer or changed evaluation order:\n%s", body)
		}
		if strings.Count(body, "Entities.isBaseNamedEntity("+name+")") != 1 || strings.Count(body, "Entities.isNamedEntity("+name+")") != 1 {
			t.Fatal("entity predicate evaluation duplicated")
		}
		tail := body[end:]
		if strings.Index(tail, "this.reader.rewindToMark();") < 0 || start >= end {
			t.Fatal("false predicate lost original rewind continuation")
		}
	})
}

// Inspect original bytecode only. Its local is an int-category OR accumulator;
// source Boolean views belong at the Z call/result sinks, not at every store.
// The executable array/alias/exception oracle is
// TestAdversarialBooleanArrayMergeCarrierRoundTrip.
func TestAsmFrameBoolAssignOneIsLoadBearing(t *testing.T) {
	const path = "testdata/regression/AsmFrame.class"
	const descriptor = "(Lorg/objectweb/asm/SymbolTable;Lorg/objectweb/asm/Frame;I)Z"
	raw, code, _ := reviewedFixtureMethod(t, path, "merge", descriptor)
	for _, op := range []struct {
		pc      uint16
		opcode  int
		operand []byte
	}{
		{0, core.OP_ICONST_0, nil}, {1, core.OP_ISTORE, []byte{4}},
		{32, core.OP_ICONST_1, nil}, {33, core.OP_ISTORE, []byte{4}},
		{140, core.OP_IOR, nil}, {141, core.OP_ISTORE, []byte{4}},
		{182, core.OP_IOR, nil}, {183, core.OP_ISTORE, []byte{4}},
		{205, core.OP_ICONST_1, nil}, {206, core.OP_ISTORE, []byte{4}},
		{220, core.OP_IOR, nil}, {221, core.OP_ISTORE, []byte{4}},
		{223, core.OP_ILOAD, []byte{4}}, {225, core.OP_IRETURN, nil},
		{258, core.OP_ICONST_1, nil}, {259, core.OP_ISTORE, []byte{4}},
		{310, core.OP_IOR, nil}, {311, core.OP_ISTORE, []byte{4}},
		{383, core.OP_IOR, nil}, {384, core.OP_ISTORE, []byte{4}},
		{392, core.OP_ILOAD, []byte{4}}, {394, core.OP_IRETURN, nil},
	} {
		assertReviewedOpcode(t, code, op.pc, op.opcode, op.operand...)
	}
	for _, pc := range []uint16{137, 179, 217, 307, 380} {
		assertReviewedTypeVarInvoke(t, path, "merge", descriptor, pc, core.OP_INVOKESTATIC, "org/objectweb/asm/Frame", "merge", "(Lorg/objectweb/asm/SymbolTable;I[II)Z")
	}
	assertReviewedSources(t, raw, "JDEC_BOOL_ZERO_LITERAL_OFF", func(source string) {
		body := reviewedSourceMethod(t, source, `final boolean merge\(SymbolTable [^,]+, Frame [^,]+, int [^)]+\)`)
		decl := requireReviewedPattern(t, body, `\b(int|boolean)\s+(\w+)\s*=\s*(?:0|false);\s*int\s+\w+\s*=\s*this\.inputLocals\.length;`)
		carrier := decl[2]
		name := regexp.QuoteMeta(carrier)
		if len(regexp.MustCompile(`\b`+name+`\s*=\s*(?:1|true);`).FindAllString(body, -1)) != 3 {
			t.Fatalf("original three destination allocations lost the carrier stores:\n%s", body)
		}
		merges := regexp.MustCompile(`\b`+name+`\s*(?:\|=|=\s*\(?`+name+`\)?\s*\|)\s*\(*merge\(`).FindAllStringIndex(body, -1)
		if len(merges) != 5 || strings.Contains(body, "||") {
			t.Fatalf("original five eager merge calls lost accumulator def-use or became short-circuit:\n%s", body)
		}
		returnPattern := `return\s+\(\(\(` + name + `\)\s*&\s*1\)\s*!=\s*0\);`
		if decl[1] == "boolean" {
			returnPattern = `return\s+\(?` + name + `\)?;`
		}
		returns := regexp.MustCompile(returnPattern).FindAllString(body, -1)
		if len(returns) != 2 {
			t.Fatalf("original two Z return sinks lost the same carrier's low-bit view:\n%s", body)
		}
		for _, field := range []string{"inputLocals", "inputStack"} {
			requireReviewedPattern(t, body, `\.`+field+`\s*=\s*new int\[`)
			requireReviewedPattern(t, body, `merge\([^;]*\.`+field+`[^;]*`)
			if decl[1] == "int" {
				requireReviewedPattern(t, body, `merge\([^;]*\.`+field+`[^;]*\?\s*1\s*:\s*0`)
			}
		}
	})
}
