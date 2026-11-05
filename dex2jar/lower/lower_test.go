package lower

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	javaclassparser "github.com/yaklang/javajive/classparser"
	"github.com/yaklang/javajive/dex2jar/model"
	"github.com/yaklang/javajive/dex2jar/report"
)

func TestClassParsesVerifiesAndExecutes(t *testing.T) {
	file, class := executableFixture()
	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatalf("parse generated class: %v", err)
	}
	if parsed.MajorVersion != 49 || parsed.MinorVersion != 0 {
		t.Fatalf("version = %d.%d", parsed.MajorVersion, parsed.MinorVersion)
	}
	if got := parsed.GetClassName(); got != "dex/Generated" {
		t.Fatalf("class name = %q", got)
	}
	if len(parsed.Fields) != 1 || len(parsed.Methods) != 5 {
		t.Fatalf("members = %d fields, %d methods", len(parsed.Fields), len(parsed.Methods))
	}

	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java is unavailable")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dex", "Generated.class")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(java, "-Xverify:all", "-cp", dir, "dex.Generated")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("java verification/execution: %v\n%s", err, output)
	}
}

func TestAbstractAndNativeMethodsHaveNoCode(t *testing.T) {
	file := &model.File{
		Entry:  "classes.dex",
		Protos: []model.Proto{{ReturnType: "V"}},
	}
	class := &model.Class{
		Descriptor:  "Ldex/Declarations;",
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic | dexAbstract,
		VirtualMethods: []model.EncodedMethod{
			{AccessFlags: dexPublic | dexAbstract, Method: model.MethodID{Class: "Ldex/Declarations;", Name: "abstractMethod", ProtoIndex: 0}},
			{AccessFlags: dexPublic | dexNative, Method: model.MethodID{Class: "Ldex/Declarations;", Name: "nativeMethod", ProtoIndex: 0}},
		},
	}
	raw, err := Class(file, class)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := javaclassparser.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range parsed.Methods {
		if len(method.Attributes) != 0 {
			t.Fatalf("declaration-only method has %d attributes", len(method.Attributes))
		}
	}
}

func TestIntArrayPathVerifiesAndExecutes(t *testing.T) {
	file, class := arrayFixture()
	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	if _, err := javaclassparser.Parse(raw); err != nil {
		t.Fatalf("parse generated array class: %v", err)
	}
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java is unavailable")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dex", "Arrays.class")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "dex.Arrays").CombinedOutput(); err != nil {
		t.Fatalf("java array verification/execution: %v\n%s", err, output)
	}
}

func TestArrayCloneOwnerVerifiesAndExecutes(t *testing.T) {
	file, class := arrayCloneFixture()
	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	if _, err := javaclassparser.Parse(raw); err != nil {
		t.Fatalf("parse generated array clone class: %v", err)
	}
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java is unavailable")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dex", "ArrayClone.class")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "dex.ArrayClone").CombinedOutput(); err != nil {
		t.Fatalf("java array clone verification/execution: %v\n%s", err, output)
	}
}

func TestUnsupportedMethodReturnsNoClassAndDiagnostic(t *testing.T) {
	file := &model.File{Entry: "classes.dex", Protos: []model.Proto{{ReturnType: "V"}}}
	class := &model.Class{
		Descriptor:  "Ldex/Rejected;",
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{{
			AccessFlags: dexPublic | dexStatic,
			Method:      model.MethodID{Class: "Ldex/Rejected;", Name: "bad", ProtoIndex: 0},
			Code:        &model.Code{RegistersSize: 1, Instructions: []uint16{0x001d, 0x000e}},
		}},
	}
	raw, err := Class(file, class)
	if err == nil || raw != nil {
		t.Fatalf("Class = %d bytes, %v; want nil bytes and error", len(raw), err)
	}
	diagnostic, ok := report.AsDiagnostic(err)
	if !ok {
		t.Fatalf("error is not Diagnostic: %T %v", err, err)
	}
	if diagnostic.Entry != "classes.dex" || diagnostic.Class != "Ldex/Rejected;" || diagnostic.Method != "bad" || diagnostic.Offset != 0 {
		t.Fatalf("diagnostic context = %+v", diagnostic)
	}
	if !strings.Contains(diagnostic.Op, "monitor-enter") || !strings.Contains(diagnostic.Op, "0x1d") {
		t.Fatalf("diagnostic opcode = %q", diagnostic.Op)
	}
}

