package javaclassparser

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeRootBridgeDelegationRequiresOriginalThisAndUnusedMarker(t *testing.T) {
	files := nativeCompileClasses(t, nativeRootPrivateConstructorFixture)
	for _, variant := range []string{"original", "foreign receiver", "non-null marker", "computed marker", "handler crosses delegation", "wrong PC", "wrong descriptor", "wrong owner", "missing plan", "foreign caller", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(append([]byte(nil), files["RootBridgePacket.class"]...))
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || len(p.rootAccessBridges) != 1 || p.children[p.owner] != nil {
				t.Fatal("real root/member separation")
			}
			child := p.children["RootBridgePacket$Member"]
			key := nativeRootBridgeDelegationKey(child.object.GetClassName(), "(Ljava/lang/Object;)V")
			plan := p.rootBridgeDelegations[key]
			if plan == nil {
				t.Fatal("original delegation")
			}
			var code *CodeAttribute
			for _, m := range child.object.Methods {
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						code = c
					}
				}
			}
			if code == nil {
				t.Fatal("original code")
			}
			switch variant {
			case "foreign receiver":
				code.Code[0] = byte(core.OP_ALOAD_1)
			case "non-null marker":
				code.Code[plan.pc-1] = byte(core.OP_ALOAD_1)
			case "computed marker":
				code.Code = append(code.Code[:plan.pc], append([]byte{byte(core.OP_CHECKCAST), byte(root.ThisClass >> 8), byte(root.ThisClass)}, code.Code[plan.pc:]...)...)
			case "handler crosses delegation":
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{StartPc: 0, EndPc: uint16(plan.pc + 3), HandlerPc: uint16(plan.pc + 3)})
			case "wrong PC":
				plan.pc++
			case "wrong descriptor":
				plan.descriptor = "()V"
			case "wrong owner":
				plan.owner = "Foreign"
			case "missing plan":
				delete(p.rootBridgeDelegations, key)
			case "foreign caller":
				child.object.ThisClass = child.object.SuperClass
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				d.Work.Charge(workbudget.CounterGraphScans, 1)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if variant == "foreign receiver" || variant == "non-null marker" || variant == "computed marker" || variant == "handler crosses delegation" {
				// Rebuild the proof from the changed packet, rather than treating
				// a previously recorded PC as authority for different bytecode.
				if !d.proveNativeRootBridgeDelegations(p) {
					return
				}
			}
			allocations, known := z.nativeMemberReader(child.object).nativeMemberAllocations(p)
			closed := known && nativeMemberJointBridgeCallersClosed(p, child.object, allocations, d.Work)
			if closed != (variant == "original") {
				t.Fatalf("caller closure=%v", closed)
			}
		})
	}
}

