package ir

import (
	"strings"
	"testing"

	"github.com/yaklang/javajive/dex2jar/model"
)

func wideInput(returnType string, parameters []string, registers, ins uint16, words []uint16) Input {
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

func TestWideParametersOccupyTwoRegisters(t *testing.T) {
	// static long sum(long a, long b) { a += b; return a; }
	in := wideInput("J", []string{"J", "J"}, 4, 4, []uint16{0x20bb, 0x0010})
	method, err := Build(in)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := []CategoryKind{Long, WideUpper, Long, WideUpper}
	for register, kind := range want {
		if got := method.RegisterTypes[register].Kind; got != kind {
			t.Errorf("v%d category = %s, want %s", register, got, kind)
		}
	}
	if definition, ok := method.DefinitionAt(0); !ok || definition.Category.Kind != Long {
		t.Fatalf("add-long/2addr definition = %#v, %v", definition, ok)
	}
}

func TestWideTwoAddressFloatAndDouble(t *testing.T) {
	// static float scale(float a, float b) { a *= b; return a; }
	float := wideInput("F", []string{"F", "F"}, 2, 2, []uint16{0x10c8, 0x000f})
	method, err := Build(float)
	if err != nil {
		t.Fatalf("Build(float) error = %v", err)
	}
	if definition, ok := method.DefinitionAt(0); !ok || definition.Category.Kind != Float {
		t.Fatalf("mul-float/2addr definition = %#v, %v", definition, ok)
	}

	// static double add(double a, double b) { a += b; return a; }
	double := wideInput("D", []string{"D", "D"}, 4, 4, []uint16{0x20cb, 0x0010})
	method, err = Build(double)
	if err != nil {
		t.Fatalf("Build(double) error = %v", err)
	}
	if definition, ok := method.DefinitionAt(0); !ok || definition.Category.Kind != Double {
		t.Fatalf("add-double/2addr definition = %#v, %v", definition, ok)
	}
}

func TestConstantRefinementAcrossBlocks(t *testing.T) {
	// static double pick(boolean c) { double v = 1.0; if (c) return v; return 2.0; }
	in := wideInput("D", []string{"Z"}, 3, 1, []uint16{
		0x0018, 0x0000, 0x0000, 0x0000, 0x3ff0, // const-wide v0, 1.0
		0x0238, 0x0003, // if-eqz v2, +3
		0x0010,                                 // return-wide v0
		0x0018, 0x0000, 0x0000, 0x0000, 0x4000, // const-wide v0, 2.0
		0x0010, // return-wide v0
	})
	method, err := Build(in)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	for _, index := range []int{0, 3} {
		definition, ok := method.DefinitionAt(index)
		if !ok || definition.Category.Kind != Double {
			t.Fatalf("const at %d definition = %#v, %v; want double", index, definition, ok)
		}
	}
	if got := method.CategoryAt(2, 0).Kind; got != Double {
		t.Fatalf("category at first return = %s, want double", got)
	}
}

func TestConstantRefinementFollowsMoveWide(t *testing.T) {
	// static double forward() { double a = 1.0; double b = a; return b; }
	in := wideInput("D", nil, 4, 0, []uint16{
		0x0018, 0x0000, 0x0000, 0x0000, 0x3ff0, // const-wide v0, 1.0
		0x0204, // move-wide v2, v0
		0x0210, // return-wide v2
	})
	method, err := Build(in)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if definition, ok := method.DefinitionAt(0); !ok || definition.Category.Kind != Double {
		t.Fatalf("const definition = %#v, %v; want double", definition, ok)
	}
	if definition, ok := method.DefinitionAt(1); !ok || definition.Category.Kind != Double {
		t.Fatalf("move-wide definition = %#v, %v; want double", definition, ok)
	}
}

func TestZeroConstantAdoptsFloat(t *testing.T) {
	// static float zero() { return 0.0f; }
	in := wideInput("F", nil, 1, 0, []uint16{0x0012, 0x000f})
	method, err := Build(in)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if definition, ok := method.DefinitionAt(0); !ok || definition.Category.Kind != Float {
		t.Fatalf("zero constant definition = %#v, %v; want float", definition, ok)
	}
}

func TestMoveResultWideUsesCalleePrototype(t *testing.T) {
	file := &model.File{
		Entry:  "classes.dex",
		Types:  []model.Type{{Descriptor: "D"}},
		Protos: []model.Proto{{ReturnType: "D"}, {ReturnType: "D"}},
		Methods: []model.MethodID{
			{Class: "LTarget;", Name: "value", ProtoIndex: 0},
			{Class: "LTest;", Name: "caller", ProtoIndex: 1},
		},
	}
	class := &model.Class{Descriptor: "LTest;"}
	method := &model.EncodedMethod{
		AccessFlags: dexAccessStatic,
		Method:      file.Methods[1],
		Code: &model.Code{RegistersSize: 2, OutsSize: 1, Instructions: []uint16{
			0x0071, 0x0000, 0x0000, // invoke-static {}, Target.value()D
			0x000b, // move-result-wide v0
			0x0010, // return-wide v0
		}},
	}
	built, err := Build(Input{File: file, Class: class, Method: method})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if definition, ok := built.DefinitionAt(1); !ok || definition.Category.Kind != Double {
		t.Fatalf("move-result-wide definition = %#v, %v; want double", definition, ok)
	}

	// A narrow move-result after a double invoke is a category mismatch.
	bad := &model.EncodedMethod{
		AccessFlags: dexAccessStatic,
		Method:      file.Methods[1],
		Code: &model.Code{RegistersSize: 2, OutsSize: 1, Instructions: []uint16{
			0x0071, 0x0000, 0x0000,
			0x000a, // move-result v0
			0x000f, // return v0 (proto returns double, so this also fails)
		}},
	}
	if _, err := Build(Input{File: file, Class: class, Method: bad}); err == nil || !strings.Contains(err.Error(), "move-result category does not match") {
		t.Fatalf("narrow move-result error = %v", err)
	}
}

func TestWideArrayAndFieldCategories(t *testing.T) {
	// static double first(double[] a, int i) { return a[i]; }
	file := &model.File{
		Entry:  "classes.dex",
		Types:  []model.Type{{Descriptor: "D"}, {Descriptor: "[D"}, {Descriptor: "I"}},
		Protos: []model.Proto{{ReturnType: "D", Parameters: []string{"[D", "I"}, ParameterTypes: []uint32{1, 2}}},
	}
	class := &model.Class{Descriptor: "LTest;"}
	method := &model.EncodedMethod{
		AccessFlags: dexAccessStatic,
		Method:      model.MethodID{Class: "LTest;", Name: "first", ProtoIndex: 0},
		Code: &model.Code{RegistersSize: 5, InsSize: 2, Instructions: []uint16{
			0x0045, 0x0403, // aget-wide v0, v3, v4
			0x0010, // return-wide v0
		}},
	}
	built, err := Build(Input{File: file, Class: class, Method: method})
	if err != nil {
		t.Fatalf("Build(array) error = %v", err)
	}
	if definition, ok := built.DefinitionAt(0); !ok || definition.Category.Kind != Double {
		t.Fatalf("aget-wide definition = %#v, %v; want double", definition, ok)
	}

	// static long total() { return Test.VALUE; }
	fields := &model.File{
		Entry:  "classes.dex",
		Types:  []model.Type{{Descriptor: "J"}},
		Protos: []model.Proto{{ReturnType: "J"}},
		Fields: []model.FieldID{{Class: "LTest;", Type: "J", Name: "VALUE"}},
	}
	fieldClass := &model.Class{
		Descriptor:   "LTest;",
		StaticFields: []model.EncodedField{{AccessFlags: dexAccessStatic, Field: fields.Fields[0]}},
	}
	fieldMethod := &model.EncodedMethod{
		AccessFlags: dexAccessStatic,
		Method:      model.MethodID{Class: "LTest;", Name: "total", ProtoIndex: 0},
		Code: &model.Code{RegistersSize: 2, Instructions: []uint16{
			0x0061, 0x0000, // sget-wide v0, VALUE
			0x0010, // return-wide v0
		}},
	}
	built, err = Build(Input{File: fields, Class: fieldClass, Method: fieldMethod})
	if err != nil {
		t.Fatalf("Build(field) error = %v", err)
	}
	if definition, ok := built.DefinitionAt(0); !ok || definition.Category.Kind != Long {
		t.Fatalf("sget-wide definition = %#v, %v; want long", definition, ok)
	}
}

func TestWideOperandValidation(t *testing.T) {
	// A category-2 value at the last register has no second half.
	last := wideInput("J", nil, 1, 0, []uint16{0x0016, 0x0001, 0x0010})
	if _, err := Build(last); err == nil || !strings.Contains(err.Error(), "requires register v1") {
		t.Fatalf("last-register wide error = %v", err)
	}

	// A wide invoke argument lists both of its registers.
	invokeFile := &model.File{
		Entry:   "classes.dex",
		Types:   []model.Type{{Descriptor: "J"}},
		Protos:  []model.Proto{{ReturnType: "V", Parameters: []string{"J"}, ParameterTypes: []uint32{0}}, {ReturnType: "V"}},
		Methods: []model.MethodID{{Class: "LTarget;", Name: "take", ProtoIndex: 0}, {Class: "LTest;", Name: "caller", ProtoIndex: 1}},
	}
	invokeClass := &model.Class{Descriptor: "LTest;"}
	invokeMethod := &model.EncodedMethod{
		AccessFlags: dexAccessStatic,
		Method:      invokeFile.Methods[1],
		Code: &model.Code{RegistersSize: 4, OutsSize: 2, Instructions: []uint16{
			0x2071, 0x0000, 0x0020, // invoke-static {v0, v2}, Target.take(J)V
			0x000e,
		}},
	}
	if _, err := Build(Input{File: invokeFile, Class: invokeClass, Method: invokeMethod}); err == nil || !strings.Contains(err.Error(), "wide argument v0 must be followed by v1") {
		t.Fatalf("non-consecutive wide argument error = %v", err)
	}

	// A constant read as both int and float cannot be resolved.
	conflict := wideInput("V", nil, 2, 0, []uint16{
		0x1012,         // const/4 v0, #1
		0x00a6, 0x0000, // add-float v1, v0, v0
		0x0090, 0x0000, // add-int v1, v0, v0
		0x000e,
	})
	if _, err := Build(conflict); err == nil || !strings.Contains(err.Error(), "incompatible stable local categories") {
		t.Fatalf("mixed constant error = %v", err)
	}

	// A tracked array component that contradicts the opcode is a stale
	// descriptor from another lifetime; the opcode family wins.
	arrayFile := &model.File{
		Entry:  "classes.dex",
		Types:  []model.Type{{Descriptor: "I"}},
		Protos: []model.Proto{{ReturnType: "I", Parameters: []string{"[I"}, ParameterTypes: []uint32{0}}},
	}
	arrayClass := &model.Class{Descriptor: "LTest;"}
	arrayMethod := &model.EncodedMethod{
		AccessFlags: dexAccessStatic,
		Method:      model.MethodID{Class: "LTest;", Name: "caller", ProtoIndex: 0},
		Code: &model.Code{RegistersSize: 3, InsSize: 1, Instructions: []uint16{
			0x0048, 0x0102, // aget-byte v0, v2, v1
			0x000f,
		}},
	}
	arrayBuilt, err := Build(Input{File: arrayFile, Class: arrayClass, Method: arrayMethod})
	if err != nil {
		t.Fatalf("stale array component rejected: %v", err)
	}
	if definition, ok := arrayBuilt.DefinitionAt(0); !ok || definition.Category.Kind != Int {
		t.Fatalf("aget-byte definition = %#v, %v", definition, ok)
	}
}
