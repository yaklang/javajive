package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAnonymousNamedFamilyRequiresCompleteReciprocalOriginalOwnership(t *testing.T) {
	base := nativeCompileClasses(t, anonymousMemberSuperIdentityFixture)
	for _, variant := range []string{"original", "missing child", "foreign child identity", "duplicate parent row", "missing self row", "different self owner", "capture flags", "wrong constructor receiver", "wrong capture parameter", "small stack", "small locals", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for name, raw := range base {
				files[name] = append([]byte(nil), raw...)
			}
			path := "OriginalNamespace$1$Derived.class"
			if variant == "duplicate parent row" {
				path = "OriginalNamespace$1.class"
			}
			object, err := Parse(files[path])
			if err != nil {
				t.Fatal(err)
			}
			cp := NewConstantPoolWithConstant(&object.ConstantPool)
			switch variant {
			case "missing child":
				delete(files, path)
			case "foreign child identity":
				object.ThisClass = uint16(cp.AddNewClassInfo("Foreign"))
			case "duplicate parent row", "missing self row", "different self owner":
				for _, attribute := range object.Attributes {
					if table, ok := attribute.(*InnerClassesAttribute); ok {
						for i, row := range table.Classes {
							name, _ := sourceBridgeClassName(object, row.InnerClassInfoIndex)
							if name != "OriginalNamespace$1$Derived" {
								continue
							}
							switch variant {
							case "duplicate parent row":
								table.Classes = append(table.Classes, row)
							case "missing self row":
								table.Classes = append(table.Classes[:i], table.Classes[i+1:]...)
							case "different self owner":
								row.OuterClassInfoIndex = uint16(cp.AddNewClassInfo("OriginalNamespace"))
							}
							break
						}
					}
				}
			case "capture flags":
				for _, field := range object.Fields {
					if field.AccessFlags&0x1000 != 0 {
						field.AccessFlags &^= 0x1000
					}
				}
			case "wrong constructor receiver", "wrong capture parameter", "small stack", "small locals":
				for _, method := range object.Methods {
					name, _ := sourceBridgeUTF8(object, method.NameIndex)
					if name != "<init>" {
						continue
					}
					for _, attribute := range method.Attributes {
						if code, ok := attribute.(*CodeAttribute); ok {
							switch variant {
							case "wrong constructor receiver":
								code.Code[0] = byte(core.OP_ALOAD_1)
							case "wrong capture parameter":
								code.Code[1] = byte(core.OP_ALOAD_0)
							case "small stack":
								code.MaxStack = 1
							case "small locals":
								code.MaxLocals = 1
							}
						}
					}
				}
			}
			if variant != "missing child" {
				files[path] = object.Bytes()
			}
			archive := nativeArchive(t, files)
			defer archive.Close()
			root, _ := Parse(files["OriginalNamespace.class"])
			reader := archive.nativeMemberReader(root)
			family := reader.planNativeMemberFamily()
			if family == nil || len(family.children) != 1 {
				t.Fatal("original named anchor")
			}
			states := map[*nativeMemberClass]nativeMemberClass{}
			constructors := map[*nativeMemberConstructor]nativeMemberConstructor{}
			for _, child := range family.children {
				states[child] = *child
				for _, ctor := range child.constructors {
					constructors[ctor] = *ctor
				}
			}
			if variant == "budget" {
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			if variant == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			known := reader.planNativeMemberAnonymousScopes(family)
			if known != (variant == "original") {
				t.Fatalf("mixed original ownership %v", known)
			}
			if !known && (len(family.children) != 1 || len(family.lexicalObjects) != 2 || len(family.anonymousUnits) != 0) {
				t.Fatal("failed mixed transaction retained partial source ownership")
			}
			if !known {
				for child, state := range states {
					if family.children[state.object.GetClassName()] != child || child.object != state.object || child.sourceName != state.sourceName || child.sourceAnonymousOwner != state.sourceAnonymousOwner {
						t.Fatal("failed mixed transaction changed an original named packet")
					}
				}
				for ctor, state := range constructors {
					if *ctor != state {
						t.Fatal("failed mixed transaction changed an original constructor certificate")
					}
				}
			}
		})
	}
}

func TestNativeAnonymousNamedEnclosingDeclarationRequiresOriginalMemberIdentity(t *testing.T) {
	files := nativeCompileClasses(t, anonymousMemberSuperIdentityFixture)
	for _, variant := range []string{"original field", "original constructor", "foreign same fields", "different object", "missing anonymous owner", "wrong descriptor", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			archive := nativeArchive(t, files)
			defer archive.Close()
			root, _ := Parse(files["OriginalNamespace.class"])
			reader := archive.nativeMemberReader(root)
			family := reader.planNativeMemberFamily()
			if family == nil || !reader.planNativeMemberAnonymousScopes(family) {
				t.Fatal("original mixed forest")
			}
			forest := family.anonymousForest
			object := forest.objects["OriginalNamespace$1$Derived"]
			member := object.Fields[0]
			if variant == "original constructor" {
				for _, method := range object.Methods {
					name, _ := sourceBridgeUTF8(object, method.NameIndex)
					if name == "<init>" {
						member = method
					}
				}
			}
			var work *workbudget.Budget
			switch variant {
			case "foreign same fields":
				copy := *member
				member = &copy
			case "different object":
				object, _ = Parse(files["OriginalNamespace$1$Derived.class"])
				member = object.Fields[0]
			case "missing anonymous owner":
				delete(forest.units, "OriginalNamespace$1")
			case "wrong descriptor":
				member.DescriptorIndex = member.NameIndex
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := variant == "original field" || variant == "original constructor"
			if nativeAnonymousForestNamedEnclosingDeclaration(forest, object, member, work) != want {
				t.Fatal("borrowed an original anonymous enclosing declaration")
			}
		})
	}
}