func TestConstructorInvokeShapesAccepted(t *testing.T) {
	tests := []struct {
		name    string
		fixture func() (*model.File, *model.Class)
	}{
		{name: "super delegation", fixture: superDelegationFixture},
		{name: "this delegation", fixture: thisDelegationFixture},
		{name: "fused allocation", fixture: fusedAllocationFixture},
		{name: "private direct method", fixture: privateDirectFixture},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, class := test.fixture()
			raw, err := Class(file, class)
			if err != nil {
				t.Fatalf("Class: %v", err)
			}
			if len(raw) == 0 {
				t.Fatal("Class returned no bytes")
			}
			if _, err := javaclassparser.Parse(raw); err != nil {
				t.Fatalf("parse generated class: %v", err)
			}
		})
	}
}

func TestThisDelegationVerifiesAndExecutes(t *testing.T) {
	file, class := thisDelegationFixture()
	raw, err := Class(file, class)
	if err != nil {
		t.Fatalf("Class: %v", err)
	}
	java, err := exec.LookPath("java")
	if err != nil {
		t.Skip("java is unavailable")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "dex", "ThisDelegation.class")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(java, "-Xverify:all", "-cp", dir, "dex.ThisDelegation").CombinedOutput(); err != nil {
		t.Fatalf("java verification/execution: %v\n%s", err, output)
	}
}

func TestInvalidConstructorReceiversReturnNoClassAndPreciseDiagnostic(t *testing.T) {
	tests := []struct {
		name        string
		fixture     func() (*model.File, *model.Class)
		method      string
		offset      int64
		op          string
		messagePart string
	}{
		{
			name:        "initialized parameter",
			fixture:     initializedParameterConstructorFixture,
			method:      "bad",
			offset:      0,
			op:          "invoke-direct(0x70)",
			messagePart: "not a fused new-instance allocation",
		},
		{
			name:        "initialized this outside constructor",
			fixture:     initializedThisConstructorFixture,
			method:      "bad",
			offset:      0,
			op:          "invoke-direct(0x70)",
			messagePart: "not a fused new-instance allocation",
		},
		{
			name:        "initialized parameter after super delegation",
			fixture:     parameterAfterDelegationFixture,
			method:      "<init>",
			offset:      3,
			op:          "invoke-direct(0x70)",
			messagePart: "is initialized; delegation requires uninitialized this",
		},
		{
			name:        "duplicate this initialization",
			fixture:     duplicateDelegationFixture,
			method:      "<init>",
			offset:      3,
			op:          "invoke-direct(0x70)",
			messagePart: "initializes this more than once",
		},
		{
			name:        "this is also delegation argument",
			fixture:     thisAsDelegationArgumentFixture,
			method:      "<init>",
			offset:      0,
			op:          "invoke-direct(0x70)",
			messagePart: "is also a constructor argument",
		},
		{
			name:        "back edge repeats delegation",
			fixture:     repeatedDelegationFixture,
			method:      "<init>",
			offset:      3,
			op:          "goto(0x28)",
			messagePart: "may repeat constructor delegation",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, class := test.fixture()
			raw, err := Class(file, class)
			if err == nil || raw != nil {
				t.Fatalf("Class = %d bytes, %v; want nil bytes and error", len(raw), err)
			}
			assertLowerDiagnostic(t, err, file.Entry, class.Descriptor, test.method, test.offset, test.op, test.messagePart)
		})
	}
}

func TestDefinedNonConstructorInvokeDirectMustBePrivateDirect(t *testing.T) {
	tests := []struct {
		name    string
		virtual bool
	}{
		{name: "public direct method"},
		{name: "virtual method", virtual: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file, class := definedDirectTargetFixture(test.virtual)
			raw, err := Class(file, class)
			if err == nil || raw != nil {
				t.Fatalf("Class = %d bytes, %v; want nil bytes and error", len(raw), err)
			}
			assertLowerDiagnostic(t, err, file.Entry, class.Descriptor, "caller", 0, "invoke-direct(0x70)", "private, non-static direct method")
		})
	}
}

