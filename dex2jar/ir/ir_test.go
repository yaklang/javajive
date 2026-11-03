package ir

import (
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
		Code:        &model.Code{RegistersSize: registers, InsSize: ins, Instructions: code},
	}
	return Input{File: file, Class: class, Method: method}
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
	if method.RegisterTypes[0].Kind != IntLike || method.RegisterTypes[1].Kind != IntLike {
		t.Fatalf("register categories = %#v", method.RegisterTypes)
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
			name: "monitor",
			in:   fixture([]uint16{0x001d, 0x000e}, 1, 0, model.Proto{ReturnType: "V"}, dexAccessStatic),
			want: "outside the supported",
		},
		{
			name: "try catch",
			in: func() Input {
				in := fixture([]uint16{0x000e}, 0, 0, model.Proto{ReturnType: "V"}, dexAccessStatic)
				in.Method.Code.Tries = []model.Try{{}}
				return in
			}(),
			want: "try/catch is unsupported",
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
	if method.RegisterTypes[0].Kind != Reference || method.RegisterTypes[1].Kind != Reference {
		t.Fatalf("register categories = %#v", method.RegisterTypes)
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
	if method.RegisterTypes[0].Allocation == 0 || method.RegisterTypes[0].Allocation != method.RegisterTypes[1].Allocation {
		t.Fatalf("register identities = %#v", method.RegisterTypes)
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

func TestBuildRejectsStableLocalCategoryConflict(t *testing.T) {
	// v0 is first an int, then a string reference.
	in := fixture([]uint16{
		0x1012,
		0x001a, 0x0000,
		0x0011,
	}, 1, 0, model.Proto{ReturnType: "Ljava/lang/String;"}, dexAccessStatic)
	_, err := Build(in)
	if err == nil || !strings.Contains(err.Error(), "incompatible stable local categories") {
		t.Fatalf("error = %v", err)
	}
}
