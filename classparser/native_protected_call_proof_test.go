package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

func nativeProtectedOriginalClasses(t *testing.T, debug string) map[string][]byte {
	return nativeCompileSourceReleaseClasses(t, map[string]string{"probe/base/DispatchParent.java": nativeProtectedParentFixture, "probe/access/ProtectedCallOwner.java": nativeProtectedCallFixture}, debug, "8")
}

func TestNativeProtectedCallRequiresOriginalVirtualPacket(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeProtectedOriginalClasses(t, debug)
		for _, variant := range []string{"original", "not synthetic", "not static", "wrong receiver", "wrong wide slot", "wrong invocation", "wrong return", "wrong stack", "wrong locals", "extra code", "handler", "public target", "private target", "static target", "generic target", "changed throws", "missing parent", "wrong parent identity", "duplicate method", "own declaration", "same package", "budget", "canceled"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				root, e := Parse(files["probe/access/ProtectedCallOwner.class"])
				if e != nil {
					t.Fatal(e)
				}
				parent, e := Parse(files["probe/base/DispatchParent.class"])
				if e != nil {
					t.Fatal(e)
				}
				var bridge, target *MemberInfo
				var code *CodeAttribute
				for _, m := range root.Methods {
					if n, _ := sourceBridgeUTF8(root, m.NameIndex); n == "access$000" {
						bridge = m
						for _, a := range m.Attributes {
							if c, ok := a.(*CodeAttribute); ok {
								code = c
							}
						}
					}
				}
				for _, m := range parent.Methods {
					n, _ := sourceBridgeUTF8(parent, m.NameIndex)
					desc, _ := sourceBridgeUTF8(parent, m.DescriptorIndex)
					if n == "prepare" && desc == "(Ljava/lang/Object;J)Ljava/lang/Object;" {
						target = m
					}
				}
				if bridge == nil || target == nil || code == nil {
					t.Fatal("original inherited virtual bridge")
				}
				var work *workbudget.Budget
				resolve := func(name string) (*ClassObject, bool) { return parent, name == parent.GetClassName() }
				switch variant {
				case "not synthetic":
					bridge.AccessFlags &^= 0x1000
				case "not static":
					bridge.AccessFlags &^= 8
				case "wrong receiver":
					code.Code[0] = core.OP_ALOAD_1
				case "wrong wide slot":
					code.Code[2] = core.OP_LLOAD_1
				case "wrong invocation":
					code.Code[3] = core.OP_INVOKESPECIAL
				case "wrong return":
					code.Code[len(code.Code)-1] = core.OP_IRETURN
				case "wrong stack":
					code.MaxStack++
				case "wrong locals":
					code.MaxLocals++
				case "extra code":
					code.Code = append([]byte{core.OP_ACONST_NULL, core.OP_POP}, code.Code...)
				case "handler":
					code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{})
				case "public target":
					target.AccessFlags = (target.AccessFlags &^ 4) | 1
				case "private target":
					target.AccessFlags = (target.AccessFlags &^ 4) | 2
				case "static target":
					target.AccessFlags |= 8
				case "generic target":
					target.Attributes = append(target.Attributes, &SignatureAttribute{})
				case "changed throws":
					for _, a := range target.Attributes {
						if ex, ok := a.(*ExceptionsAttribute); ok {
							ex.ExceptionIndexTable = []uint16{parent.ThisClass}
						}
					}
				case "missing parent":
					resolve = func(string) (*ClassObject, bool) { return nil, false }
				case "wrong parent identity":
					resolve = func(string) (*ClassObject, bool) { return root, true }
				case "duplicate method":
					parent.Methods = append(parent.Methods, target)
				case "own declaration":
					root.Methods = append(root.Methods, &MemberInfo{AccessFlags: 4, NameIndex: sourceBridgePoolString(t, root, "prepare"), DescriptorIndex: sourceBridgePoolString(t, root, "(Ljava/lang/Object;J)Ljava/lang/Object;")})
				case "same package":
					old := parent.GetClassName()
					name := "probe/access/DispatchParent"
					idx := parent.ConstantPool[parent.ThisClass-1].(*ConstantClassInfo).NameIndex
					parent.ConstantPool[idx-1].(*ConstantUtf8Info).Value = name
					idx = root.ConstantPool[root.SuperClass-1].(*ConstantClassInfo).NameIndex
					root.ConstantPool[idx-1].(*ConstantUtf8Info).Value = name
					resolve = func(n string) (*ClassObject, bool) { return parent, n == name || n == old }
				case "budget":
					work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					work = workbudget.New(ctx, workbudget.Limits{})
				}
				got := nativeMemberProtectedCallProof(root, bridge, resolve, work)
				if (got != nil) != (variant == "original") {
					t.Fatalf("protected packet admitted=%v", got != nil)
				}
				if got != nil && (got.call == nil || !got.call.inherited || got.call.static || got.staticField || got.setter) {
					t.Fatal("wrong source operation identity")
				}
			})
		}
	}
}

func TestNativeProtectedCallRegistrationRequiresDistinctSourceOccurrences(t *testing.T) {
	files := nativeProtectedOriginalClasses(t, "none")
	root, _ := Parse(files["probe/access/ProtectedCallOwner.class"])
	parent, _ := Parse(files["probe/base/DispatchParent.class"])
	resolve := func(name string) (*ClassObject, bool) { return parent, name == parent.GetClassName() }
	p := &nativeMemberFamily{owner: root.GetClassName(), lexicalObjects: map[string]*ClassObject{root.GetClassName(): root}, getters: map[string]*nativeMemberPrivateGetter{}}
	if !nativeMemberCollectPrivateGettersResolved(p, resolve, nil) || len(p.getters) != 2 {
		t.Fatal("two original compiler-cloned protected symbols")
	}
	a := "/*jdec-owned-getter:0:probe/access/ProtectedCallOwner:prepare*/"
	b := "/*jdec-owned-getter:100:probe/access/ProtectedCallOwner:prepare*/"
	for _, row := range []struct {
		name, source string
		want         bool
	}{{"original", a + b, true}, {"same target cannot merge", a, false}, {"same bridge cannot duplicate", a + a + b, false}, {"source registration order", b + a, false}, {"quoted marker", "\"" + a + "\"" + b, false}} {
		t.Run(row.name, func(t *testing.T) {
			if got := nativeMemberPrivateGetterSourceClosed(p, row.source, nil); got != row.want {
				t.Fatalf("original identity/order=%v", got)
			}
		})
	}
}

func TestNativeProtectedCallRequiresCompleteParentMetadata(t *testing.T) {
	files := nativeCompileClasses(t, nativeProtectedUnavailableFixture)
	root, _ := Parse(files["ProtectedCallOwner.class"])
	for _, m := range root.Methods {
		n, _ := sourceBridgeUTF8(root, m.NameIndex)
		if strings.HasPrefix(n, "access$") {
			if nativeMemberProtectedCallProof(root, m, nil, nil) != nil || nativeMemberProtectedCallProof(root, m, func(string) (*ClassObject, bool) { return nil, false }, nil) != nil {
				t.Fatal("unknown external superclass cannot license inherited protected access")
			}
		}
	}
}