func assertLowerDiagnostic(t *testing.T, err error, entry, class, method string, offset int64, op, messagePart string) {
	t.Helper()
	diagnostic, ok := report.AsDiagnostic(err)
	if !ok {
		t.Fatalf("error is not Diagnostic: %T %v", err, err)
	}
	if diagnostic.Entry != entry || diagnostic.Class != class || diagnostic.Method != method || diagnostic.Offset != offset || diagnostic.Op != op {
		t.Fatalf("diagnostic context = %+v", diagnostic)
	}
	if !strings.Contains(diagnostic.Error(), messagePart) {
		t.Fatalf("diagnostic %q does not contain %q", diagnostic.Error(), messagePart)
	}
}

func superDelegationFixture() (*model.File, *model.Class) {
	const owner = "Ldex/SuperDelegation;"
	proto := model.Proto{ReturnType: "V"}
	superInit := model.MethodID{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 0}
	constructor := model.MethodID{Class: owner, Name: "<init>", ProtoIndex: 0}
	file := &model.File{Entry: "classes.dex", Protos: []model.Proto{proto}, Methods: []model.MethodID{superInit}}
	class := &model.Class{
		Descriptor: owner, Superclass: "Ljava/lang/Object;", AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{{
			AccessFlags: dexPublic | dexConstructor, Method: constructor,
			Code: &model.Code{RegistersSize: 1, InsSize: 1, OutsSize: 1, Instructions: []uint16{0x1070, 0x0000, 0x0000, 0x000e}},
		}},
	}
	return file, class
}

func thisDelegationFixture() (*model.File, *model.Class) {
	const owner = "Ldex/ThisDelegation;"
	protos := []model.Proto{
		{ReturnType: "V"},
		{ReturnType: "V", Parameters: []string{"I"}, ParameterTypes: []uint32{0}},
		{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{1}},
	}
	superInit := model.MethodID{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 0}
	intInit := model.MethodID{Class: owner, Name: "<init>", ProtoIndex: 1}
	noArgInit := model.MethodID{Class: owner, Name: "<init>", ProtoIndex: 0}
	file := &model.File{
		Entry: "classes.dex", Types: []model.Type{{Descriptor: owner}}, Protos: protos,
		Methods: []model.MethodID{superInit, intInit, noArgInit},
	}
	class := &model.Class{
		Descriptor: owner, Superclass: "Ljava/lang/Object;", AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{
			{
				AccessFlags: dexPublic | dexConstructor, Method: noArgInit,
				Code: &model.Code{RegistersSize: 2, InsSize: 1, OutsSize: 2, Instructions: []uint16{0x7012, 0x2070, 0x0001, 0x0001, 0x000e}},
			},
			{
				AccessFlags: dexPrivate | dexConstructor, Method: intInit,
				Code: &model.Code{RegistersSize: 2, InsSize: 2, OutsSize: 1, Instructions: []uint16{0x1070, 0x0000, 0x0000, 0x000e}},
			},
			{
				AccessFlags: dexPublic | dexStatic, Method: model.MethodID{Class: owner, Name: "main", ProtoIndex: 2},
				Code: &model.Code{RegistersSize: 2, InsSize: 1, OutsSize: 1, Instructions: []uint16{0x0022, 0x0000, 0x1070, 0x0002, 0x0000, 0x000e}},
			},
		},
	}
	return file, class
}

func fusedAllocationFixture() (*model.File, *model.Class) {
	const owner = "Ldex/FusedFactory;"
	const allocated = "Ldex/Allocated;"
	protos := []model.Proto{{ReturnType: allocated}, {ReturnType: "V"}}
	allocatedInit := model.MethodID{Class: allocated, Name: "<init>", ProtoIndex: 1}
	file := &model.File{
		Entry: "classes.dex", Types: []model.Type{{Descriptor: allocated}}, Protos: protos, Methods: []model.MethodID{allocatedInit},
	}
	class := &model.Class{
		Descriptor: owner, Superclass: "Ljava/lang/Object;", AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{{
			AccessFlags: dexPublic | dexStatic, Method: model.MethodID{Class: owner, Name: "create", ProtoIndex: 0},
			Code: &model.Code{RegistersSize: 1, OutsSize: 1, Instructions: []uint16{0x0022, 0x0000, 0x1070, 0x0000, 0x0000, 0x0011}},
		}},
	}
	return file, class
}

func privateDirectFixture() (*model.File, *model.Class) {
	file, class := definedDirectTargetFixture(false)
	class.DirectMethods[0].AccessFlags = dexPrivate
	return file, class
}

