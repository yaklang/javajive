package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	coreutils "github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func lambdaParameterTestOriginal(t *testing.T) []byte {
	t.Helper()
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for original implementation parameter tables")
	}
	return compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, uncapturedLambdaParameterFixture, "UncapturedLambdaNames", "none")["UncapturedLambdaNames.class"]
}

func lambdaParameterTestDumper(t *testing.T, raw []byte, selected ...string) (*ClassObjectDumper, *MemberInfo, *UnparsedAttribute, *CodeAttribute) {
	t.Helper()
	object, err := Parse(bytes.Clone(raw))
	if err != nil {
		t.Fatal(err)
	}
	d := NewClassObjectDumper(object)
	d.options = DecompileOptions{SourceCompiler: NativeJavac8, TargetSourceVersion: 8}
	want := "(II)I"
	if len(selected) == 1 {
		want = selected[0]
	}
	for _, method := range object.Methods {
		descriptor, _ := sourceBridgeUTF8(object, method.DescriptorIndex)
		if method.AccessFlags != 0x100a {
			continue
		}
		if want == "with-parent-metadata" {
			p, known := d.originalLambdaParameterMetadata(method, nil)
			if !known || p == nil || len(p.parentNames) == 0 {
				continue
			}
		} else if descriptor != want {
			continue
		}
		var table *UnparsedAttribute
		var code *CodeAttribute
		for _, attribute := range method.Attributes {
			switch a := attribute.(type) {
			case *CodeAttribute:
				code = a
			case *UnparsedAttribute:
				if a.Name == "MethodParameters" {
					table = a
				}
			}
		}
		if table == nil || code == nil {
			t.Fatal("original compiler did not emit the expected MP/Code packets")
		}
		return d, method, table, code
	}
	t.Fatal("missing original binary implementation")
	return nil, nil, nil, nil
}

