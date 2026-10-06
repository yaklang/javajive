package javaclassparser

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeAnonymousStandaloneTailRequiresIndependentOriginalScope(t *testing.T) {
	base := nativeCompileClasses(t, `class TailAdmissionOwner{Runnable first(){return new Runnable(){public void run(){}};}static java.util.Iterator<Integer> tail(final int n){return new java.util.Iterator<Integer>(){public boolean hasNext(){return false;}public Integer next(){return n;}};}}`)
	for _, variant := range []string{"original", "wrong identity", "wrong owner", "not final", "wrong superclass", "modern version", "minor version", "preownership version", "instance lexical owner", "missing lexical method", "prefix class constant", "prefix array constant", "prefix field declaration", "prefix method signature", "free field binder", "field own binder", "wrong original capture", "unsupported source profile", "budget", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			root, err := Parse(base["TailAdmissionOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			tail, err := Parse(base["TailAdmissionOwner$2.class"])
			if err != nil {
				t.Fatal(err)
			}
			tail.AccessFlags |= 0x0010
			cp := NewConstantPoolWithConstant(&tail.ConstantPool)
			switch variant {
			case "wrong identity":
				tail.ThisClass = uint16(cp.AddNewClassInfo("Foreign"))
			case "wrong owner":
				for _, a := range tail.Attributes {
					if raw, ok := a.(*UnparsedAttribute); ok && raw.Name == "EnclosingMethod" {
						binary.BigEndian.PutUint16(raw.Info, uint16(cp.AddNewClassInfo("Foreign")))
					}
				}
			case "not final":
				tail.AccessFlags &^= 0x0010
			case "modern version":
				tail.MajorVersion = 55
			case "minor version":
				tail.MinorVersion = 1
			case "preownership version":
				tail.MajorVersion = 48
			case "wrong superclass":
				tail.SuperClass = uint16(cp.AddNewClassInfo("Foreign"))
			case "instance lexical owner", "missing lexical method":
				for _, m := range root.Methods {
					n, _ := sourceBridgeUTF8(root, m.NameIndex)
					if n == "tail" {
						if variant == "instance lexical owner" {
							m.AccessFlags &^= 8
						} else {
							pool := NewConstantPoolWithConstant(&root.ConstantPool)
							m.NameIndex = uint16(pool.AddUtf8Info("different"))
						}
					}
				}
			case "prefix class constant":
				cp.AddNewClassInfo("TailAdmissionOwner$1")
			case "prefix array constant":
				cp.AddNewClassInfo("[LTailAdmissionOwner$1;")
			case "prefix field declaration":
				tail.Fields = append(tail.Fields, &MemberInfo{AccessFlags: 0x0010, NameIndex: uint16(cp.AddUtf8Info("escaped")), DescriptorIndex: uint16(cp.AddUtf8Info("LTailAdmissionOwner$1;"))})
			case "prefix method signature":
				for _, m := range tail.Methods {
					n, _ := sourceBridgeUTF8(tail, m.NameIndex)
					if n == "next" {
						m.Attributes = append(m.Attributes, &SignatureAttribute{Type: "Signature", AttrLen: 2, SignatureIndex: uint16(cp.AddUtf8Info("()LTailAdmissionOwner$1;"))})
						break
					}
				}
			case "free field binder", "field own binder":
				sig := "TT;"
				if variant == "field own binder" {
					sig = "<T:Ljava/lang/Object;>TT;"
				}
				tail.Fields[0].Attributes = append(tail.Fields[0].Attributes, &SignatureAttribute{Type: "Signature", AttrLen: 2, SignatureIndex: uint16(cp.AddUtf8Info(sig))})
			case "wrong original capture":
				for _, m := range tail.Methods {
					n, _ := sourceBridgeUTF8(tail, m.NameIndex)
					if n == "<init>" {
						for _, a := range m.Attributes {
							if code, ok := a.(*CodeAttribute); ok {
								if len(code.Code) < 2 || code.Code[1] != byte(core.OP_ILOAD_1) {
									t.Fatal("original capture packet")
								}
								code.Code[1] = byte(core.OP_ICONST_0)
							}
						}
					}
				}
			}
			z := nativeArchive(t, base)
			defer z.Close()
			reader := z.nativeMemberReader(root)
			p := &nativeAnonymousFamily{owner: root.GetClassName(), children: map[string]*nativeAnonymousClass{"TailAdmissionOwner$1": {ordinal: 1}}, standalone: map[string]*ClassObject{"TailAdmissionOwner$2": tail}}
			switch variant {
			case "unsupported source profile":
				reader.options.TargetSourceVersion = 17
			case "budget":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				reader.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				reader.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := reader.nativeAnonymousStandaloneTailClosed(p, nil, nil, reader.buildInvocationMetadata(), nil); got != (variant == "original") {
				t.Fatalf("independent tail admission=%v", got)
			}
		})
	}
}

func TestNativeAnonymousPrefixArchiveUsersCannotBorrowIndependentTransaction(t *testing.T) {
	for _, variant := range []string{"closed", "invalid index", "foreign user", "standalone user", "handle", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			p := &nativeAnonymousFamily{owner: "Owner", children: map[string]*nativeAnonymousClass{"Owner$1": {}}, standalone: map[string]*ClassObject{"Owner$2": {}}}
			index := &nativeMemberIndex{valid: true, typeUsers: map[string]map[string]bool{"Owner$1": {"Owner": true, "Owner$1": true}}, handles: map[string]bool{}}
			var work *workbudget.Budget
			switch variant {
			case "invalid index":
				index.valid = false
			case "foreign user":
				index.typeUsers["Owner$1"]["Foreign"] = true
			case "standalone user":
				index.typeUsers["Owner$1"]["Owner$2"] = true
			case "handle":
				index.handles["Owner$1"] = true
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnonymousPrefixArchiveClosed(p, index, work); got != (variant == "closed") {
				t.Fatalf("archive closure=%v", got)
			}
		})
	}
}

