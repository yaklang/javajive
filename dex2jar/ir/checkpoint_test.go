package ir

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/dex2jar/model"
	"github.com/yaklang/javajive/dex2jar/report"
)

func TestDecodeFormats(t *testing.T) {
	tests := []struct {
		name  string
		words []uint16
		want  Instruction
	}{
		{
			name:  "12x move",
			words: []uint16{0x2101},
			want:  Instruction{Opcode: 0x01, Name: "move", Format: "12x", Width: 1, Regs: []uint16{1, 2}, Target: -1},
		},
		{
			name:  "11n signed const",
			words: []uint16{0xf312},
			want:  Instruction{Opcode: 0x12, Name: "const/4", Format: "11n", Width: 1, Regs: []uint16{3}, Literal: -1, Target: -1},
		},
		{
			name:  "31i const",
			words: []uint16{0x0214, 0xcdef, 0x89ab},
			want:  Instruction{Opcode: 0x14, Name: "const", Format: "31i", Width: 3, Regs: []uint16{2}, Literal: -1985229329, Target: -1},
		},
		{
			name:  "22c instance field",
			words: []uint16{0x3252, 0x4567},
			want:  Instruction{Opcode: 0x52, Name: "iget", Format: "22c", Width: 2, Regs: []uint16{2, 3}, Index: 0x4567, Target: -1},
		},
		{
			name:  "35c invoke register order",
			words: []uint16{0x556e, 0x1234, 0x4321},
			want:  Instruction{Opcode: 0x6e, Name: "invoke-virtual", Format: "35c", Width: 3, Regs: []uint16{1, 2, 3, 4, 5}, Index: 0x1234, Target: -1},
		},
		{
			name:  "3rc invoke range",
			words: []uint16{0x0377, 0x0021, 0x0008},
			want:  Instruction{Opcode: 0x77, Name: "invoke-static/range", Format: "3rc", Width: 3, Regs: []uint16{8, 9, 10}, Index: 0x21, Target: -1},
		},
		{
			name:  "30t backward goto",
			words: []uint16{0x002a, 0xfffe, 0xffff},
			want:  Instruction{Opcode: 0x2a, Name: "goto/32", Format: "30t", Width: 3, Target: -2},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := Decode(&model.Code{Instructions: test.words}, Context{})
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			if len(got) != 1 {
				t.Fatalf("Decode() count = %d, want 1", len(got))
			}
			instructionEqual(t, got[0], test.want)
		})
	}
}

func TestDecodeTruncatedAndPayload(t *testing.T) {
	for _, test := range []struct {
		name  string
		words []uint16
		part  string
	}{
		{"truncated", []uint16{0x0014, 0x1234}, "truncated 31i"},
		{"payload", []uint16{0x0100}, "switch/fill-array payload"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode(&model.Code{Instructions: test.words}, Context{Entry: "classes.dex", Class: "LC;", Method: "m"})
			if err == nil || !strings.Contains(err.Error(), test.part) {
				t.Fatalf("Decode() error = %v, want substring %q", err, test.part)
			}
			assertDiagnostic(t, err, "classes.dex", "LC;", "m", 0)
		})
	}
}

func TestBuildBranchCFG(t *testing.T) {
	// 0: const/4 v0, #1
	// 1: if-eqz v0, 4
	// 3: const/4 v1, #2
	// 4: return-void
	in := testInput("V", nil, 2, 0, []uint16{0x1012, 0x0038, 0x0003, 0x2112, 0x000e})
	method, err := Build(in)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(method.Blocks) != 3 {
		t.Fatalf("block count = %d, want 3", len(method.Blocks))
	}
	assertBlock(t, method.Blocks[0], 0, 3, []int{2, 1})
	assertBlock(t, method.Blocks[1], 3, 4, []int{2})
	assertBlock(t, method.Blocks[2], 4, 5, nil)
}

