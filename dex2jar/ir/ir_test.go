package ir

import (
	"math"
	"strings"
	"testing"

	"github.com/yaklang/javajive/dex2jar/model"
	"github.com/yaklang/javajive/dex2jar/report"
)

func fixture(code []uint16, registers, ins uint16, proto model.Proto, flags uint32) Input {
	file := &model.File{
		Entry:   "classes.dex",
		Strings: []string{"hello"},
		Types: []model.Type{
			{Descriptor: "Ljava/lang/String;"},
			{Descriptor: "LFoo;"},
			{Descriptor: "[I"},
		},
		Protos: []model.Proto{proto, {ReturnType: "LFoo;"}, {ReturnType: "V"}},
		Fields: []model.FieldID{{Class: "LFoo;", Name: "value", Type: "I"}},
		Methods: []model.MethodID{
			{Class: "LFoo;", Name: "test", ProtoIndex: 0},
			{Class: "LFoo;", Name: "make", ProtoIndex: 1},
			{Class: "LFoo;", Name: "<init>", ProtoIndex: 2},
		},
	}
	class := &model.Class{Descriptor: "LFoo;"}
	method := &model.EncodedMethod{
		AccessFlags: flags,
		Method:      file.Methods[0],
		Code:        &model.Code{RegistersSize: registers, InsSize: ins, OutsSize: 8, Instructions: code},
	}
	return Input{File: file, Class: class, Method: method}
}

func TestBuildRejectsAmplifiedRegisterStateSpace(t *testing.T) {
	words := make([]uint16, 65)
	for index := range words {
		words[index] = 0x000e
	}
	_, err := Build(fixture(words, math.MaxUint16, 0, model.Proto{ReturnType: "V"}, dexAccessStatic))
	if err == nil || !strings.Contains(err.Error(), "instruction/register state space") || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("Build() error = %v", err)
	}
	if report.IsUnsupported(err) {
		t.Fatalf("resource error was marked unsupported: %v", err)
	}
}