func TestNativeRootBridgeDeclarationAndSymbolicReferencesAreExact(t *testing.T) {
	files := nativeCompileClasses(t, nativeRootPrivateConstructorFixture)
	for _, variant := range []string{"original", "copied declaration", "changed delegate", "missing bridge", "wrong marker", "method handle", "field reference", "interface reference", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(append([]byte(nil), files["RootBridgePacket.class"]...))
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			var bridge *nativeConstructorAccessBridge
			for _, b := range p.rootAccessBridges {
				bridge = b
			}
			if bridge == nil {
				t.Fatal("original root bridge")
			}
			copied, _ := Parse(append([]byte(nil), files["RootBridgePacket.class"]...))
			var method *MemberInfo
			for _, m := range copied.Methods {
				desc, _ := sourceBridgeUTF8(copied, m.DescriptorIndex)
				if desc == bridge.descriptor {
					method = m
				}
			}
			child := p.children["RootBridgePacket$Member"].object
			var ref *ConstantMethodrefInfo
			refIndex := 0
			for i, c := range child.ConstantPool {
				if r, ok := c.(*ConstantMethodrefInfo); ok {
					nt, ok := child.ConstantPool[r.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
					if ok {
						desc, _ := sourceBridgeUTF8(child, nt.DescriptorIndex)
						if desc == bridge.descriptor {
							ref = r
							refIndex = i + 1
						}
					}
				}
			}
			if ref == nil || method == nil {
				t.Fatal("original symbolic reference")
			}
			marker := bridge.marker
			var work *workbudget.Budget
			switch variant {
			case "changed delegate":
				for _, a := range method.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						code.Code[1] = byte(core.OP_ACONST_NULL)
					}
				}
			case "missing bridge":
				p.rootAccessBridges = nil
			case "wrong marker":
				marker = "Foreign$1"
			case "method handle":
				child.ConstantPool = append(child.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 8, ReferenceIndex: uint16(refIndex)})
			case "field reference":
				child.ConstantPool[refIndex-1] = &ConstantFieldrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "interface reference":
				child.ConstantPool[refIndex-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				work.Charge(workbudget.CounterGraphScans, 1)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			decl := nativeMemberJointBridgeDeclaration(p, copied, method, marker, work)
			symbolic := nativeMemberJointBridgeNameTypes(p, child, work)[int(ref.NameAndTypeIndex)]
			good := variant == "original" || variant == "copied declaration"
			if (decl && symbolic) != good {
				t.Fatalf("declaration=%v symbolic=%v", decl, symbolic)
			}
		})
	}
}

func TestNativeRootBridgeSourceRequiresRecordedOriginAndLiteralNull(t *testing.T) {
	files := nativeCompileClasses(t, nativeRootPrivateConstructorFixture)
	for _, variant := range []string{"original", "wrong PC", "wrong descriptor", "foreign owner", "wrong method", "foreign constructor", "missing marker", "non-null marker", "effectful marker", "missing operand", "budget"} {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ := Parse(append([]byte(nil), files["RootBridgePacket.class"]...))
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			obj := p.children["RootBridgePacket$Member"].object
			plan := p.rootBridgeDelegations[nativeRootBridgeDelegationKey(obj.GetClassName(), "(Ljava/lang/Object;)V")]
			ctx := &class_context.ClassContext{ClassName: obj.GetClassName(), FunctionName: "<init>", CurrentMethodDesc: "(Ljava/lang/Object;)V", InvocationMetadata: d.buildInvocationMetadata()}
			binding := nativeMemberBinding(ctx, p, nil)
			args := []any{values.NewJavaLiteral("null", types.NewJavaClass("java/lang/Object")), values.NewJavaLiteral("null", types.NewJavaClass("RootBridgePacket$1"))}
			owner, desc, pc := plan.owner, plan.descriptor, plan.pc
			switch variant {
			case "wrong PC":
				pc++
			case "wrong descriptor":
				desc = "()V"
			case "foreign owner":
				owner = "Foreign"
			case "wrong method":
				ctx.FunctionName = "make"
			case "foreign constructor":
				ctx.CurrentMethodDesc = "()V"
			case "missing marker":
				args = args[:1]
			case "non-null marker":
				args[1] = values.NewJavaLiteral("7", types.NewJavaPrimer("int"))
			case "effectful marker":
				args[1] = values.NewCustomValue(func(*class_context.ClassContext) string { panic("must not render unused marker") }, func() types.JavaType { return types.NewJavaClass("RootBridgePacket$1") })
			case "missing operand":
				args = args[1:]
			case "budget": // Exhausted request metadata must prevent source binding.
				work := workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				work.Charge(workbudget.CounterGraphScans, 1)
				binding = nativeMemberBinding(ctx, p, work)
			}
			source, known := nativeRootBridgeSourceDelegation(p, obj, ctx, binding, owner, desc, pc, args)
			if known != (variant == "original") {
				t.Fatalf("source=%q known=%v", source, known)
			}
		})
	}
}