func TestBuildGotoSplitsFollowingCode(t *testing.T) {
	in := testInput("V", nil, 0, 0, []uint16{0x0228, 0x000e, 0x000e})
	method, err := Build(in)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(method.Blocks) != 3 {
		t.Fatalf("block count = %d, want 3", len(method.Blocks))
	}
	assertBlock(t, method.Blocks[0], 0, 1, []int{2})
	assertBlock(t, method.Blocks[1], 1, 2, nil)
	assertBlock(t, method.Blocks[2], 2, 3, nil)
}

func TestBuildRejectsInvalidBranchRegisterIndexAndMoveResult(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		part string
		off  int64
	}{
		{
			name: "target inside instruction",
			in:   testInput("V", nil, 0, 0, []uint16{0x0029, 0x0001, 0x000e}),
			part: "not an instruction boundary",
			off:  0,
		},
		{
			name: "register out of bounds",
			in:   testInput("V", nil, 2, 0, []uint16{0x0212, 0x000e}),
			part: "register v2 is outside",
			off:  0,
		},
		{
			name: "type index out of bounds",
			in:   testInput("V", nil, 1, 0, []uint16{0x001c, 0x0001, 0x000e}),
			part: "type index 1 is out of range",
			off:  0,
		},
		{
			name: "move-result adjacency",
			in:   testInput("V", nil, 1, 0, []uint16{0x000a, 0x000e}),
			part: "must immediately follow an invoke",
			off:  0,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Build(test.in)
			if err == nil || !strings.Contains(err.Error(), test.part) {
				t.Fatalf("Build() error = %v, want substring %q", err, test.part)
			}
			assertDiagnostic(t, err, "classes.dex", "LTest;", "caller", test.off)
		})
	}
}

func TestBuildValidatesMoveResultAndReturnCategories(t *testing.T) {
	caller := testInput("I", nil, 1, 0, []uint16{0x0071, 0x0000, 0x0000, 0x000a, 0x000f})
	caller.File.Protos = append(caller.File.Protos, model.Proto{ReturnType: "I"})
	caller.File.Methods = []model.MethodID{{Class: "LTarget;", Name: "value", ProtoIndex: 1}}
	method, err := Build(caller)
	if err != nil {
		t.Fatalf("Build() valid invoke error = %v", err)
	}
	if method.RegisterTypes[0].Kind != Int {
		t.Fatalf("v0 category = %s, want int", method.RegisterTypes[0].Kind)
	}

	badMove := testInput("V", nil, 1, 0, []uint16{0x0071, 0x0000, 0x0000, 0x000c, 0x000e})
	badMove.File.Protos = append(badMove.File.Protos, model.Proto{ReturnType: "I"})
	badMove.File.Methods = []model.MethodID{{Class: "LTarget;", Name: "value", ProtoIndex: 1}}
	if _, err := Build(badMove); err == nil || !strings.Contains(err.Error(), "category does not match") {
		t.Fatalf("Build() bad move-result error = %v", err)
	}

	badReturn := testInput("I", nil, 1, 0, []uint16{0x000e})
	if _, err := Build(badReturn); err == nil || !strings.Contains(err.Error(), "return-void does not match") {
		t.Fatalf("Build() bad return error = %v", err)
	}
}

func TestCategoryAnalysisSeedsParametersAndRetainsUnknownAndNull(t *testing.T) {
	seeded := testInput("V", []string{"I", "Ljava/lang/String;"}, 3, 2, []uint16{0x000e})
	method, err := Build(seeded)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := []CategoryKind{Unknown, Int, Reference}
	for register, category := range want {
		if method.RegisterTypes[register].Kind != category {
			t.Errorf("v%d category = %s, want %s", register, method.RegisterTypes[register].Kind, category)
		}
	}

	nullInput := testInput("V", nil, 1, 0, []uint16{0x0012, 0x000e})
	nullMethod, err := Build(nullInput)
	if err != nil {
		t.Fatalf("Build(null) error = %v", err)
	}
	if nullMethod.RegisterTypes[0].Kind != Null {
		t.Fatalf("zero constant category = %s, want null", nullMethod.RegisterTypes[0].Kind)
	}
	if Conflict.String() != "conflict" {
		t.Fatalf("Conflict.String() = %q", Conflict.String())
	}
}