func TestBuildRejectsInsufficientOutsSize(t *testing.T) {
	// invoke-direct {v0},LFoo;-><init>()V needs one outgoing word.
	in := fixture([]uint16{0x1070, 0x0002, 0x0000, 0x000e}, 1, 0, model.Proto{ReturnType: "V"}, dexAccessStatic)
	in.Method.Code.OutsSize = 0
	_, err := Build(in)
	if err == nil || !strings.Contains(err.Error(), "outs_size is 0") {
		t.Fatalf("error = %v", err)
	}
	diagnostic, ok := report.AsDiagnostic(err)
	if !ok || diagnostic.Offset != 0 {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

func TestDescriptorValidationAcceptsLegalDEXTypes(t *testing.T) {
	for _, test := range []struct {
		descriptor string
		category   CategoryKind
		internal   string
	}{
		{"I", Int, ""},
		{"V", Unknown, ""},
		{"Ljava/lang/String;", Reference, "java/lang/String"},
		{"Lpkg/Outer$Inner;", Reference, "pkg/Outer$Inner"},
		{"[[Ljava/lang/String;", Reference, "[[Ljava/lang/String;"},
		{strings.Repeat("[", 255) + "I", Reference, strings.Repeat("[", 255) + "I"},
	} {
		t.Run(test.descriptor, func(t *testing.T) {
			category, err := DescriptorCategory(test.descriptor)
			if err != nil || category != test.category {
				t.Fatalf("DescriptorCategory(%q) = %s, %v", test.descriptor, category, err)
			}
			if test.internal != "" {
				name, err := InternalName(test.descriptor)
				if err != nil || name != test.internal {
					t.Fatalf("InternalName(%q) = %q, %v", test.descriptor, name, err)
				}
			}
		})
	}
}

func TestDescriptorValidationRejectsMalformedDEXTypes(t *testing.T) {
	for _, descriptor := range []string{
		"", "II", "VV", "L;", "Ljava/lang/String", "Ljava//lang/String;", "L/java/lang/String;",
		"Ljava.lang.String;", "Ljava/lang/String;;", "[", "[V", "[L;", "[Ljava//lang/String;",
		strings.Repeat("[", 256) + "I",
	} {
		t.Run(descriptor, func(t *testing.T) {
			if _, err := DescriptorCategory(descriptor); err == nil {
				t.Fatalf("DescriptorCategory accepted %q", descriptor)
			}
			if _, err := InternalName(descriptor); err == nil && (strings.HasPrefix(descriptor, "L") || strings.HasPrefix(descriptor, "[")) {
				t.Fatalf("InternalName accepted %q", descriptor)
			}
		})
	}
}

func TestBuildRejectsMalformedDescriptorsAndIllegalNames(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(Input) Input
		want   string
	}{
		{
			name: "malformed class descriptor",
			mutate: func(in Input) Input {
				in.Class.Descriptor = "Lbad//Class;"
				return in
			},
			want: "class descriptor",
		},
		{
			name: "malformed return object descriptor",
			mutate: func(in Input) Input {
				in.File.Protos[0].ReturnType = "Lbad//Type;"
				return in
			},
			want: "return type",
		},
		{
			name: "void parameter",
			mutate: func(in Input) Input {
				in.File.Protos[0].Parameters = []string{"V"}
				in.File.Protos[0].ParameterTypes = []uint32{0}
				return in
			},
			want: "void is not permitted",
		},
		{
			name: "malformed array parameter",
			mutate: func(in Input) Input {
				in.File.Protos[0].Parameters = []string{"[Lbad//Type;"}
				in.File.Protos[0].ParameterTypes = []uint32{0}
				return in
			},
			want: "invalid object type",
		},
		{
			name: "excess array depth",
			mutate: func(in Input) Input {
				in.File.Protos[0].ReturnType = strings.Repeat("[", 256) + "I"
				return in
			},
			want: "more than 255 dimensions",
		},
		{
			name: "illegal method name",
			mutate: func(in Input) Input {
				in.Method.Method.Name = "bad/name"
				return in
			},
			want: "method name",
		},
		{
			name: "constructor return",
			mutate: func(in Input) Input {
				in.Method.Method.Name = "<init>"
				in.File.Protos[0].ReturnType = "I"
				return in
			},
			want: "constructor must return void",
		},
		{
			name: "class initializer parameters",
			mutate: func(in Input) Input {
				in.Method.Method.Name = "<clinit>"
				in.File.Protos[0].Parameters = []string{"I"}
				in.File.Protos[0].ParameterTypes = []uint32{0}
				return in
			},
			want: "class initializer must have descriptor ()V",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			in := fixture([]uint16{0x000e}, 0, 0, model.Proto{ReturnType: "V"}, dexAccessStatic)
			in = test.mutate(in)
			_, err := Build(in)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Build error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBuildRejectsOpcodeSpecificTypeDescriptors(t *testing.T) {
	tests := []struct {
		name       string
		code       []uint16
		descriptor string
		want       string
	}{
		{"check-cast primitive", []uint16{0x001f, 0x0000, 0x000e}, "I", "not a reference type"},
		{"new-instance array", []uint16{0x0022, 0x0000, 0x000e}, "[I", "not an object type"},
		{"new-array object", []uint16{0x0123, 0x0000, 0x000e}, "Ljava/lang/String;", "not an array type"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			in := fixture(test.code, 2, 0, model.Proto{ReturnType: "V"}, dexAccessStatic)
			in.File.Types[0].Descriptor = test.descriptor
			_, err := Build(in)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Build error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestBuildRejectsInconsistentIndexedMetadata(t *testing.T) {
	valid := func() Input {
		file := &model.File{
			Entry:  "classes.dex",
			Header: model.Header{StringIDsSize: 6, TypeIDsSize: 3, ProtoIDsSize: 1, FieldIDsSize: 1, MethodIDsSize: 1},
			Strings: []string{
				"LTest;", "V", "V", "caller", "I", "value",
			},
			Types: []model.Type{
				{DescriptorIndex: 0, Descriptor: "LTest;"},
				{DescriptorIndex: 1, Descriptor: "V"},
				{DescriptorIndex: 4, Descriptor: "I"},
			},
			Protos:  []model.Proto{{ShortyIndex: 2, Shorty: "V", ReturnTypeIndex: 1, ReturnType: "V"}},
			Fields:  []model.FieldID{{ClassIndex: 0, TypeIndex: 2, NameIndex: 5, Class: "LTest;", Type: "I", Name: "value"}},
			Methods: []model.MethodID{{ClassIndex: 0, ProtoIndex: 0, NameIndex: 3, Class: "LTest;", Name: "caller"}},
		}
		class := &model.Class{ClassIndex: 0, Descriptor: "LTest;"}
		method := &model.EncodedMethod{MethodIndex: 0, AccessFlags: dexAccessStatic, Method: file.Methods[0], Code: &model.Code{Instructions: []uint16{0x000e}}}
		return Input{File: file, Class: class, Method: method}
	}
	if _, err := Build(valid()); err != nil {
		t.Fatalf("valid indexed metadata rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(Input) Input
		want   string
	}{
		{"header count", func(in Input) Input { in.File.Header.TypeIDsSize = 4; return in }, "type_ids size"},
		{"type string index", func(in Input) Input { in.File.Types[0].DescriptorIndex = 1; return in }, "descriptor does not match"},
		{"proto return index", func(in Input) Input { in.File.Protos[0].ReturnTypeIndex = 0; return in }, "return type index"},
		{"field class index", func(in Input) Input { in.File.Fields[0].ClassIndex = 1; return in }, "field 0 class index"},
		{"field type index", func(in Input) Input { in.File.Fields[0].TypeIndex = 1; return in }, "field 0 type index"},
		{"field name index", func(in Input) Input { in.File.Fields[0].NameIndex = 3; return in }, "field 0 name index"},
		{"method class index", func(in Input) Input {
			in.File.Methods[0].ClassIndex = 1
			in.Method.Method = in.File.Methods[0]
			return in
		}, "class index"},
		{"method name index", func(in Input) Input {
			in.File.Methods[0].NameIndex = 0
			in.Method.Method = in.File.Methods[0]
			return in
		}, "name index"},
		{"encoded method index", func(in Input) Input { in.Method.MethodIndex = 1; return in }, "method index 1"},
		{"encoded method copy", func(in Input) Input { in.Method.Method.Name = "other"; return in }, "does not match encoded"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			in := test.mutate(valid())
			_, err := Build(in)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Build error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestDecodeExactFormatsAndOffsets(t *testing.T) {
	code := &model.Code{Instructions: []uint16{
		0xf312,         // const/4 v3, -1
		0x0202, 0x012c, // move/from16 v2, v300
		0x0320, 0x0001, // instance-of v0, v2, type@1
		0x1074, 0x0001, 0x0014, // invoke-virtual/range {v20}, method@1
		0x002a, 0xfff8, 0xffff, // goto/32 -8 code units
	}}
	got, err := Decode(code, Context{Entry: "classes.dex", Class: "LFoo;", Method: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 5 {
		t.Fatalf("decoded %d instructions", len(got))
	}
	checks := []struct {
		offset uint32
		width  uint16
		format string
	}{
		{0, 1, "11n"}, {1, 2, "22x"}, {3, 2, "22c"}, {5, 3, "3rc"}, {8, 3, "30t"},
	}
	for index, check := range checks {
		if got[index].Offset != check.offset || got[index].Width != check.width || got[index].Format != check.format {
			t.Fatalf("instruction %d = offset %d width %d format %q", index, got[index].Offset, got[index].Width, got[index].Format)
		}
	}
	if got[0].Literal != -1 || got[1].Regs[1] != 300 || got[2].Index != 1 || got[3].Regs[0] != 20 || got[4].Target != 0 {
		t.Fatalf("decoded operands = %#v", got)
	}
}

func TestDecodeRejectsReservedBitsAndTruncation(t *testing.T) {
	for name, code := range map[string][]uint16{
		"reserved":  {0x010e},
		"truncated": {0x0014, 1},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Decode(&model.Code{Instructions: code}, Context{Entry: "x.dex", Class: "LC;", Method: "m"})
			if err == nil {
				t.Fatal("expected error")
			}
			diagnostic, ok := report.AsDiagnostic(err)
			if !ok || diagnostic.Offset != 0 || diagnostic.Entry != "x.dex" || diagnostic.Op == "" {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
		})
	}
}

func TestBuildCFGAndRegisterCategories(t *testing.T) {
	// static int choose(int p): v0 = 0; if-eq p,v0,+4; v0=2; return v0; v0=1; return v0
	in := fixture([]uint16{
		0x0012,
		0x0132, 0x0004,
		0x2012,
		0x000f,
		0x1012,
		0x000f,
	}, 2, 1, model.Proto{ReturnType: "I", Parameters: []string{"I"}, ParameterTypes: []uint32{0}}, dexAccessStatic)
	method, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(method.Blocks) != 3 {
		t.Fatalf("blocks = %#v", method.Blocks)
	}
	if got := method.Blocks[0].Successors; len(got) != 2 || got[0] != 2 || got[1] != 1 {
		t.Fatalf("entry successors = %v", got)
	}
	if method.RegisterTypes[1].Kind != IntLike {
		t.Fatalf("parameter category = %#v", method.RegisterTypes)
	}
	// const/4 v0, #0 starts as a null constant and adopts the int type of its
	// first use, so the definition is refined to int.
	if definition, ok := method.DefinitionAt(0); !ok || definition.Category.Kind != IntLike {
		t.Fatalf("entry definition = %#v, %v", definition, ok)
	}
	if definition, ok := method.DefinitionAt(2); !ok || definition.Category.Kind != IntLike {
		t.Fatalf("second definition = %#v, %v", definition, ok)
	}
	for _, block := range method.Blocks {
		if len(block.Entry) != 2 {
			t.Fatalf("block entry categories = %#v", block.Entry)
		}
	}
}

func TestBuildValidatesTargetsMoveResultRegistersAndUnsupported(t *testing.T) {
	cases := []struct {
		name string
		in   Input
		want string
	}{
		{
			name: "middle of instruction",
			in:   fixture([]uint16{0x0228, 0x0013, 0x0000, 0x000e}, 1, 0, model.Proto{ReturnType: "V"}, dexAccessStatic),
			want: "not an instruction boundary",
		},
		{
			name: "move-result placement",
			in:   fixture([]uint16{0x000a, 0x000f}, 1, 0, model.Proto{ReturnType: "I"}, dexAccessStatic),
			want: "must immediately follow",
		},
		{
			name: "register range",
			in:   fixture([]uint16{0x0102, 0x0002, 0x000e}, 2, 0, model.Proto{ReturnType: "V"}, dexAccessStatic),
			want: "outside register file",
		},
		{
			name: "method handle",
			in:   fixture([]uint16{0x00fe, 0x0000}, 1, 0, model.Proto{ReturnType: "V"}, dexAccessStatic),
			want: "outside the supported",
		},
		{
			name: "try catch",
			in: func() Input {
				in := fixture([]uint16{0x000e}, 0, 0, model.Proto{ReturnType: "V"}, dexAccessStatic)
				in.Method.Code.Tries = []model.Try{{}}
				return in
			}(),
			want: "empty protected range",
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := Build(test.in)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
			diagnostic, ok := report.AsDiagnostic(err)
			if !ok || diagnostic.Entry != "classes.dex" || diagnostic.Class != "LFoo;" || diagnostic.Method != "test" {
				t.Fatalf("diagnostic = %#v", diagnostic)
			}
		})
	}
}

func TestBuildMoveResultAndNullReferenceInference(t *testing.T) {
	// invoke-static make()LFoo;; move-result-object v0; const/4 v1,0;
	// if-eq v0,v1,+3; return-object v0; return-object v1
	in := fixture([]uint16{
		0x0071, 0x0001, 0x0000,
		0x000c,
		0x0112,
		0x1032, 0x0003,
		0x0011,
		0x0111,
	}, 2, 0, model.Proto{ReturnType: "LFoo;"}, dexAccessStatic)
	method, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if definition, ok := method.DefinitionAt(1); !ok || definition.Register != 0 || definition.Category.Kind != Reference {
		t.Fatalf("move-result definition = %#v, %v", definition, ok)
	}
	// v1 is first a null constant and is promoted to a reference by the compare.
	if category := method.CategoryAt(5, 1); category.Kind != Reference {
		t.Fatalf("v1 category before compare = %#v", category)
	}
}

func TestBuildConstructorAllocationIdentity(t *testing.T) {
	// new-instance v0,LFoo;; move-object v1,v0; invoke-direct {v1},<init>; return-object v0
	in := fixture([]uint16{
		0x0022, 0x0001,
		0x0107,
		0x1070, 0x0002, 0x0001,
		0x0011,
	}, 2, 0, model.Proto{ReturnType: "LFoo;"}, dexAccessStatic)
	method, err := Build(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(method.Allocations) != 1 || method.Allocations[0].Constructor != 2 {
		t.Fatalf("allocations = %#v", method.Allocations)
	}
	if got := method.Allocations[0].AliasRegisters; len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("aliases = %v", got)
	}
	first, firstOK := method.DefinitionAt(0)
	second, secondOK := method.DefinitionAt(1)
	if !firstOK || !secondOK || first.Category.Allocation == 0 || first.Category.Allocation != second.Category.Allocation {
		t.Fatalf("allocation identities = %#v / %#v", first, second)
	}
}

func TestBuildRejectsUnsafeConstructorShape(t *testing.T) {
	in := fixture([]uint16{
		0x0022, 0x0001,
		0x0011,
	}, 1, 0, model.Proto{ReturnType: "LFoo;"}, dexAccessStatic)
	_, err := Build(in)
	if err == nil || !strings.Contains(err.Error(), "unsafe uninitialized allocation") {
		t.Fatalf("error = %v", err)
	}
	diagnostic, ok := report.AsDiagnostic(err)
	if !ok || diagnostic.Offset != 2 || diagnostic.Op == "" {
		t.Fatalf("diagnostic = %#v", diagnostic)
	}
}

// Reusing one register for an int and later for a reference is ordinary DEX and
// JVM practice, so it must translate rather than be rejected.
func TestBuildAllowsRegisterLifetimeReuse(t *testing.T) {
	in := fixture([]uint16{
		0x1012,         // const/4 v0, #1
		0x001a, 0x0000, // const-string v0, "hello"
		0x0011, // return-object v0
	}, 1, 0, model.Proto{ReturnType: "Ljava/lang/String;"}, dexAccessStatic)
	method, err := Build(in)
	if err != nil {
		t.Fatalf("Build() rejected legal register reuse: %v", err)
	}
	if definition, ok := method.DefinitionAt(1); !ok || definition.Category.Kind != Reference {
		t.Fatalf("second lifetime = %#v, %v", definition, ok)
	}
}

// A register that is live across a join with two different categories is a real
// type conflict and must still be rejected.
func TestBuildRejectsIncompatibleCategoriesAtJoin(t *testing.T) {
	// const/4 v0,#1; if-eqz v0,+5; const-string v0,"x"; goto +2; const/4 v0,#2;
	// return-object v0. Both branches reach the return with different categories.
	in := fixture([]uint16{
		0x1012,         // const/4 v0, #1
		0x0038, 0x0005, // if-eqz v0, +5
		0x001a, 0x0000, // const-string v0, "hello"
		0x0228, // goto +2
		0x2012, // const/4 v0, #2
		0x0011, // return-object v0
	}, 1, 0, model.Proto{ReturnType: "Ljava/lang/String;"}, dexAccessStatic)
	_, err := Build(in)
	if err == nil || !strings.Contains(err.Error(), "incompatible categories at a control-flow join") {
		t.Fatalf("error = %v", err)
	}
}

func filledArrayInput(typeDescriptor string, words []uint16) Input {
	file := &model.File{
		Entry:   "classes.dex",
		Types:   []model.Type{{Descriptor: typeDescriptor}},
		Protos:  []model.Proto{{ReturnType: "[I"}},
		Methods: []model.MethodID{{Class: "LFoo;", Name: "test", ProtoIndex: 0}},
	}
	class := &model.Class{Descriptor: "LFoo;"}
	method := &model.EncodedMethod{
		AccessFlags: dexAccessStatic,
		Method:      file.Methods[0],
		Code:        &model.Code{RegistersSize: 2, OutsSize: 2, Instructions: words},
	}
	return Input{File: file, Class: class, Method: method}
}

func TestBuildMarksOnlyKnownCapabilityFailuresUnsupported(t *testing.T) {
	tests := []struct {
		name string
		in   Input
	}{
		{
			name: "opcode family",
			in:   fixture([]uint16{0x00fe, 0x0000}, 1, 0, model.Proto{ReturnType: "V"}, dexAccessStatic),
		},
		{
			name: "filled new array element",
			in:   filledArrayInput("[J", []uint16{0x1024, 0x0000, 0x0000, 0x000c, 0x0011}),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Build(test.in)
			if err == nil || !report.IsUnsupported(err) {
				t.Fatalf("Build() error = %v, want typed unsupported capability", err)
			}
		})
	}

	malformed := filledArrayInput("I", []uint16{0x1024, 0x0000, 0x0000, 0x000c, 0x0011})
	if _, err := Build(malformed); err == nil || report.IsUnsupported(err) {
		t.Fatalf("malformed Build() error = %v, want non-capability diagnostic", err)
	}
}

func TestBuildRejectsMalformedFilledNewArray(t *testing.T) {
	tests := []struct {
		name  string
		kind  string
		words []uint16
		part  string
	}{
		{
			name:  "type index out of range",
			kind:  "[I",
			words: []uint16{0x1024, 0x0005, 0x0000, 0x000c, 0x0011},
			part:  "type index 5 is out of range",
		},
		{
			name:  "non array type",
			kind:  "I",
			words: []uint16{0x1024, 0x0000, 0x0000, 0x000c, 0x0011},
			part:  "is not an array",
		},
		{
			name:  "wide element",
			kind:  "[J",
			words: []uint16{0x1024, 0x0000, 0x0000, 0x000c, 0x0011},
			part:  "filled-new-array of a wide",
		},
		{
			name:  "missing move-result-object",
			kind:  "[I",
			words: []uint16{0x1024, 0x0000, 0x0000, 0x000a, 0x0011},
			part:  "requires move-result-object",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Build(filledArrayInput(test.kind, test.words))
			if err == nil || !strings.Contains(err.Error(), test.part) {
				t.Fatalf("error = %v, want substring %q", err, test.part)
			}
		})
	}
}

func TestBuildRejectsIncompleteInput(t *testing.T) {
	valid := fixture([]uint16{0x000e}, 0, 0, model.Proto{ReturnType: "V"}, dexAccessStatic)
	cases := map[string]Input{
		"nil file":   {Class: valid.Class, Method: valid.Method},
		"nil class":  {File: valid.File, Method: valid.Method},
		"nil method": {File: valid.File, Class: valid.Class},
		"nil code": {
			File: valid.File, Class: valid.Class,
			Method: &model.EncodedMethod{AccessFlags: dexAccessStatic, Method: valid.Method.Method},
		},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Build(in); err == nil || !strings.Contains(err.Error(), "incomplete method input") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
