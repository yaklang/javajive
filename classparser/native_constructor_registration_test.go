package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeConstructorRegistrationRequiresOriginalBridgeAndOrder(t *testing.T) {
	files := nativeCompileClasses(t, nativeConstructorOrdinalFixture)
	for _, variant := range []string{"original", "repeated constructor", "missing constructor event", "constructor after getter", "unknown constructor event", "quoted constructor event", "commented constructor event", "missing original class", "mutated original bridge", "invented ordinal", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			root, e := Parse(files["CtorOrdinalOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			child, e := Parse(files["CtorOrdinalOwner$Reader.class"])
			if e != nil {
				t.Fatal(e)
			}
			bridges := NewClassObjectDumper(child).nativeConstructorAccessBridges()
			if len(bridges) != 1 {
				t.Fatal("one original marker bridge")
			}
			p := &nativeMemberFamily{owner: root.GetClassName(), lexicalObjects: map[string]*ClassObject{root.GetClassName(): root, child.GetClassName(): child}, children: map[string]*nativeMemberClass{child.GetClassName(): {object: child, accessBridges: bridges}}, getters: map[string]*nativeMemberPrivateGetter{}}
			var descriptor string
			var bridge *nativeConstructorAccessBridge
			for d, b := range bridges {
				descriptor, bridge = d, b
			}
			constructor := nativeMemberConstructorRegistration(p, child.GetClassName(), descriptor)
			getter := "/*jdec-owned-getter:100:CtorOrdinalOwner:value*/"
			source := constructor + getter
			var work *workbudget.Budget
			switch variant {
			case "repeated constructor":
				source = constructor + constructor + getter
			case "missing constructor event":
				source = getter
			case "constructor after getter":
				source = getter + constructor
			case "unknown constructor event":
				source = strings.Replace(constructor, "CtorOrdinalOwner$Reader", "UnprovedOwner", 1) + getter
			case "quoted constructor event":
				source = "\"" + constructor + "\"" + getter
			case "commented constructor event":
				source = "//" + constructor + "\n" + getter
			case "missing original class":
				delete(p.lexicalObjects, child.GetClassName())
			case "mutated original bridge":
				for _, a := range bridge.method.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						c.ExceptionTable = append(c.ExceptionTable, &ExceptionTableEntry{})
					}
				}
			case "invented ordinal":
				for _, m := range root.Methods {
					if n, _ := sourceBridgeUTF8(root, m.NameIndex); n == "access$100" {
						m.NameIndex = sourceBridgePoolString(t, root, "access$200")
					}
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := nativeMemberCollectPrivateGetters(p, work) && nativeMemberPrivateGetterSourceClosed(p, source, work)
			if got != (variant == "original" || variant == "repeated constructor") {
				t.Fatalf("registration order admitted=%v", got)
			}
		})
	}
}

// A valid JVM caller may invoke its own synthetic constructor bridge. A Java
// call from that declaring class will invoke the private target directly, so
// source ownership alone does not prove regeneration of that call/stack frame.
func TestNativeConstructorRegistrationRefusesOwnBridgeCaller(t *testing.T) {
	fixture := strings.Replace(nativeConstructorOrdinalFixture, "private Reader(){}", "private Reader(){}static Reader self(){return new Reader();}", 1)
	fixture = strings.Replace(fixture, "first==second||", "first==second||CtorOrdinalOwner.Reader.self().read(owner)!=17||", 1)
	files := nativeCompileClasses(t, fixture)
	child, e := Parse(files["CtorOrdinalOwner$Reader.class"])
	if e != nil {
		t.Fatal(e)
	}
	bridges := NewClassObjectDumper(child).nativeConstructorAccessBridges()
	if len(bridges) != 1 {
		t.Fatal("one original bridge")
	}
	var descriptor string
	for d := range bridges {
		descriptor = d
	}
	nameIndex := sourceBridgePoolString(t, child, "<init>")
	descIndex := sourceBridgePoolString(t, child, descriptor)
	child.ConstantPool = append(child.ConstantPool, &ConstantNameAndTypeInfo{NameIndex: nameIndex, DescriptorIndex: descIndex})
	nt := uint16(len(child.ConstantPool))
	child.ConstantPool = append(child.ConstantPool, &ConstantMethodrefInfo{ConstantMemberrefInfo: ConstantMemberrefInfo{ClassIndex: child.ThisClass, NameAndTypeIndex: nt}})
	ref := uint16(len(child.ConstantPool))
	seen := false
	for _, m := range child.Methods {
		if n, _ := sourceBridgeUTF8(child, m.NameIndex); n == "self" {
			for _, a := range m.Attributes {
				if c, ok := a.(*CodeAttribute); ok {
					c.AttrLen += uint32(9 - len(c.Code))
					c.Code = []byte{core.OP_NEW, byte(child.ThisClass >> 8), byte(child.ThisClass), core.OP_DUP, core.OP_ACONST_NULL, core.OP_INVOKESPECIAL, byte(ref >> 8), byte(ref), core.OP_ARETURN}
					c.MaxStack = 3
					seen = true
				}
			}
		}
	}
	if !seen {
		t.Fatal("original allocation method")
	}
	files["CtorOrdinalOwner$Reader.class"] = child.Bytes()
	_, java := t04Tools(t)
	original := t.TempDir()
	for n, raw := range files {
		if e := os.WriteFile(filepath.Join(original, n), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	if got := t04RunJava(t, java, original, "CtorOrdinalDriver"); got != "original:private:constructor:accessor:ordinal\n" {
		t.Fatalf("valid original JVM=%q", got)
	}
	member := &nativeMemberClass{object: child, static: true, accessBridges: bridges}
	p := &nativeMemberFamily{owner: "CtorOrdinalOwner", children: map[string]*nativeMemberClass{child.GetClassName(): member}}
	allocations := map[string]map[int]*nativeMemberAllocation{"self()LCtorOrdinalOwner$Reader;": {5: {child: member, descriptor: descriptor, newPC: 0, invokePC: 5}}}
	if nativeMemberJointBridgeCallersClosed(p, child, allocations, nil) {
		t.Fatal("own synthetic invocation cannot be regenerated as a private source call")
	}
}
