package javaclassparser

import (
	"encoding/binary"
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Metadata/source inspection only: the historical class is not host executed.
// Original slot3 is an int materialization, while _nonMerging is declared Z.
// Their IF_ICMPEQ compares computational words. The active renderer may lift Z
// to 0/1; replacing an arbitrary int by int!=0 is not an equivalent comparison.
func TestBoolIntOperandCmpIsLoadBearing(t *testing.T) {
	data, err := os.ReadFile("testdata/regression/UntypedObjectDeserializer.class")
	if err != nil {
		t.Fatal(err)
	}
	const owner = "com/fasterxml/jackson/databind/deser/std/UntypedObjectDeserializer"
	const descriptor = "(Lcom/fasterxml/jackson/databind/DeserializationContext;Lcom/fasterxml/jackson/databind/BeanProperty;)Lcom/fasterxml/jackson/databind/JsonDeserializer;"
	assertReviewedGenericField(t, data, "_nonMerging", "Z", "")
	code, object := reviewedControlCode(t, data, "createContextual", descriptor, nil)
	reviewedControlInvokes(t, code, object,
		reviewedViewInvoke{owner, "_nonMerging", "Z", core.OP_GETFIELD},
		reviewedViewInvoke{owner + "NR", "instance", "(Z)Lcom/fasterxml/jackson/databind/deser/std/UntypedObjectDeserializerNR;", core.OP_INVOKESTATIC},
		reviewedViewInvoke{owner, "<init>", "(L" + owner + ";Z)V", core.OP_INVOKESPECIAL})
	decoder := core.NewDecompiler(code.Code, nil)
	if err := decoder.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	positions := map[uint16]*core.OpCode{}
	for _, op := range decoder.Opcodes() {
		positions[op.CurrentOffset] = op
	}
	// Both canonical producer arms define slot3; the same local is loaded into
	// the equality, the fast factory and the conditional copy constructor.
	for _, wanted := range []struct {
		pc     uint16
		opcode int
	}{{22, core.OP_ICONST_1}, {23, core.OP_GOTO}, {26, core.OP_ICONST_0}, {27, core.OP_ISTORE_3}, {65, core.OP_ILOAD_3}, {66, core.OP_INVOKESTATIC}, {70, core.OP_ILOAD_3}, {71, core.OP_ALOAD_0}, {72, core.OP_GETFIELD}, {75, core.OP_IF_ICMPEQ}, {78, core.OP_NEW}, {82, core.OP_ALOAD_0}, {83, core.OP_ILOAD_3}, {84, core.OP_INVOKESPECIAL}, {87, core.OP_ARETURN}, {88, core.OP_ALOAD_0}, {89, core.OP_ARETURN}} {
		op := positions[wanted.pc]
		if op == nil || op.Instr.OpCode != wanted.opcode {
			t.Fatalf("original slot/branch binding atPC%d changed", wanted.pc)
		}
	}
	for _, edge := range []struct{ pc, target uint16 }{{23, 27}, {75, 88}} {
		op := positions[edge.pc]
		if len(op.Data) != 2 || int(edge.pc)+int(int16(binary.BigEndian.Uint16(op.Data))) != int(edge.target) {
			t.Fatal("original local join or equality return ownership changed")
		}
	}
	field := object.ConstantPool[int(binary.BigEndian.Uint16(positions[72].Data))-1].(*ConstantFieldrefInfo)
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	pair := cp.IndexInfo(int(field.NameAndTypeIndex)).(*ConstantNameAndTypeInfo)
	if cp.GetClassName(int(field.ClassIndex)) != owner || cp.GetUtf8(int(pair.NameIndex)).Value != "_nonMerging" || cp.GetUtf8(int(pair.DescriptorIndex)).Value != "Z" {
		t.Fatal("equality field is not original Z receiver member")
	}
	var enabledCondition, local string
	for _, setting := range []string{"", "1"} {
		t.Setenv("JDEC_BOOL_INT_OPERAND_CMP_OFF", setting)
		source, err := Decompile(data)
		if err != nil {
			t.Fatal(err)
		}
		body := reviewedControlBody(t, source, `\bcreateContextual\s*\(`)
		decl := regexp.MustCompile(`\bint\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=`).FindAllStringSubmatch(body, -1)
		if len(decl) != 1 {
			t.Fatalf("original materialized int local declaration lost: %s", body)
		}
		local = decl[0][1]
		var condition string
		cursor := 0
		for cursor < len(body) {
			hit := strings.Index(body[cursor:], "if (")
			if hit < 0 {
				break
			}
			open := cursor + hit + 3
			close := javaMatchParen(body, open)
			if close < 0 {
				t.Fatal("unclosed source control condition")
			}
			candidate := body[open+1 : close]
			if strings.Contains(candidate, "_nonMerging") {
				if condition != "" {
					t.Fatal("original equality field duplicated into multiple controls")
				}
				condition = candidate
				tail := compactReviewedGenericSource(body[close+1:])
				if !strings.HasPrefix(tail, "{returnnewUntypedObjectDeserializer(this,") || !strings.Contains(tail, local) {
					t.Fatal("equality lost original copy-constructor branch ownership")
				}
			}
			cursor = close + 1
		}
		if condition == "" || strings.Count(condition, "_nonMerging") != 1 || len(regexp.MustCompile(`\b`+regexp.QuoteMeta(local)+`\b`).FindAllString(condition, -1)) != 1 {
			t.Fatal("original field/local comparison def-use relationship missing")
		}
		if setting == "" {
			enabledCondition = condition
			continue
		}
		compact := compactReviewedGenericSource(condition)
		plain := strings.NewReplacer("(", "", ")", "").Replace(compact)
		if plain != "this._nonMerging!="+local && plain != local+"!=this._nonMerging" {
			t.Fatalf("gateOFF no longer pins raw int/Z comparison: %s", condition)
		}
	}
	// Compile and execute only the extracted condition in an authored tiny
	// receiver. Historical Jackson code and dependencies are never executed.
	// 2/-1 distinguish exact numeric equality from the old nonzero collapse.
	javac, java := t04Tools(t)
	dir := t.TempDir()
	fragment := fmt.Sprintf(`public class BoolIntComparisonFragment {boolean _nonMerging;boolean changed(int word){int %s=word;return %s;}public static void main(String[]args){BoolIntComparisonFragment x=new BoolIntComparisonFragment();for(boolean b:new boolean[]{false,true})for(int word:new int[]{0,1,2,-1,-2}){x._nonMerging=b;System.out.println(x.changed(word));}}}`, local, enabledCondition)
	file := filepath.Join(dir, "BoolIntComparisonFragment.java")
	if err := os.WriteFile(file, []byte(fragment), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("original-binding source condition not typecorrect: %v %s", err, out)
	}
	var want strings.Builder
	for _, wordOfBool := range []int{0, 1} {
		for _, word := range []int{0, 1, 2, -1, -2} {
			fmt.Fprintln(&want, wordOfBool != word)
		}
	}
	if got := t04RunJava(t, java, dir, "BoolIntComparisonFragment"); got != want.String() {
		t.Fatalf("emitted equality is not full computational-word equality: got%q want%q condition%s", got, want.String(), enabledCondition)
	}
}

func snippetNonMerging(src string) string {
	var b strings.Builder
	for _, ln := range strings.Split(src, "\n") {
		if strings.Contains(ln, "_nonMerging") || strings.Contains(ln, "var3") {
			b.WriteString(ln)
			b.WriteByte('\n')
		}
	}
	s := b.String()
	if len(s) > 1200 {
		return s[:1200]
	}
	return s
}
