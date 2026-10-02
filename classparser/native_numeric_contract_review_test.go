package javaclassparser

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// These original-class checks inspect metadata/bytecode only. Historical JARs
// are never executed on the host; authored JVM counterexamples are separate.
func reviewedFixtureMethod(t *testing.T, path, name, descriptor string) ([]byte, *CodeAttribute, *ClassObject) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range object.Methods {
		n, _ := object.getUtf8(method.NameIndex)
		d, _ := object.getUtf8(method.DescriptorIndex)
		if n != name || descriptor != "" && d != descriptor {
			continue
		}
		for _, attr := range method.Attributes {
			if code, ok := attr.(*CodeAttribute); ok {
				return raw, code, object
			}
		}
	}
	t.Fatalf("original method %s%s not found in %s", name, descriptor, path)
	return nil, nil, nil
}

func assertReviewedOpcode(t *testing.T, code *CodeAttribute, pc uint16, opcode int, operand ...byte) {
	t.Helper()
	d := core.NewDecompiler(code.Code, nil)
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	op := d.OpcodeByPC(pc)
	if op == nil || op.Instr.OpCode != opcode || len(operand) > len(op.Data) {
		t.Fatalf("original instruction PC %d is not opcode %#x with %v", pc, opcode, operand)
	}
	for i, b := range operand {
		if op.Data[i] != b {
			t.Fatalf("original PC %d operand %v differs from %v", pc, op.Data, operand)
		}
	}
}

func reviewedSourceMethod(t *testing.T, source, declarationPattern string) string {
	t.Helper()
	start := regexp.MustCompile(declarationPattern).FindStringIndex(source)
	if start == nil {
		t.Fatalf("missing declaration %s\n%s", declarationPattern, source)
	}
	// Generated top-level members use one tab. Inner blocks are indented more.
	end := strings.Index(source[start[0]:], "\n\t}")
	if end < 0 {
		t.Fatalf("missing member boundary for %s", declarationPattern)
	}
	return source[start[0] : start[0]+end+3]
}

func assertReviewedSources(t *testing.T, raw []byte, flag string, check func(string)) {
	t.Helper()
	for _, setting := range []string{"", "1"} {
		t.Setenv(flag, setting)
		source, err := Decompile(raw)
		if err != nil {
			t.Fatalf("%s=%q: %v", flag, setting, err)
		}
		check(source)
	}
}

func requireReviewedPattern(t *testing.T, body, pattern string) []string {
	t.Helper()
	match := regexp.MustCompile(pattern).FindStringSubmatch(body)
	if match == nil {
		t.Fatalf("missing semantic structure %s\n%s", pattern, body)
	}
	return match
}

func assertReviewedThrowableResource(t *testing.T, path, flag, declaration string, firstThrow, lastThrow uint16) {
	t.Helper()
	raw, code, object := reviewedFixtureMethod(t, path, "toString", "")
	assertReviewedOpcode(t, code, firstThrow, core.OP_ATHROW)
	assertReviewedOpcode(t, code, lastThrow, core.OP_ATHROW)
	d := core.NewDecompiler(code.Code, nil)
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	for _, op := range d.Opcodes() {
		if op.Instr.OpCode == core.OP_NEW {
			index := int(op.Data[0])<<8 | int(op.Data[1])
			if cp.GetClassName(index) == "java/lang/RuntimeException" {
				t.Fatal("fixture now allocates a RuntimeException; review the contract again")
			}
		}
	}
	assertReviewedSources(t, raw, flag, func(source string) {
		body := reviewedSourceMethod(t, source, declaration)
		if strings.Contains(body, "new RuntimeException(") {
			t.Fatalf("original ATHROW identity became a newly allocated wrapper:\n%s", body)
		}
		// The original exception table has cleanup and addSuppressed paths.
		// Structured try-with-resources is equally valid and synthesizes those paths.
		if !regexp.MustCompile(`try\s*\(`).MatchString(body) &&
			(!strings.Contains(body, "finally") || !strings.Contains(body, "addSuppressed(")) {
			t.Fatalf("original resource cleanup/suppression is not represented:\n%s", body)
		}
	})
}