func TestAdversarialUncapturedLambdaParameterMetadataRequiresOriginalSchemaAndFactory(t *testing.T) {
	raw := lambdaParameterTestOriginal(t)
	for _, variant := range []string{"original", "final SAM", "modern profile", "source release", "old major", "new major", "public", "instance", "nonsynthetic", "synchronized", "owner interface kind", "missing MP", "duplicate MP", "MP length", "MP count", "unnamed", "duplicate name", "lambda underscore", "missing synthetic bit", "mandated bit", "unknown bit", "unknown name CP", "unknown attribute", "duplicate declaration", "detached declaration", "missing Code", "entry write", "wide entry word write", "parent name collision", "implementation spelling", "shared factory", "factory capture", "factory handle kind", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			descriptor := "(II)I"
			if variant == "wide entry word write" {
				descriptor = "(JJ)J"
			} else if variant == "parent name collision" {
				descriptor = "with-parent-metadata"
			}
			d, method, table, code := lambdaParameterTestDumper(t, raw, descriptor)
			original, known := d.originalLambdaParameterMetadata(method, nil)
			if !known || original == nil || original.factory == nil {
				t.Fatal("independent original schema/factory")
			}
			switch variant {
			case "final SAM":
				binary.BigEndian.PutUint16(table.Info[3:5], 0x1010)
			case "modern profile":
				d.options.SourceCompiler = ModernJavac
			case "source release":
				d.options.TargetSourceVersion = 17
			case "old major":
				d.obj.MajorVersion = 51
			case "new major":
				d.obj.MajorVersion = 53
			case "public":
				method.AccessFlags = 0x1009
			case "instance":
				method.AccessFlags = 0x1002
			case "nonsynthetic":
				method.AccessFlags = 0x000a
			case "synchronized":
				method.AccessFlags |= 0x20
			case "owner interface kind":
				d.obj.AccessFlags ^= 0x0200
			case "missing MP", "missing Code":
				for i, attribute := range method.Attributes {
					if variant == "missing MP" && attribute == table || variant == "missing Code" && attribute == code {
						method.Attributes = append(method.Attributes[:i], method.Attributes[i+1:]...)
						break
					}
				}
			case "duplicate MP":
				method.Attributes = append(method.Attributes, table)
			case "MP length":
				table.Length++
			case "MP count":
				table.Info[0]++
			case "unnamed":
				binary.BigEndian.PutUint16(table.Info[1:3], 0)
			case "duplicate name":
				copy(table.Info[5:7], table.Info[1:3])
			case "lambda underscore":
				binary.BigEndian.PutUint16(table.Info[1:3], sourceBridgePoolString(t, d.obj, "_"))
			case "missing synthetic bit":
				binary.BigEndian.PutUint16(table.Info[3:5], 0)
			case "mandated bit":
				binary.BigEndian.PutUint16(table.Info[3:5], 0x9000)
			case "unknown bit":
				binary.BigEndian.PutUint16(table.Info[3:5], 0x1020)
			case "unknown name CP":
				index := binary.BigEndian.Uint16(table.Info[1:3])
				d.obj.ConstantPool[index-1] = &ConstantIntegerInfo{Value: 1}
			case "unknown attribute":
				method.Attributes = append(method.Attributes, &UnparsedAttribute{Name: "RuntimeInvisibleAnnotations", Length: 2, Info: []byte{0, 0}})
			case "duplicate declaration":
				copy := *method
				d.obj.Methods = append(d.obj.Methods, &copy)
			case "detached declaration":
				copy := *method
				method = &copy
			case "entry write":
				code.Code = append([]byte{0x84, 0, 1}, code.Code...) // iinc physical parameter word zero
			case "wide entry word write":
				code.Code = append([]byte{0x84, 3, 1}, code.Code...) // second word of the second long
			case "parent name collision":
				for _, attribute := range original.factory.method.Attributes {
					if parent, ok := attribute.(*UnparsedAttribute); ok && parent.Name == "MethodParameters" {
						copy(parent.Info[1:3], table.Info[1:3])
					}
				}
			case "implementation spelling":
				before := method.NameIndex
				name := sourceBridgePoolString(t, d.obj, "ordinaryImplementation")
				method.NameIndex = name
				for _, constant := range d.obj.ConstantPool {
					if nt, ok := constant.(*ConstantNameAndTypeInfo); ok && nt.NameIndex == before {
						nt.NameIndex = name
					}
				}
			case "shared factory":
				factory := original.factory.code
				if len(factory.Code) != 6 || factory.Code[0] != 0xba || factory.Code[5] != 0xb0 {
					t.Fatal("one original zero-capture factory followed by areturn")
				}
				factory.Code = append(append(append([]byte(nil), factory.Code[:5]...), 0x57), factory.Code...)
			case "factory capture":
				pc := original.factory.pc
				index := binary.BigEndian.Uint16(original.factory.code.Code[pc+1 : pc+3])
				dynamic := d.obj.ConstantPool[index-1].(*ConstantInvokeDynamicInfo)
				nt := d.obj.ConstantPool[dynamic.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
				nt.DescriptorIndex = sourceBridgePoolString(t, d.obj, "(I)Ljava/util/function/IntBinaryOperator;")
			case "factory handle kind":
				name, _ := sourceBridgeUTF8(d.obj, method.NameIndex)
				for _, constant := range d.obj.ConstantPool {
					if handle, ok := constant.(*ConstantMethodHandleInfo); ok {
						if target, valid := nativeLambdaOriginalHandleTarget(d.obj, handle); valid && target.name == name {
							handle.ReferenceKind = 5
						}
					}
				}
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1, MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := d.obj.Bytes()
			p, known := d.originalLambdaParameterMetadata(method, d.Work)
			want := variant == "original" || variant == "final SAM"
			if (known && p != nil) != want {
				t.Fatalf("projection=%v known=%v want=%v", p != nil, known, want)
			}
			if !bytes.Equal(before, d.obj.Bytes()) {
				t.Fatal("metadata/source query changed original input")
			}
		})
	}
}