func TestNativeAnonymousNamedConstructorNameTypeRequiresEveryOriginalReference(t *testing.T) {
	files := nativeCompileClasses(t, anonymousMemberSuperIdentityFixture)
	for _, variant := range []string{"original", "different caller object", "missing owner", "foreign owner object", "missing constructor", "wrong member kind", "foreign reference reuse", "invokedynamic reuse", "dynamic reuse", "zero index", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			archive := nativeArchive(t, files)
			defer archive.Close()
			root, _ := Parse(files["OriginalNamespace.class"])
			reader := archive.nativeMemberReader(root)
			family := reader.planNativeMemberFamily()
			if family == nil || !reader.planNativeMemberAnonymousScopes(family) {
				t.Fatal("original mixed forest")
			}
			forest := family.anonymousForest
			object := forest.objects["OriginalNamespace$1"]
			index, referenceIndex := 0, 0
			var reference *ConstantMemberrefInfo
			for i, constant := range object.ConstantPool {
				ref := nativeConstantMember(constant)
				if ref == nil {
					continue
				}
				owner, _ := sourceBridgeClassName(object, ref.ClassIndex)
				if owner == "OriginalNamespace$1$Derived" {
					index, referenceIndex, reference = int(ref.NameAndTypeIndex), i, ref
					break
				}
			}
			if index == 0 {
				t.Fatal("original named allocation reference")
			}
			var work *workbudget.Budget
			switch variant {
			case "different caller object":
				object, _ = Parse(files["OriginalNamespace$1.class"])
			case "missing owner":
				delete(forest.units, "OriginalNamespace$1")
			case "foreign owner object":
				foreign, _ := Parse(files["OriginalNamespace$1.class"])
				forest.units["OriginalNamespace$1"].object = foreign
			case "missing constructor":
				forest.members.children["OriginalNamespace$1$Derived"].constructors = nil
			case "wrong member kind":
				object.ConstantPool[referenceIndex] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: *reference}
			case "foreign reference reuse":
				cp := NewConstantPoolWithConstant(&object.ConstantPool)
				foreign := *reference
				foreign.ClassIndex = uint16(cp.AddNewClassInfo("Foreign"))
				object.ConstantPool = append(object.ConstantPool, &ConstantMethodrefInfo{ConstantMemberrefInfo: foreign})
			case "invokedynamic reuse":
				object.ConstantPool = append(object.ConstantPool, &ConstantInvokeDynamicInfo{NameAndTypeIndex: uint16(index)})
			case "dynamic reuse":
				object.ConstantPool = append(object.ConstantPool, &ConstantDynamicInfo{NameAndTypeIndex: uint16(index)})
			case "zero index":
				index = 0
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if nativeAnonymousForestNamedEnclosingNameType(forest, object, index, work) != (variant == "original") {
				t.Fatal("an unrelated reference borrowed anonymous enclosing ownership")
			}
		})
	}
}

func TestNativeAnonymousNamedRelativeSpellingStaysInOriginalAnonymousScope(t *testing.T) {
	files := nativeCompileClasses(t, anonymousMemberSuperIdentityFixture)
	archive := nativeArchive(t, files)
	defer archive.Close()
	root, _ := Parse(files["OriginalNamespace.class"])
	reader := archive.nativeMemberReader(root)
	family := reader.planNativeMemberFamily()
	if family == nil || !reader.planNativeMemberAnonymousScopes(family) {
		t.Fatal("original mixed forest")
	}
	name := "OriginalNamespace$1$Derived"
	anchor := family.anonymousNamedAnchor(name)
	if anchor != "OriginalNamespace$1" {
		t.Fatal("original anonymous source anchor")
	}
	for _, caller := range []string{anchor, name, "OriginalNamespace", "OriginalNamespace$Entry", "Foreign"} {
		want := caller == anchor || caller == name
		if family.anonymousNamedScopeContains(anchor, caller) != want {
			t.Fatalf("relative name scope for %s", caller)
		}
	}
	if source, known := family.sourceName(name); !known || source != "Derived" {
		t.Fatal("named child did not retain relative anonymous scope")
	}
	// A dependency view retains its source anchor but grants no local caller
	// an ownership path into the original anonymous expression.
	foreign := &nativeMemberFamily{children: map[string]*nativeMemberClass{name: family.children[name]}, lexicalObjects: map[string]*ClassObject{"OriginalNamespace": root}}
	if foreign.anonymousNamedAnchor(name) != anchor || foreign.anonymousNamedScopeContains(anchor, "OriginalNamespace") || foreign.anonymousNamedScopeContains(anchor, anchor) {
		t.Fatal("a foreign source alias borrowed anonymous lexical scope")
	}
}