func TestNativeRootBridgeArchiveRejectsUnclosedOriginalUsers(t *testing.T) {
	compiled := nativeCompileClasses(t, nativeRootPrivateConstructorFixture+"\nclass RootBridgeForeign {}")
	for _, variant := range []string{"original", "foreign field", "foreign signature", "foreign constructor handle", "root signature", "missing marker", "executable marker", "bridge delegate changed"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for n, raw := range compiled {
				files[n] = append([]byte(nil), raw...)
			}
			root, _ := Parse(append([]byte(nil), files["RootBridgePacket.class"]...))
			reader := NewClassObjectDumper(root)
			var bridge *nativeConstructorAccessBridge
			for _, b := range reader.nativeConstructorAccessBridges() {
				bridge = b
			}
			if bridge == nil {
				t.Fatal("original bridge")
			}
			foreign, _ := Parse(files["RootBridgeForeign.class"])
			utf := func(obj *ClassObject, text string) uint16 {
				obj.ConstantPool = append(obj.ConstantPool, &ConstantUtf8Info{Value: text})
				return uint16(len(obj.ConstantPool))
			}
			switch variant {
			case "foreign field":
				foreign.Fields = append(foreign.Fields, &MemberInfo{AccessFlags: 8, NameIndex: utf(foreign, "marker"), DescriptorIndex: utf(foreign, "L"+bridge.marker+";")})
			case "foreign signature":
				utf(foreign, "Signature")
				foreign.Fields = append(foreign.Fields, &MemberInfo{AccessFlags: 8, NameIndex: utf(foreign, "marker"), DescriptorIndex: utf(foreign, "Ljava/lang/Object;"), Attributes: []AttributeInfo{&SignatureAttribute{Type: "Signature", AttrLen: 2, SignatureIndex: utf(foreign, "L"+bridge.marker+";")}}})
			case "foreign constructor handle":
				name := utf(foreign, root.GetClassName())
				foreign.ConstantPool = append(foreign.ConstantPool, &ConstantClassInfo{NameIndex: name})
				owner := uint16(len(foreign.ConstantPool))
				name = utf(foreign, "<init>")
				desc := utf(foreign, bridge.descriptor)
				foreign.ConstantPool = append(foreign.ConstantPool, &ConstantNameAndTypeInfo{NameIndex: name, DescriptorIndex: desc})
				nt := uint16(len(foreign.ConstantPool))
				foreign.ConstantPool = append(foreign.ConstantPool, &ConstantMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: owner, NameAndTypeIndex: nt}})
				ref := uint16(len(foreign.ConstantPool))
				foreign.ConstantPool = append(foreign.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: 8, ReferenceIndex: ref})
			case "root signature":
				utf(root, "Signature")
				root.Fields[0].Attributes = append(root.Fields[0].Attributes, &SignatureAttribute{Type: "Signature", AttrLen: 2, SignatureIndex: utf(root, "L"+bridge.marker+";")})
				files["RootBridgePacket.class"] = root.Bytes()
			case "missing marker":
				delete(files, bridge.marker+".class")
			case "executable marker":
				marker, _ := Parse(files[bridge.marker+".class"])
				marker.Fields = append(marker.Fields, &MemberInfo{AccessFlags: 8, NameIndex: utf(marker, "value"), DescriptorIndex: utf(marker, "I")})
				files[bridge.marker+".class"] = marker.Bytes()
			case "bridge delegate changed":
				for _, a := range bridge.method.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						code.Code[1] = byte(core.OP_ACONST_NULL)
					}
				}
				files["RootBridgePacket.class"] = root.Bytes()
			}
			files["RootBridgeForeign.class"] = foreign.Bytes()
			for name, raw := range files {
				if _, err := Parse(append([]byte(nil), raw...)); err != nil {
					t.Fatalf("mutated original metadata %s: %v", name, err)
				}
			}
			z := nativeArchive(t, files)
			defer z.Close()
			root, _ = Parse(append([]byte(nil), files["RootBridgePacket.class"]...))
			entry := z.nativeMemberEntry(root)
			known := entry != nil && entry.family != nil
			if known != (variant == "original") {
				t.Fatalf("archive closure=%v", known)
			}
		})
	}
}