func initializedParameterConstructorFixture() (*model.File, *model.Class) {
	const owner = "Ldex/InitializedParameter;"
	const target = "Ldex/Target;"
	protos := []model.Proto{{ReturnType: "V"}, {ReturnType: "V", Parameters: []string{target}, ParameterTypes: []uint32{0}}}
	file := &model.File{Entry: "classes.dex", Protos: protos, Methods: []model.MethodID{{Class: target, Name: "<init>", ProtoIndex: 0}}}
	class := &model.Class{
		Descriptor: owner, Superclass: "Ljava/lang/Object;", AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{{
			AccessFlags: dexPublic | dexStatic, Method: model.MethodID{Class: owner, Name: "bad", ProtoIndex: 1},
			Code: &model.Code{RegistersSize: 1, InsSize: 1, OutsSize: 1, Instructions: []uint16{0x1070, 0x0000, 0x0000, 0x000e}},
		}},
	}
	return file, class
}

func initializedThisConstructorFixture() (*model.File, *model.Class) {
	const owner = "Ldex/InitializedThis;"
	proto := model.Proto{ReturnType: "V"}
	file := &model.File{Entry: "classes.dex", Protos: []model.Proto{proto}, Methods: []model.MethodID{{Class: owner, Name: "<init>", ProtoIndex: 0}}}
	class := &model.Class{
		Descriptor: owner, Superclass: "Ljava/lang/Object;", AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{{
			AccessFlags: dexPublic, Method: model.MethodID{Class: owner, Name: "bad", ProtoIndex: 0},
			Code: &model.Code{RegistersSize: 1, InsSize: 1, OutsSize: 1, Instructions: []uint16{0x1070, 0x0000, 0x0000, 0x000e}},
		}},
	}
	return file, class
}

func parameterAfterDelegationFixture() (*model.File, *model.Class) {
	const owner = "Ldex/ParameterAfterDelegation;"
	protos := []model.Proto{{ReturnType: "V"}, {ReturnType: "V", Parameters: []string{owner}, ParameterTypes: []uint32{0}}}
	file := &model.File{
		Entry: "classes.dex", Protos: protos,
		Methods: []model.MethodID{{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 0}, {Class: owner, Name: "<init>", ProtoIndex: 0}},
	}
	class := &model.Class{
		Descriptor: owner, Superclass: "Ljava/lang/Object;", AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{{
			AccessFlags: dexPublic | dexConstructor, Method: model.MethodID{Class: owner, Name: "<init>", ProtoIndex: 1},
			Code: &model.Code{RegistersSize: 2, InsSize: 2, OutsSize: 1, Instructions: []uint16{0x1070, 0x0000, 0x0000, 0x1170, 0x0001, 0x0001, 0x000e}},
		}},
	}
	return file, class
}

func duplicateDelegationFixture() (*model.File, *model.Class) {
	const owner = "Ldex/DuplicateDelegation;"
	proto := model.Proto{ReturnType: "V"}
	file := &model.File{
		Entry: "classes.dex", Protos: []model.Proto{proto},
		Methods: []model.MethodID{{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 0}, {Class: owner, Name: "<init>", ProtoIndex: 0}},
	}
	class := &model.Class{
		Descriptor: owner, Superclass: "Ljava/lang/Object;", AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{{
			AccessFlags: dexPublic | dexConstructor, Method: model.MethodID{Class: owner, Name: "<init>", ProtoIndex: 0},
			Code: &model.Code{RegistersSize: 1, InsSize: 1, OutsSize: 1, Instructions: []uint16{0x1070, 0x0000, 0x0000, 0x1070, 0x0001, 0x0000, 0x000e}},
		}},
	}
	return file, class
}

func thisAsDelegationArgumentFixture() (*model.File, *model.Class) {
	const owner = "Ldex/ThisAsArgument;"
	proto := model.Proto{ReturnType: "V", Parameters: []string{owner}, ParameterTypes: []uint32{0}}
	file := &model.File{Entry: "classes.dex", Protos: []model.Proto{proto}, Methods: []model.MethodID{{Class: owner, Name: "<init>", ProtoIndex: 0}}}
	class := &model.Class{
		Descriptor: owner, Superclass: "Ljava/lang/Object;", AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{{
			AccessFlags: dexPublic | dexConstructor, Method: model.MethodID{Class: owner, Name: "<init>", ProtoIndex: 0},
			Code: &model.Code{RegistersSize: 2, InsSize: 2, OutsSize: 2, Instructions: []uint16{0x2070, 0x0000, 0x0000, 0x000e}},
		}},
	}
	return file, class
}