func TestNativeAnonymousNamedEmptyRootAdmissionIsIndependentOfReadOrder(t *testing.T) {
	files := nativeCompileClasses(t, anonymousNamedWithoutRootAnchorFixture())
	for _, first := range []string{"OriginalNamespace", "OriginalNamespace$1", "OriginalNamespace$1$Derived"} {
		t.Run(first, func(t *testing.T) {
			archive := nativeArchive(t, files)
			defer archive.Close()
			root, _ := Parse(files["OriginalNamespace.class"])
			reader := archive.nativeMemberReader(root)
			family := reader.planNativeMemberFamily()
			if family == nil || len(family.children) != 0 || len(family.lexicalObjects) != 1 {
				t.Fatal("discovery granted ownership before the mixed proof")
			}
			object, _ := Parse(files[first+".class"])
			entry := archive.nativeMemberEntry(object)
			if entry == nil || entry.family == nil || len(entry.family.children) != 1 || len(entry.family.anonymousUnits) != 1 || entry.family.anonymousForest == nil {
				t.Fatal("complete mixed source family did not publish")
			}
			again := archive.nativeMemberEntry(root)
			if again != entry {
				t.Fatal("one original family acquired separate source ownership")
			}
		})
	}
	for _, variant := range []string{"missing child", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			input := map[string][]byte{}
			for name, raw := range files {
				input[name] = raw
			}
			if variant == "missing child" {
				delete(input, "OriginalNamespace$1$Derived.class")
			}
			archive := nativeArchive(t, input)
			defer archive.Close()
			root, _ := Parse(input["OriginalNamespace.class"])
			reader := archive.nativeMemberReader(root)
			if variant == "budget" {
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			}
			if variant == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			family := reader.planNativeMemberFamily()
			if family != nil && reader.planNativeMemberAnonymousScopes(family) {
				t.Fatal("incomplete original evidence acquired mixed source ownership")
			}
		})
	}
}

func TestNativeAnonymousNamedSuperSourceRequiresOriginalEnclosingOperand(t *testing.T) {
	files := nativeCompileClasses(t, anonymousMemberSuperIdentityFixture)
	for _, variant := range []string{"original", "effectful outer", "null outer", "foreign parameter", "THIS receiver", "missing binding", "wrong PC"} {
		t.Run(variant, func(t *testing.T) {
			archive := nativeArchive(t, files)
			defer archive.Close()
			root, _ := Parse(files["OriginalNamespace.class"])
			d := archive.nativeMemberReader(root)
			family := d.planNativeMemberFamily()
			if family == nil || !d.planNativeMemberAnonymousScopes(family) {
				t.Fatal("original mixed family")
			}
			child := family.children["OriginalNamespace$1$Derived"]
			var ctor *nativeMemberConstructor
			for _, packet := range child.constructors {
				ctor = packet
			}
			if ctor == nil || !ctor.projectedSuper || ctor.enclosingSuperPath != nil {
				t.Fatal("original unchanged parameter SUPER widening")
			}
			id := &utils.VariableId{}
			ctx := &class_context.ClassContext{ClassName: child.object.GetClassName(), LocalNames: map[*utils.VariableId]string{id: "OriginalNamespace$1.this"}}
			outer := values.NewJavaRef(id, nil, types.NewJavaClass(child.owner))
			outer.IsParam = true
			reader := archive.nativeMemberReader(child.object)
			reader.FuncCtx = ctx
			reader.nativeMemberRoot, reader.nativeMemberCurrent = family, child
			reader.wireNativeMemberSource()
			ctx.FunctionName, ctx.CurrentMethodDesc = "<init>", ctor.descriptor
			args := []any{outer, values.NewJavaLiteral("null", types.NewJavaClass("java.lang.Object"))}
			pc := ctor.delegatePC
			switch variant {
			case "effectful outer":
				args[0] = values.NewCustomValue(func(*class_context.ClassContext) string { panic("removed enclosing operand must never render") }, func() types.JavaType { return types.NewJavaClass(child.owner) })
			case "null outer":
				args[0] = values.NewJavaLiteral("null", types.NewJavaClass(child.owner))
			case "foreign parameter":
				ref := values.NewJavaRef(&utils.VariableId{}, nil, types.NewJavaClass(child.owner))
				ref.IsParam = true
				args[0] = ref
			case "THIS receiver":
				outer.IsThis = true
			case "missing binding":
				ctx.LocalNames = nil
			case "wrong PC":
				pc++
			}
			if source, known := ctx.SourceMemberDelegation(ctor.delegateOwner, ctor.delegateDescriptor, pc, args); known != (variant == "original") {
				t.Fatalf("hidden SUPER enclosing operand %q admitted=%v", source, known)
			}
		})
	}
}