// The forest can leave a proved terminal binary flat, but cannot assign it a
// different allocating owner or silently treat an unknown anonymous allocation
// as a lexical unit. Full constructor/scope admission is tested above.
func TestNativeAnonymousForestIndependentAllocationKeepsOriginalOwner(t *testing.T) {
	base := nativeCompileClasses(t, `class ForestTailOwner{static Runnable tail(){return new Runnable(){public void run(){}};}}`)
	for _, variant := range []string{"independent", "missing group", "wrong allocating owner", "missing tail", "wrong tail identity", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			root, e := Parse(base["ForestTailOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			tail, e := Parse(base["ForestTailOwner$1.class"])
			if e != nil {
				t.Fatal(e)
			}
			name := tail.GetClassName()
			group := &nativeAnonymousFamily{owner: root.GetClassName(), standalone: map[string]*ClassObject{name: tail}}
			forest := &nativeAnonymousForest{root: root.GetClassName(), groups: map[string]*nativeAnonymousFamily{root.GetClassName(): group}, objects: map[string]*ClassObject{root.GetClassName(): root}, units: map[string]*nativeAnonymousClass{}, anonymousTypes: map[string]bool{name: true}}
			var work *workbudget.Budget
			switch variant {
			case "missing group":
				delete(forest.groups, root.GetClassName())
			case "wrong allocating owner":
				group.owner = "Foreign"
			case "missing tail":
				delete(group.standalone, name)
			case "wrong tail identity":
				pool := NewConstantPoolWithConstant(&tail.ConstantPool)
				tail.ThisClass = uint16(pool.AddNewClassInfo("Foreign"))
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnonymousForestOpcodeClosure(forest, work); got != (variant == "independent") {
				t.Fatalf("independent allocation closure=%v", got)
			}
			if len(forest.units) != 0 || len(forest.objects) != 1 {
				t.Fatal("independent tail acquired lexical ownership")
			}
		})
	}
}

func TestNativeAnonymousPrivateSuperBridgeRequiresOriginalPacket(t *testing.T) {
	base := nativeCompileClasses(t, `class PrivateSuperPacketOwner{private PrivateSuperPacketOwner(int n){}PrivateSuperPacketOwner make(final int n){return new PrivateSuperPacketOwner(n){};}static class Named{}}`)
	for _, variant := range []string{"original", "missing family", "wrong method", "wrong descriptor", "wrong target", "wrong physical descriptor", "wrong super pc", "different original object", "missing unit", "wrong source descriptor", "missing bridge", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			root, e := Parse(base["PrivateSuperPacketOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			z := nativeArchive(t, base)
			defer z.Close()
			reader := z.nativeMemberReader(root)
			p := reader.planNativeMemberFamily()
			if p == nil || !reader.planNativeMemberAnonymousScopes(p) {
				t.Fatal("original private bridge scope")
			}
			group := p.anonymousUnits["PrivateSuperPacketOwner$1"]
			if group == nil {
				t.Fatal("original anonymous unit")
			}
			unit := group.children["PrivateSuperPacketOwner$1"]
			object, method, descriptor, target, physical, pc := unit.object, "<init>", unit.descriptor, unit.object.GetSupperClassName(), unit.superDescriptor, unit.superPC
			var work *workbudget.Budget
			switch variant {
			case "missing family":
				p = nil
			case "wrong method":
				method = "later"
			case "wrong descriptor":
				descriptor = "()V"
			case "wrong target":
				target = "Foreign"
			case "wrong physical descriptor":
				physical = "()V"
			case "wrong super pc":
				pc++
			case "different original object":
				object, e = Parse(base["PrivateSuperPacketOwner$1.class"])
				if e != nil {
					t.Fatal(e)
				}
			case "missing unit":
				delete(p.anonymousUnits, unit.object.GetClassName())
			case "wrong source descriptor":
				unit.sourceSuperDescriptor = "()V"
			case "missing bridge":
				delete(p.rootAccessBridges, physical)
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeAnonymousBridgeSuperOwned(p, object, method, descriptor, target, physical, pc, work); got != (variant == "original") {
				t.Fatalf("private anonymous super bridge=%v", got)
			}
		})
	}
}

func TestNativeAnonymousPrefixMethodHandleScopeNeedsCaptureTransfer(t *testing.T) {
	base := nativeCompileClasses(t, `class MethodHandlePrefixOwner{Runnable first(){return new Runnable(){public void run(){}};}Runnable other(){return null;}static Runnable tail(final int n){return new Runnable(){public void run(){if(n==7)throw new AssertionError();}};}}`)
	for _, variant := range []string{"direct method", "implementation handle", "different method", "foreign method", "different descriptor", "constructor handle", "malformed handle", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			root, e := Parse(base["MethodHandlePrefixOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			tail, e := Parse(base["MethodHandlePrefixOwner$2.class"])
			if e != nil {
				t.Fatal(e)
			}
			tail.AccessFlags |= 0x10
			pool := NewConstantPoolWithConstant(&root.ConstantPool)
			if variant != "direct method" && variant != "canceled" {
				owner, name, desc, kind := "MethodHandlePrefixOwner", "first", "()Ljava/lang/Runnable;", uint8(5)
				switch variant {
				case "different method":
					name = "other"
				case "foreign method":
					owner = "Foreign"
				case "different descriptor":
					desc = "(I)Ljava/lang/Runnable;"
				case "constructor handle":
					name = "<init>"
					desc = "()V"
					kind = 8
				}
				ref := uint16(pool.AddNewMethodInfo(owner, name, desc))
				if variant == "malformed handle" {
					ref = 0
				}
				root.ConstantPool = append(root.ConstantPool, &ConstantMethodHandleInfo{ReferenceKind: kind, ReferenceIndex: ref})
			}
			z := nativeArchive(t, base)
			defer z.Close()
			reader := z.nativeMemberReader(root)
			p := &nativeAnonymousFamily{owner: root.GetClassName(), children: map[string]*nativeAnonymousClass{"MethodHandlePrefixOwner$1": {ordinal: 1, method: "first()Ljava/lang/Runnable;"}}, standalone: map[string]*ClassObject{"MethodHandlePrefixOwner$2": tail}}
			var work *workbudget.Budget
			if variant == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			want := variant != "implementation handle" && variant != "malformed handle" && variant != "canceled"
			if got := nativeAnonymousAllocationMethodsStayLexical(root, p, work); got != want {
				t.Fatalf("capture source scope=%v", got)
			}
			// Promotion is exercised through the actual production planner, not only
			// the classifier; the original independent tail packet remains unchanged.
			if variant == "direct method" || variant == "implementation handle" {
				reader.foldSiblingResolver = func(name string) ([]byte, bool) {
					if name == tail.GetClassName() {
						return tail.Bytes(), true
					}
					raw, ok := base[name+".class"]
					return raw, ok
				}
				if got := reader.planNativeAnonymousFamily(); (got != nil) != (variant == "direct method") {
					t.Fatalf("partial production promotion=%v", got != nil)
				}
			}
		})
	}
}

func TestNativeAnonymousPrivateMemberSuperBridgeRequiresOriginalSourceProjection(t *testing.T) {
	base := nativeCompileClasses(t, `class MemberSuperProjectionOwner{class Parent{private Parent(int n){}}Parent make(final int n){return new Parent(n){};}}`)
	for _, variant := range []string{"original", "wrong mapped source descriptor", "wrong target packet descriptor", "copied parent declaration", "missing target packet"} {
		t.Run(variant, func(t *testing.T) {
			root, e := Parse(base["MemberSuperProjectionOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			z := nativeArchive(t, base)
			defer z.Close()
			reader := z.nativeMemberReader(root)
			p := reader.planNativeMemberFamily()
			if p == nil || !reader.planNativeMemberAnonymousScopes(p) {
				t.Fatal("original private member scope")
			}
			group := p.anonymousUnits["MemberSuperProjectionOwner$1"]
			if group == nil {
				t.Fatal("original anonymous packet")
			}
			unit := group.children["MemberSuperProjectionOwner$1"]
			member := unit.memberSuper
			if member == nil {
				t.Fatal("original member SUPER")
			}
			target := member.object.GetClassName()
			bridge := p.constructorBridges(target)[unit.superDescriptor]
			if bridge == nil {
				t.Fatal("original bridge")
			}
			ctor := member.constructors[bridge.target]
			if ctor == nil {
				t.Fatal("original target")
			}
			switch variant {
			case "wrong mapped source descriptor":
				ctor.sourceDescriptor = "()V"
			case "wrong target packet descriptor":
				ctor.descriptor = "()V"
			case "copied parent declaration":
				copy := *member
				p.children[target] = &copy
			case "missing target packet":
				delete(member.constructors, bridge.target)
			}
			if got := nativeAnonymousBridgeSuperOwned(p, unit.object, "<init>", unit.descriptor, target, unit.superDescriptor, unit.superPC, nil); got != (variant == "original") {
				t.Fatalf("original enclosing-word projection=%v", got)
			}
		})
	}
}