func repeatedDelegationFixture() (*model.File, *model.Class) {
	file, class := superDelegationFixture()
	class.Descriptor = "Ldex/RepeatedDelegation;"
	class.DirectMethods[0].Method.Class = class.Descriptor
	class.DirectMethods[0].Code.Instructions = []uint16{0x1070, 0x0000, 0x0000, 0xfd28}
	return file, class
}

func definedDirectTargetFixture(virtual bool) (*model.File, *model.Class) {
	const owner = "Ldex/DirectTarget;"
	proto := model.Proto{ReturnType: "V"}
	target := model.MethodID{Class: owner, Name: "target", ProtoIndex: 0}
	file := &model.File{Entry: "classes.dex", Protos: []model.Proto{proto}, Methods: []model.MethodID{target}}
	targetDefinition := model.EncodedMethod{
		AccessFlags: dexPublic, Method: target,
		Code: &model.Code{RegistersSize: 1, InsSize: 1, Instructions: []uint16{0x000e}},
	}
	caller := model.EncodedMethod{
		AccessFlags: dexPublic, Method: model.MethodID{Class: owner, Name: "caller", ProtoIndex: 0},
		Code: &model.Code{RegistersSize: 1, InsSize: 1, OutsSize: 1, Instructions: []uint16{0x1070, 0x0000, 0x0000, 0x000e}},
	}
	class := &model.Class{Descriptor: owner, Superclass: "Ljava/lang/Object;", AccessFlags: dexPublic}
	if virtual {
		class.DirectMethods = []model.EncodedMethod{caller}
		class.VirtualMethods = []model.EncodedMethod{targetDefinition}
	} else {
		class.DirectMethods = []model.EncodedMethod{targetDefinition, caller}
	}
	return file, class
}

func arrayCloneFixture() (*model.File, *model.Class) {
	const owner = "Ldex/ArrayClone;"
	protos := []model.Proto{
		{ReturnType: "Ljava/lang/Object;", Parameters: []string{"[I"}, ParameterTypes: []uint32{0}},
		{ReturnType: "Ljava/lang/Object;"},
		{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{1}},
	}
	methods := []model.MethodID{
		{Class: "[I", Name: "clone", ProtoIndex: 1},
		{Class: owner, Name: "cloneArray", ProtoIndex: 0},
		{Class: owner, Name: "main", ProtoIndex: 2},
	}
	file := &model.File{
		Entry:   "classes.dex",
		Types:   []model.Type{{Descriptor: "[I"}, {Descriptor: "[Ljava/lang/String;"}},
		Protos:  protos,
		Methods: methods,
	}
	class := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[1],
				Code: &model.Code{RegistersSize: 2, InsSize: 1, OutsSize: 1, Instructions: []uint16{
					0x106e, 0x0000, 0x0001,
					0x000c,
					0x0011,
				}},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[2],
				Code: &model.Code{RegistersSize: 5, InsSize: 1, OutsSize: 1, Instructions: []uint16{
					0x2312,
					0x3423, 0x0000,
					0x1071, 0x0001, 0x0004,
					0x000c,
					0x0120, 0x0000,
					0x0138, 0x0003,
					0x000e,
					0x0212,
					0x0227,
				}},
			},
		},
	}
	return file, class
}

func arrayFixture() (*model.File, *model.Class) {
	const owner = "Ldex/Arrays;"
	protos := []model.Proto{
		{ReturnType: "I"},
		{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{1}},
	}
	methods := []model.MethodID{
		{Class: owner, Name: "run", ProtoIndex: 0},
		{Class: owner, Name: "main", ProtoIndex: 1},
	}
	file := &model.File{
		Entry:   "classes.dex",
		Types:   []model.Type{{Descriptor: "[I"}, {Descriptor: "[Ljava/lang/String;"}},
		Protos:  protos,
		Methods: methods,
	}
	class := &model.Class{
		Descriptor:  owner,
		Superclass:  "Ljava/lang/Object;",
		AccessFlags: dexPublic,
		DirectMethods: []model.EncodedMethod{
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[0],
				Code: &model.Code{RegistersSize: 5, Instructions: []uint16{
					0x3312,
					0x3423, 0x0000,
					0x0112,
					0x7212,
					0x024b, 0x0104,
					0x0344, 0x0104,
					0x030f,
				}},
			},
			{
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[1],
				Code: &model.Code{RegistersSize: 4, InsSize: 1, Instructions: []uint16{
					0x0071, 0x0000, 0x0000,
					0x010a,
					0x7012,
					0x0133, 0x0003,
					0x000e,
					0x0212,
					0x0227,
				}},
			},
		},
	}
	return file, class
}