func TestNativeRootBridgeAllocationUsesItsOwnSourceOriginProof(t *testing.T) {
	fixture := strings.Replace(nativeRootPrivateConstructorFixture,
		"Member(Object value){super(value);RootBridgeEffects.trace+=\"M\";}",
		"Member(Object value){super(value);RootBridgeEffects.trace+=\"M\";} static RootBridgePacket direct(Object v){return new RootBridgePacket(v);}", 1)
	fixture = strings.Replace(fixture, "Object token=new Object();int rows=0;", "Object token=new Object();if(RootBridgePacket.Member.direct(token).value!=token)throw new AssertionError(\"root allocation identity\");int rows=0;", 1)
	files := nativeCompileClasses(t, fixture)
	_, java := t04Tools(t)
	original := t.TempDir()
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "RootBridgeDriver"); got != "2:private-root:identity:order:owner\n" {
		t.Fatalf("original JVM=%q", got)
	}
	z := nativeArchive(t, files)
	defer z.Close()
	root, err := Parse(append([]byte(nil), files["RootBridgePacket.class"]...))
	if err != nil {
		t.Fatal(err)
	}
	entry := z.nativeMemberEntry(root)
	if entry == nil || entry.family == nil {
		t.Fatal("original NEW allocation must have independent closure")
	}
	p := entry.family
	member := p.children["RootBridgePacket$Member"].object
	plans, known := z.nativeMemberReader(member).nativeMemberAllocations(p)
	if !known || len(plans["direct(Ljava/lang/Object;)LRootBridgePacket;"]) != 1 {
		t.Fatal("missing original direct allocation origin")
	}
	for pc, plan := range plans["direct(Ljava/lang/Object;)LRootBridgePacket;"] {
		if plan.child != nil || plan.rootObject != p.lexicalObjects[p.owner] || plan.newPC != 0 || plan.invokePC != pc {
			t.Fatal("root NEW borrowed lexical child/SUPER identity")
		}
	}

}

func TestNativeRootAbstractPrivateConstructorBridgeNeedsCompilerProfileProof(t *testing.T) {
	// A valid original JVM may keep the abstract root's constructor private and
	// delegate through the synthetic access bridge. Modern javac --release 8
	// instead widens that constructor and emits no bridge/marker. Source syntax
	// alone is insufficient evidence of equivalent regenerated binary metadata.
	fixture := strings.Replace(nativeRootPrivateConstructorFixture,
		"Object token=new Object();int rows=0;",
		"Object token=new Object();if(!java.lang.reflect.Modifier.isAbstract(RootBridgePacket.class.getModifiers())||!java.lang.reflect.Modifier.isPrivate(RootBridgePacket.class.getDeclaredConstructor(Object.class).getModifiers())||!Class.forName(\"RootBridgePacket$1\").isSynthetic())throw new AssertionError(\"original abstract/private/marker metadata\");int rows=0;", 1)
	fixture = strings.Replace(fixture, "main(String[]args){", "main(String[]args)throws Exception{", 1)
	files := nativeCompileClasses(t, fixture)
	root, err := Parse(append([]byte(nil), files["RootBridgePacket.class"]...))
	if err != nil {
		t.Fatal(err)
	}
	root.AccessFlags |= 0x0400
	files["RootBridgePacket.class"] = root.Bytes()
	_, java := t04Tools(t)
	original := t.TempDir()
	for name, raw := range files {
		if err := os.WriteFile(filepath.Join(original, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if got := t04RunJava(t, java, original, "RootBridgeDriver"); got != "2:private-root:identity:order:owner\n" {
		t.Fatalf("original JVM=%q", got)
	}
	z := nativeArchive(t, files)
	defer z.Close()
	entry := z.nativeMemberEntry(root)
	if entry != nil && entry.family != nil {
		t.Fatal("abstract private constructor metadata lacks a matching compiler profile proof")
	}
}