func TestUnsupportedFamiliesHavePreciseDiagnostics(t *testing.T) {
	tests := []struct {
		name  string
		words []uint16
		part  string
	}{
		{"wide", []uint16{0x0004, 0x000e}, "wide/float/double"},
		{"monitor", []uint16{0x001d, 0x000e}, "monitor"},
		{"switch", []uint16{0x002b, 0, 0}, "switch"},
		{"fill-array", []uint16{0x0026, 0, 0}, "filled/fill-array"},
		{"narrow-array", []uint16{0x0047, 0, 0x000e}, "non-int/non-reference array element"},
		{"polymorphic", []uint16{0x00fa, 0, 0, 0}, "polymorphic/custom"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			in := testInput("V", nil, 4, 0, test.words)
			_, err := Build(in)
			if err == nil || !strings.Contains(err.Error(), test.part) {
				t.Fatalf("Build() error = %v, want substring %q", err, test.part)
			}
			diagnostic := assertDiagnostic(t, err, "classes.dex", "LTest;", "caller", 0)
			if diagnostic.Op == "" || !strings.Contains(diagnostic.Op, "0x") {
				t.Fatalf("diagnostic opcode = %q, want mnemonic and hex opcode", diagnostic.Op)
			}
		})
	}
}

func testInput(returnType string, parameters []string, registers, ins uint16, words []uint16) Input {
	parameterTypes := make([]uint32, len(parameters))
	file := &model.File{
		Entry:  "classes.dex",
		Types:  []model.Type{{Descriptor: "Ljava/lang/Object;"}},
		Protos: []model.Proto{{ReturnType: returnType, Parameters: parameters, ParameterTypes: parameterTypes}},
	}
	class := &model.Class{Descriptor: "LTest;"}
	method := &model.EncodedMethod{
		AccessFlags: dexAccessStatic,
		Method:      model.MethodID{Class: class.Descriptor, Name: "caller", ProtoIndex: 0},
		Code:        &model.Code{RegistersSize: registers, InsSize: ins, Instructions: words},
	}
	return Input{File: file, Class: class, Method: method}
}

func instructionEqual(t *testing.T, got, want Instruction) {
	t.Helper()
	if got.Offset != want.Offset || got.Opcode != want.Opcode || got.Name != want.Name || got.Format != want.Format || got.Width != want.Width || got.Index != want.Index || got.Literal != want.Literal || got.Target != want.Target || !uint16sEqual(got.Regs, want.Regs) {
		t.Fatalf("instruction = %#v, want %#v", got, want)
	}
}

func uint16sEqual(left, right []uint16) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func assertBlock(t *testing.T, got Block, start, end uint32, successors []int) {
	t.Helper()
	if got.Start != start || got.End != end || !intsEqual(got.Successors, successors) {
		t.Fatalf("block = start %d end %d successors %v, want start %d end %d successors %v", got.Start, got.End, got.Successors, start, end, successors)
	}
}

func intsEqual(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func assertDiagnostic(t *testing.T, err error, entry, class, method string, offset int64) *report.Diagnostic {
	t.Helper()
	diagnostic, ok := report.AsDiagnostic(err)
	if !ok {
		t.Fatalf("error %T is not report.Diagnostic: %v", err, err)
	}
	if diagnostic.Entry != entry || diagnostic.Class != class || diagnostic.Method != method || diagnostic.Offset != offset {
		t.Fatalf("diagnostic context = entry %q class %q method %q offset %d", diagnostic.Entry, diagnostic.Class, diagnostic.Method, diagnostic.Offset)
	}
	return diagnostic
}
