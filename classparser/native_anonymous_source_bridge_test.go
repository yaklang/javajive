package javaclassparser

import (
	"context"
	"encoding/binary"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

// Isolate the regeneration certificate from the separately checked allocation
// scope. All baseline declarations and forwarder bodies come from javac.
func TestNativeAnonymousZeroArgSourceBridgeOriginalProof(t *testing.T) {
	files := nativeCompileSourceReleaseClasses(t, map[string]string{
		"p/Source.java":      `package p;public interface Source<T>{T fetch();}`,
		"q/BridgeOwner.java": `package q;public class BridgeOwner implements p.Source<String>{public String fetch(){return "value";}}`,
	}, "none", "8")
	for _, variant := range []string{"original", "unrelated bridge", "owner name CP tag", "interface name CP tag", "cyclic interface name", "nonbridge", "nonsynthetic", "nonpublic", "static bridge", "synchronized bridge", "final target", "static target", "copied target", "removed bridge", "duplicate bridge", "duplicate target", "nil method", "no code", "duplicate code", "signature bridge", "nil attribute", "wrong stack", "wrong locals", "exception handler", "wrong receiver", "extra effect", "special dispatch", "wrong return", "zero member", "wrong member tag", "wrong owner", "wrong target name", "wrong target descriptor", "malformed target descriptor", "target signature", "target exception", "interface exception", "no resolve", "unknown interface", "wrong interface identity", "not interface", "interface has parent", "raw source", "wrong physical edge", "wrong class header", "duplicate owner signature", "wrong instantiation", "free source formal", "bounded interface", "wrong interface erasure", "duplicate interface signature", "method inference", "wrong method signature erasure", "wrong interface return", "extra abstract overload", "extra default declaration", "unknown Object", "inherited same name", "graph budget", "memory budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			objects := map[string]*ClassObject{}
			for n, b := range files {
				o, e := Parse(append([]byte(nil), b...))
				if e != nil {
					t.Fatal(e)
				}
				objects[n[:len(n)-6]] = o
			}
			owner, iface := objects["q/BridgeOwner"], objects["p/Source"]
			var target, bridge *MemberInfo
			var code *CodeAttribute
			for _, m := range owner.Methods {
				n, _ := sourceBridgeUTF8(owner, m.NameIndex)
				if n != "fetch" {
					continue
				}
				if m.AccessFlags == 0x1041 {
					bridge = m
					code = m.Attributes[0].(*CodeAttribute)
				} else {
					target = m
				}
			}
			if target == nil || bridge == nil || code == nil {
				t.Fatal("original javac forwarder missing")
			}
			archive := nativeArchive(t, files)
			defer archive.Close()
			fallback := archive.nativeMemberReader(owner).nativeAnnotationDeclarationResolver()
			resolve := func(n string) (*ClassObject, bool) {
				if o := objects[n]; o != nil {
					return o, true
				}
				return fallback(n)
			}
			forest := &nativeAnonymousForest{objects: objects, resolve: resolve}
			setSig := func(o *ClassObject, attrs *[]AttributeInfo, text string) {
				pool := NewConstantPoolWithConstant(&o.ConstantPool)
				for _, a := range *attrs {
					if s, ok := a.(*SignatureAttribute); ok {
						s.SignatureIndex = uint16(pool.AddUtf8Info(text))
						return
					}
				}
				*attrs = append(*attrs, &SignatureAttribute{SignatureIndex: uint16(pool.AddUtf8Info(text))})
			}
			pool := NewConstantPoolWithConstant(&owner.ConstantPool)
			ref := owner.ConstantPool[binary.BigEndian.Uint16(code.Code[2:4])-1].(*ConstantMethodrefInfo)
			nt := owner.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
			var work *workbudget.Budget
			switch variant {
			case "unrelated bridge":
				copy := *bridge
				copy.NameIndex = uint16(pool.AddUtf8Info("unrelated"))
				owner.Methods = append(owner.Methods, &copy)
			case "owner name CP tag":
				owner.ConstantPool = append(owner.ConstantPool, &ConstantStringInfo{StringIndex: owner.ThisClass})
				owner.ThisClass = uint16(len(owner.ConstantPool))
			case "interface name CP tag":
				iface.ConstantPool = append(iface.ConstantPool, &ConstantStringInfo{StringIndex: iface.ThisClass})
				iface.ThisClass = uint16(len(iface.ConstantPool))
			case "cyclic interface name":
				class := iface.ConstantPool[iface.ThisClass-1].(*ConstantClassInfo)
				iface.ConstantPool = append(iface.ConstantPool, &ConstantStringInfo{StringIndex: uint16(len(iface.ConstantPool) + 1)})
				class.NameIndex = uint16(len(iface.ConstantPool))
			case "nonbridge":
				bridge.AccessFlags &^= 0x40
			case "nonsynthetic":
				bridge.AccessFlags &^= 0x1000
			case "nonpublic":
				bridge.AccessFlags &^= 1
			case "static bridge":
				bridge.AccessFlags |= 8
			case "synchronized bridge":
				bridge.AccessFlags |= 0x20
			case "final target":
				target.AccessFlags |= 0x10
			case "static target":
				target.AccessFlags |= 8
			case "copied target":
				copy := *target
				target = &copy
			case "removed bridge":
				for i, m := range owner.Methods {
					if m == bridge {
						owner.Methods = append(owner.Methods[:i], owner.Methods[i+1:]...)
						break
					}
				}
			case "duplicate bridge":
				copy := *bridge
				owner.Methods = append(owner.Methods, &copy)
			case "duplicate target":
				copy := *target
				owner.Methods = append(owner.Methods, &copy)
			case "nil method":
				owner.Methods = append(owner.Methods, nil)
			case "no code":
				bridge.Attributes = nil
			case "duplicate code":
				bridge.Attributes = append(bridge.Attributes, code)
			case "signature bridge":
				setSig(owner, &bridge.Attributes, "()Ljava/lang/Object;")
			case "nil attribute":
				bridge.Attributes = append(bridge.Attributes, nil)
			case "wrong stack":
				code.MaxStack = 2
			case "wrong locals":
				code.MaxLocals = 2
			case "exception handler":
				code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 4, HandlerPc: 4}}
			case "wrong receiver":
				code.Code[0] = core.OP_ACONST_NULL
			case "extra effect":
				code.Code = append([]byte{core.OP_NOP}, code.Code...)
			case "special dispatch":
				code.Code[1] = core.OP_INVOKESPECIAL
			case "wrong return":
				code.Code[4] = core.OP_IRETURN
			case "zero member":
				code.Code[2], code.Code[3] = 0, 0
			case "wrong member tag":
				owner.ConstantPool[binary.BigEndian.Uint16(code.Code[2:4])-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "wrong owner":
				ref.ClassIndex = owner.SuperClass
			case "wrong target name":
				nt.NameIndex = uint16(pool.AddUtf8Info("other"))
			case "wrong target descriptor":
				nt.DescriptorIndex = uint16(pool.AddUtf8Info("()Ljava/lang/Object;"))
			case "malformed target descriptor":
				target.DescriptorIndex = uint16(pool.AddUtf8Info("broken"))
			case "target signature":
				setSig(owner, &target.Attributes, "()Ljava/lang/String;")
			case "target exception":
				target.Attributes = append(target.Attributes, &ExceptionsAttribute{ExceptionIndexTable: []uint16{uint16(pool.AddNewClassInfo("java/lang/Exception"))}})
			case "interface exception":
				iface.Methods[0].Attributes = append(iface.Methods[0].Attributes, &ExceptionsAttribute{})
			case "no resolve":
				forest.resolve = nil
			case "unknown interface":
				forest.resolve = func(n string) (*ClassObject, bool) {
					if n == "p/Source" {
						return nil, false
					}
					return resolve(n)
				}
			case "wrong interface identity":
				iface.ThisClass = uint16(NewConstantPoolWithConstant(&iface.ConstantPool).AddNewClassInfo("Wrong"))
			case "not interface":
				iface.AccessFlags = 0x21
			case "interface has parent":
				iface.Interfaces = []uint16{uint16(NewConstantPoolWithConstant(&iface.ConstantPool).AddNewClassInfo("p/Other"))}
			case "raw source":
				owner.Attributes = nil
			case "wrong physical edge":
				owner.Interfaces = []uint16{uint16(pool.AddNewClassInfo("p/Other"))}
			case "wrong class header":
				setSig(owner, &owner.Attributes, "Ljava/lang/Number;Lp/Source<Ljava/lang/String;>;")
			case "duplicate owner signature":
				for _, a := range owner.Attributes {
					if _, ok := a.(*SignatureAttribute); ok {
						owner.Attributes = append(owner.Attributes, a)
						break
					}
				}
			case "wrong instantiation":
				setSig(owner, &owner.Attributes, "Ljava/lang/Object;Lp/Source<Ljava/lang/Integer;>;")
			case "free source formal":
				setSig(owner, &owner.Attributes, "Ljava/lang/Object;Lp/Source<TE;>;")
			case "bounded interface":
				setSig(iface, &iface.Attributes, "<T:Ljava/lang/Number;>Ljava/lang/Object;")
			case "wrong interface erasure":
				setSig(iface, &iface.Attributes, "<T:Ljava/lang/Number;>Ljava/lang/Object;")
				setSig(owner, &owner.Attributes, "Ljava/lang/Object;Lp/Source<Ljava/lang/Integer;>;")
			case "duplicate interface signature":
				for _, a := range iface.Attributes {
					if _, ok := a.(*SignatureAttribute); ok {
						iface.Attributes = append(iface.Attributes, a)
						break
					}
				}
			case "method inference":
				setSig(iface, &iface.Methods[0].Attributes, "<R:Ljava/lang/Object;>()TR;")
			case "wrong method signature erasure":
				setSig(iface, &iface.Methods[0].Attributes, "()Ljava/lang/String;")
			case "wrong interface return":
				iface.Methods[0].DescriptorIndex = uint16(NewConstantPoolWithConstant(&iface.ConstantPool).AddUtf8Info("()Ljava/lang/String;"))
			case "extra abstract overload":
				copy := *iface.Methods[0]
				copy.NameIndex = uint16(NewConstantPoolWithConstant(&iface.ConstantPool).AddUtf8Info("other"))
				iface.Methods = append(iface.Methods, &copy)
			case "extra default declaration":
				copy := *iface.Methods[0]
				copy.AccessFlags = 1
				iface.Methods = append(iface.Methods, &copy)
			case "unknown Object":
				forest.resolve = func(n string) (*ClassObject, bool) {
					if n == "java/lang/Object" {
						return nil, false
					}
					return resolve(n)
				}
			case "inherited same name":
				o, ok := resolve("java/lang/Object")
				if !ok {
					t.Fatal("original Object")
				}
				copy := *o
				copy.Methods = append([]*MemberInfo{}, o.Methods...)
				m := *target
				m.NameIndex = uint16(NewConstantPoolWithConstant(&copy.ConstantPool).AddUtf8Info("fetch"))
				m.DescriptorIndex = uint16(NewConstantPoolWithConstant(&copy.ConstantPool).AddUtf8Info("()Ljava/lang/Object;"))
				copy.Methods = append(copy.Methods, &m)
				forest.resolve = func(n string) (*ClassObject, bool) {
					if n == "java/lang/Object" {
						return &copy, true
					}
					return resolve(n)
				}
			case "graph budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory budget":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if known := nativeAnonymousZeroArgSourceBridge(forest, owner, target, bridge, work); known != (variant == "original") {
				t.Fatalf("%s original bridge regeneration=%v", variant, known)
			}
		})
	}
}