func TestAdversarialUncapturedLambdaParameterBindingRequiresEntryAndCallbackIdentities(t *testing.T) {
	raw := lambdaParameterTestOriginal(t)
	for _, variant := range []string{"original", "missing factory", "wrong factory PC", "foreign Code", "foreign method", "foreign object", "captured request", "ordinary request", "missing entry", "wrong slot", "wrong type", "not parameter", "receiver", "duplicate identity", "missing formal", "prior name", "stale factory Code", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, method, _, _ := lambdaParameterTestDumper(t, raw)
			p, known := d.originalLambdaParameterMetadata(method, nil)
			if !known || p == nil {
				t.Fatal("original metadata projection")
			}
			d.CurrentMethod = method
			d.FuncCtx = &class_context.ClassContext{ClassName: d.obj.GetClassName(), FunctionName: p.name, CurrentMethodDesc: p.descriptor, IsStatic: true, LocalNames: map[*coreutils.VariableId]string{}}
			var parameters []values.JavaValue
			for i := range p.parameters {
				ref := values.NewJavaRef(coreutils.NewRootVariableId(), nil, types.NewJavaPrimer(types.JavaInteger))
				ref.IsParam = true
				ref.MarkOriginalParameter(i)
				parameters = append(parameters, ref)
			}
			request, known := d.newLambdaMethodSourceRequest(0)
			if !known {
				t.Fatal("fresh actual callback")
			}
			request.factory = &lambdaSourceFactory{object: d.obj, method: p.factory.method, code: p.factory.code, pc: p.factory.pc}
			first := parameters[0].(*values.JavaRef)
			switch variant {
			case "missing factory":
				request.factory = nil
			case "wrong factory PC":
				request.factory.pc++
			case "foreign Code":
				copy := *request.factory.code
				request.factory.code = &copy
			case "foreign method":
				request.factory.method = d.obj.Methods[0]
			case "foreign object":
				copy := *d.obj
				request.factory.object = &copy
			case "captured request":
				request.captureCount = 1
			case "ordinary request":
				request.role = methodSourceDeclaration
			case "missing entry":
				ref := values.NewJavaRef(first.Id, nil, first.Type())
				ref.IsParam = true
				parameters[0] = ref
			case "wrong slot":
				ref := values.NewJavaRef(first.Id, nil, first.Type())
				ref.IsParam = true
				ref.MarkOriginalParameter(7)
				parameters[0] = ref
			case "wrong type":
				ref := values.NewJavaRef(first.Id, nil, types.NewJavaClass("java.lang.Object"))
				ref.IsParam = true
				ref.MarkOriginalParameter(0)
				parameters[0] = ref
			case "not parameter":
				first.IsParam = false
			case "receiver":
				first.IsThis = true
			case "duplicate identity":
				parameters[1].(*values.JavaRef).Id = first.Id
			case "missing formal":
				parameters = parameters[:1]
			case "prior name":
				d.FuncCtx.LocalNames[first.Id] = "other"
			case "stale factory Code":
				p.factory.code.Code[0] = 0
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := make([]string, len(parameters))
			for i, parameter := range parameters {
				before[i] = parameter.String(d.FuncCtx)
			}
			binding, known := d.bindLambdaParameterMetadata(p, request, parameters)
			if (known && binding != nil) != (variant == "original") {
				t.Fatalf("bound=%v for %s", known, variant)
			}
			if !known {
				for i, parameter := range parameters {
					if parameter.String(d.FuncCtx) != before[i] {
						t.Fatal("failed proof partially renamed an entry")
					}
				}
			} else if !binding.parametersClosed(d.FuncCtx, nil) {
				t.Fatal("bound entries no longer match original names/descriptors")
			}
			if d.nativeMemberRoot != nil || d.nativeAnonymousRoot != nil || d.FuncCtx.SourceCaptureStable != nil {
				t.Fatal("parameter metadata granted source ownership or allocation permission")
			}
		})
	}
}

