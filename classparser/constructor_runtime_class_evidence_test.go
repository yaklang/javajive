package javaclassparser

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeConstructorRuntimeClassRequiresFreshReceiverAndExactBootstrap(t *testing.T) {
	files := nativeCompileClasses(t, `class QueryEvidenceBase{} final class QueryEvidenceChild extends QueryEvidenceBase{}`)
	bootstrap, ok := jdkConstructorClassBytes("java/lang/Object", 8)
	if !ok {
		t.Fatal("missing original bootstrap evidence")
	}
	for _, variant := range []string{
		"original", "inherited symbolic owner", "parent symbolic owner", "self storage", "runtime-specific evidence",
		"not fresh", "nil storage", "class mirror receiver", "class mirror ancestor", "final ancestor", "open finalizer", "special", "interface", "static",
		"wrong member", "wrong descriptor", "unrelated owner", "unrelated caller", "missing parent", "cycle", "interface ancestor",
		"root shadow", "parent shadow", "provider changes root methods", "provider changes root name", "missing native", "not final", "private", "static native", "synchronized native", "native with code", "duplicate method", "bootstrap parent",
		"chain budget", "work", "memory", "canceled",
	} {
		t.Run(variant, func(t *testing.T) {
			parse := func(raw []byte) *ClassObject {
				o, err := Parse(bytes.Clone(raw))
				if err != nil {
					t.Fatal(err)
				}
				return o
			}
			root := parse(files["QueryEvidenceChild.class"])
			parent := parse(files["QueryEvidenceBase.class"])
			object := parse(bootstrap)
			var target *MemberInfo
			for _, method := range object.Methods {
				n, _ := object.getUtf8(method.NameIndex)
				d, _ := object.getUtf8(method.DescriptorIndex)
				if n == "getClass" && d == "()Ljava/lang/Class;" {
					target = method
				}
			}
			if target == nil {
				t.Fatal("missing physical bootstrap method")
			}
			shadow := func(o *ClassObject) {
				o.Methods = append(o.Methods, &MemberInfo{NameIndex: sourceBridgePoolString(t, o, "getClass"), DescriptorIndex: sourceBridgePoolString(t, o, "()Ljava/lang/Class;"), AccessFlags: 0x0001})
			}
			inputs := map[string]*ClassObject{parent.GetClassName(): parent, "java/lang/Object": object}
			d := NewClassObjectDumper(root)
			d.options.TargetSourceVersion = 8
			d.constructorReceiverFinalizerSilent = true
			d.foldSiblingResolver = func(n string) ([]byte, bool) {
				if n == "java/lang/Object" {
					switch variant {
					case "provider changes root methods":
						shadow(root)
					case "provider changes root name":
						root.ConstantPool[root.ThisClass-1].(*ConstantClassInfo).NameIndex = sourceBridgePoolString(t, root, "ChangedRoot")
					}
				}
				o := inputs[n]
				if o == nil {
					return nil, false
				}
				return o.Bytes(), true
			}
			member := &values.JavaClassMember{Name: "java/lang/Object", Member: "getClass", Description: "()Ljava/lang/Class;"}
			caller, opcode, remaining := parent, core.OP_INVOKEVIRTUAL, 512
			aliases := &constructorSelfStorageProof{freshReceiver: true}
			switch variant {
			case "inherited symbolic owner":
				member.Name = root.GetClassName()
			case "parent symbolic owner":
				member.Name = parent.GetClassName()
			case "self storage":
				aliases.selfStored = true
			case "runtime-specific evidence":
				d.constructorProfileEvidence = &constructorProfileEvidence{eligible: true}
			case "not fresh":
				aliases.freshReceiver = false
			case "nil storage":
				aliases = nil
			case "class mirror receiver":
				root.ConstantPool[root.ThisClass-1].(*ConstantClassInfo).NameIndex = sourceBridgePoolString(t, root, "java/lang/Class")
			case "class mirror ancestor":
				parent.ConstantPool[parent.ThisClass-1].(*ConstantClassInfo).NameIndex = sourceBridgePoolString(t, parent, "java/lang/Class")
				root.SuperClass = uint16(NewConstantPoolWithConstant(&root.ConstantPool).AddNewClassInfo("java/lang/Class"))
				inputs["java/lang/Class"] = parent
			case "final ancestor":
				parent.AccessFlags |= 0x0010
			case "open finalizer":
				d.constructorReceiverFinalizerSilent = false
			case "special":
				opcode = core.OP_INVOKESPECIAL
			case "interface":
				opcode = core.OP_INVOKEINTERFACE
			case "static":
				opcode = core.OP_INVOKESTATIC
			case "wrong member":
				member.Member = "clone"
			case "wrong descriptor":
				member.Description = "()Ljava/lang/Object;"
			case "unrelated owner":
				member.Name = "OtherClass"
			case "unrelated caller":
				caller = parse(files["QueryEvidenceChild.class"])
				caller.ConstantPool[caller.ThisClass-1].(*ConstantClassInfo).NameIndex = sourceBridgePoolString(t, caller, "OtherClass")
			case "missing parent":
				root.SuperClass = uint16(NewConstantPoolWithConstant(&root.ConstantPool).AddNewClassInfo("MissingQueryParent"))
			case "cycle":
				parent.SuperClass = parent.ThisClass
			case "interface ancestor":
				parent.AccessFlags |= 0x0200
			case "root shadow":
				shadow(root)
			case "parent shadow":
				shadow(parent)
			case "missing native":
				target.AccessFlags &^= 0x0100
			case "not final":
				target.AccessFlags &^= 0x0010
			case "private":
				target.AccessFlags = 0x0112
			case "static native":
				target.AccessFlags |= 0x0008
			case "synchronized native":
				target.AccessFlags |= 0x0020
			case "native with code":
				target.Attributes = append(target.Attributes, &CodeAttribute{Type: "Code", MaxStack: 1, MaxLocals: 1, Code: []byte{core.OP_ALOAD_0, core.OP_ARETURN}})
			case "duplicate method":
				object.Methods = append(object.Methods, target)
			case "bootstrap parent":
				object.SuperClass = object.ThisClass
			case "chain budget":
				remaining = 1
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := d.constructorReceiverRuntimeClass(caller, member, opcode, &remaining, aliases)
			want := variant == "original" || variant == "inherited symbolic owner" || variant == "parent symbolic owner" || variant == "self storage" || variant == "runtime-specific evidence"
			if got != want {
				t.Fatalf("bootstrap query=%v want=%v", got, want)
			}
			if aliases != nil && aliases.referenceRead {
				t.Fatal("type query acquired reference-field read evidence")
			}
			if variant == "runtime-specific evidence" && (!d.constructorProfileEvidence.eligible || !d.constructorProfileEvidence.runtimeSpecific) {
				t.Fatal("VM primitive must preserve owned evidence and forbid runtime certificate reuse")
			}
		})
	}
}

func TestNativeConstructorRuntimeClassEvidenceCannotShortcutAnotherRuntime(t *testing.T) {
	files := nativeCompileClasses(t, `final class RuntimeEvidenceRoot{}`)
	for _, specific := range []bool{false, true} {
		obj, err := Parse(bytes.Clone(files["RuntimeEvidenceRoot.class"]))
		if err != nil {
			t.Fatal(err)
		}
		d := NewClassObjectDumper(obj)
		d.options.TargetSourceVersion = 8
		d.FuncCtx = &class_context.ClassContext{}
		d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
		e := &constructorProfileEvidence{eligible: true, rootName: obj.GetClassName(), rootSuper: obj.GetSupperClassName(), rootFlags: obj.AccessFlags}
		previous := *e
		previous.bootstrapUsed, previous.runtimeSpecific = true, specific
		remaining := 512
		if got := e.revalidates(d, &previous, &remaining); got == specific {
			t.Fatalf("runtime-specific=%v certificate reused=%v", specific, got)
		}
	}
}