func executableFixture() (*model.File, *model.Class) {
	const owner = "Ldex/Generated;"
	protos := []model.Proto{
		{ReturnType: "V"},
		{ReturnType: "V", Parameters: []string{"I"}, ParameterTypes: []uint32{0}},
		{ReturnType: "I"},
		{ReturnType: "I", Parameters: []string{"I"}, ParameterTypes: []uint32{0}},
		{ReturnType: "V", Parameters: []string{"[Ljava/lang/String;"}, ParameterTypes: []uint32{2}},
	}
	methods := []model.MethodID{
		{Class: "Ljava/lang/Object;", Name: "<init>", ProtoIndex: 0},
		{Class: owner, Name: "<init>", ProtoIndex: 1},
		{Class: owner, Name: "value", ProtoIndex: 2},
		{Class: owner, Name: "choose", ProtoIndex: 3},
		{Class: owner, Name: "run", ProtoIndex: 2},
		{Class: owner, Name: "main", ProtoIndex: 4},
	}
	field := model.FieldID{Class: owner, Name: "value", Type: "I"}
	file := &model.File{
		Entry:   "classes.dex",
		Types:   []model.Type{{Descriptor: owner}, {Descriptor: "Ljava/lang/Object;"}, {Descriptor: "[Ljava/lang/String;"}},
		Protos:  protos,
		Fields:  []model.FieldID{field},
		Methods: methods,
	}
	class := &model.Class{
		Descriptor:     owner,
		Superclass:     "Ljava/lang/Object;",
		AccessFlags:    dexPublic,
		InstanceFields: []model.EncodedField{{FieldIndex: 0, AccessFlags: dexPrivate, Field: field}},
		DirectMethods: []model.EncodedMethod{
			{
				MethodIndex: 1,
				AccessFlags: dexPublic | dexConstructor,
				Method:      methods[1],
				Code: &model.Code{RegistersSize: 2, InsSize: 2, OutsSize: 1, Instructions: []uint16{
					0x1070, 0x0000, 0x0000,
					0x0159, 0x0000,
					0x000e,
				}},
			},
			{
				MethodIndex: 3,
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[3],
				Code: &model.Code{RegistersSize: 3, InsSize: 1, Instructions: []uint16{
					0x2012,
					0x0190, 0x0002,
					0x013d, 0x0006,
					0x1012,
					0x0190, 0x0001,
					0x010f,
					0xf112,
					0x010f,
				}},
			},
			{
				MethodIndex: 4,
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[4],
				Code: &model.Code{RegistersSize: 3, OutsSize: 2, Instructions: []uint16{
					0x7112,
					0x0022, 0x0000,
					0x2070, 0x0001, 0x0010,
					0x106e, 0x0002, 0x0000,
					0x020a,
					0x020f,
				}},
			},
			{
				MethodIndex: 5,
				AccessFlags: dexPublic | dexStatic,
				Method:      methods[5],
				Code: &model.Code{RegistersSize: 4, InsSize: 1, OutsSize: 1, Instructions: []uint16{
					0x4012,
					0x1071, 0x0003, 0x0000,
					0x010a,
					0x7012,
					0x0133, 0x000a,
					0x0071, 0x0004, 0x0000,
					0x010a,
					0x7012,
					0x0133, 0x0003,
					0x000e,
					0x0212,
					0x0227,
				}},
			},
		},
		VirtualMethods: []model.EncodedMethod{{
			MethodIndex: 2,
			AccessFlags: dexPublic,
			Method:      methods[2],
			Code: &model.Code{RegistersSize: 2, InsSize: 1, Instructions: []uint16{
				0x1052, 0x0000,
				0x000f,
			}},
		}},
	}
	return file, class
}