func TestAdversarialUncapturedLambdaParameterCacheReopensMetadataCodeAndFactory(t *testing.T) {
	raw := lambdaParameterTestOriginal(t)
	for _, variant := range []string{"original", "MP name", "MP final", "MP pointer", "remove MP", "unrelated parameter table", "target Code", "target limits", "target handlers", "target Code pointer", "class pointer", "major", "method flags", "getter pool", "factory Code", "factory Code pointer", "factory member pointer", "factory method flags", "parent name", "modern profile", "source release", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			selected := "(II)I"
			if variant == "parent name" {
				selected = "with-parent-metadata"
			}
			d, method, table, code := lambdaParameterTestDumper(t, raw, selected)
			p, known := d.originalLambdaParameterMetadata(method, nil)
			if !known || p == nil {
				t.Fatal("original source projection")
			}
			stamp, known := d.newMethodParameterNameCacheStamp(method, p.descriptor, p.names)
			if !known || stamp == nil {
				t.Fatal("original source stamp")
			}
			stamp.lambdaParameters, stamp.lambdaFactory = true, p.factory
			stamp.lambdaParentNames = append([]string(nil), p.parentNames...)
			if !d.methodParameterNameCacheClosed(stamp) {
				t.Fatal("fresh original metadata/code/factory stamp")
			}
			switch variant {
			case "MP name":
				binary.BigEndian.PutUint16(table.Info[1:3], sourceBridgePoolString(t, d.obj, "changedEntry"))
			case "MP final":
				binary.BigEndian.PutUint16(table.Info[3:5], 0x1010)
			case "MP pointer", "target Code pointer":
				for i, attribute := range method.Attributes {
					if variant == "MP pointer" && attribute == table {
						copy := *table
						method.Attributes[i] = &copy
					} else if variant == "target Code pointer" && attribute == code {
						copy := *code
						method.Attributes[i] = &copy
					}
				}
			case "remove MP":
				for i, attribute := range method.Attributes {
					if attribute == table {
						method.Attributes = append(method.Attributes[:i], method.Attributes[i+1:]...)
						break
					}
				}
			case "unrelated parameter table":
				ordinary, metadata := lambdaParameterUnrelatedOrdinaryTable(t, d.obj)
				removeLambdaParameterTestTable(ordinary, metadata)
			case "target Code":
				code.Code[0] = 0x03
			case "target limits":
				code.MaxLocals++
			case "target handlers":
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{})
			case "class pointer":
				copy := *d.obj
				d.obj = &copy
			case "major":
				d.obj.MajorVersion++
			case "method flags":
				method.AccessFlags |= 0x80
			case "getter pool":
				d.ConstantPool = append([]ConstantInfo(nil), d.ConstantPool...)
				index := binary.BigEndian.Uint16(table.Info[1:3])
				d.ConstantPool[index-1] = &ConstantIntegerInfo{Value: 1}
			case "factory Code":
				p.factory.code.Code[p.factory.pc] = 0
			case "factory Code pointer":
				for i, attribute := range p.factory.method.Attributes {
					if attribute == p.factory.code {
						copy := *p.factory.code
						p.factory.method.Attributes[i] = &copy
					}
				}
			case "factory member pointer":
				for i, member := range d.obj.Methods {
					if member == p.factory.method {
						copy := *member
						d.obj.Methods[i] = &copy
					}
				}
			case "factory method flags":
				p.factory.method.AccessFlags ^= 0x20
			case "parent name":
				for _, attribute := range p.factory.method.Attributes {
					if parent, ok := attribute.(*UnparsedAttribute); ok && parent.Name == "MethodParameters" {
						binary.BigEndian.PutUint16(parent.Info[1:3], sourceBridgePoolString(t, d.obj, "changedParent"))
					}
				}
			case "modern profile":
				d.options.SourceCompiler = ModernJavac
			case "source release":
				d.options.TargetSourceVersion = 17
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := d.methodParameterNameCacheClosed(stamp); got != (variant == "original") {
				t.Fatalf("cached metadata source accepted=%v for %s", got, variant)
			}
		})
	}
}

func TestAdversarialUncapturedLambdaParameterCompilerFlagRequiresWholeClass(t *testing.T) {
	raw := lambdaParameterTestOriginal(t)
	for _, variant := range []string{"original", "missing table", "duplicate table", "length", "count", "unnamed", "unknown name CP", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			d, implementation, _, _ := lambdaParameterTestDumper(t, raw)
			ordinary, table := lambdaParameterUnrelatedOrdinaryTable(t, d.obj)
			switch variant {
			case "missing table":
				removeLambdaParameterTestTable(ordinary, table)
			case "duplicate table":
				ordinary.Attributes = append(ordinary.Attributes, table)
			case "length":
				table.Length++
			case "count":
				table.Info[0]++
			case "unnamed":
				binary.BigEndian.PutUint16(table.Info[1:3], 0)
			case "unknown name CP":
				index := binary.BigEndian.Uint16(table.Info[1:3])
				d.obj.ConstantPool[index-1] = &ConstantIntegerInfo{Value: 1}
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			before := d.obj.Bytes()
			projection, known := d.originalLambdaParameterMetadata(implementation, d.Work)
			if (known && projection != nil) != (variant == "original") {
				t.Fatalf("class-wide compiler flag accepted=%v for %s", known, variant)
			}
			if !bytes.Equal(before, d.obj.Bytes()) {
				t.Fatal("class-wide metadata consistency query changed original input")
			}
		})
	}
}
