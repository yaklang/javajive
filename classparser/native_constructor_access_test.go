package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativePrivateConstructorAccessNeedsCompleteOriginalEvidence(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "PrivateAccessCapture.java")
	if err := os.WriteFile(file, []byte(nativePrivateAccessFixture), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "PrivateAccessCapture.class"))
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"original", "not synthetic", "public bridge", "nonprivate target", "missing code", "duplicate code", "extra computation", "wrong return", "wrong slot", "stack too small", "locals too small", "wrong owner", "missing target", "duplicate target", "duplicate bridge", "4096 duplicate declarations", "missing target code", "handler", "throws mismatch", "duplicate throws", "source17", "major55", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(obj)
			var bridge, target *MemberInfo
			var code *CodeAttribute
			var targetIndex int
			for i, m := range obj.Methods {
				name, _ := obj.getUtf8(m.NameIndex)
				desc, _ := obj.getUtf8(m.DescriptorIndex)
				if name == "<init>" && m.AccessFlags == 0x1000 {
					bridge = m
					for _, a := range m.Attributes {
						if ca, ok := a.(*CodeAttribute); ok {
							code = ca
						}
					}
				}
				if name == "<init>" && desc == "(J)V" {
					target = m
					targetIndex = i
				}
			}
			if bridge == nil || target == nil || code == nil {
				t.Fatal("original evidence absent")
			}
			switch variant {
			case "not synthetic":
				bridge.AccessFlags = 0
			case "public bridge":
				bridge.AccessFlags |= 1
			case "nonprivate target":
				target.AccessFlags = 0
			case "missing code":
				bridge.Attributes = nil
			case "duplicate code":
				bridge.Attributes = append(bridge.Attributes, code)
			case "extra computation":
				code.Code = append([]byte{byte(core.OP_ICONST_0), byte(core.OP_POP)}, code.Code...)
			case "wrong return":
				code.Code[len(code.Code)-1] = byte(core.OP_ATHROW)
			case "wrong slot":
				code.Code[1] = byte(core.OP_LLOAD_2)
			case "stack too small":
				code.MaxStack = 1
			case "locals too small":
				code.MaxLocals = 1
			case "wrong owner":
				index := int(core.Convert2bytesToInt(code.Code[len(code.Code)-3 : len(code.Code)-1]))
				ref, ok := obj.ConstantPool[index-1].(*ConstantMethodrefInfo)
				if !ok {
					t.Fatal("original call kind")
				}
				ref.ClassIndex = obj.SuperClass
			case "missing target":
				obj.Methods = append(obj.Methods[:targetIndex], obj.Methods[targetIndex+1:]...)
			case "duplicate target":
				obj.Methods = append(obj.Methods, target)
			case "duplicate bridge":
				obj.Methods = append(obj.Methods, bridge)
			case "4096 duplicate declarations":
				for i := 0; i < 4096; i++ {
					obj.Methods = append(obj.Methods, bridge)
				}
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 20000})
			case "missing target code":
				target.Attributes = nil
			case "handler":
				code.ExceptionTable = []*ExceptionTableEntry{{}}
			case "throws mismatch":
				bridge.Attributes = append(bridge.Attributes, &ExceptionsAttribute{ExceptionIndexTable: []uint16{obj.SuperClass}})
			case "duplicate throws":
				bridge.Attributes = append(bridge.Attributes, &ExceptionsAttribute{}, &ExceptionsAttribute{})
			case "source17":
				d.options.TargetSourceVersion = 17
			case "major55":
				obj.MajorVersion = 55
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			result := d.nativeConstructorAccessBridges()
			if variant == "4096 duplicate declarations" && d.Work.Err() != nil {
				t.Fatalf("duplicate index exhausted budget: %v", d.Work.Err())
			}
			desc, _ := obj.getUtf8(bridge.DescriptorIndex)
			if got := result[desc] != nil; got != (variant == "original") {
				t.Fatalf("bridge accepted=%v in %s", got, variant)
			}
		})
	}
}

func TestNativeAccessBridgeRejectsIndirectOrForeignSymbolicUses(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "PrivateAccessCapture.java")
	if err := os.WriteFile(file, []byte(nativePrivateAccessFixture), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "PrivateAccessCapture.class"))
	root, _ := Parse(raw)
	d := NewClassObjectDumper(root)
	bridges := d.nativeConstructorAccessBridges()
	raw, _ = os.ReadFile(filepath.Join(dir, "PrivateAccessCapture$1.class"))
	for _, variant := range []string{"original", "foreign owner", "fieldref", "interface ref", "method handle", "dynamic", "invoke dynamic", "4096 duplicate pool entries", "budget", "bad index"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			var index, cpIndex int
			var ref *ConstantMethodrefInfo
			for i, v := range obj.ConstantPool {
				if r, ok := v.(*ConstantMethodrefInfo); ok {
					nt, ok := obj.ConstantPool[r.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
					if !ok {
						continue
					}
					desc, _ := sourceBridgeUTF8(obj, nt.DescriptorIndex)
					if bridges[desc] != nil {
						index = int(r.NameAndTypeIndex)
						cpIndex = i + 1
						ref = r
					}
				}
			}
			if ref == nil {
				t.Fatal("original symbolic call absent")
			}
			var work *workbudget.Budget
			switch variant {
			case "foreign owner":
				ref.ClassIndex = obj.ThisClass
			case "fieldref":
				obj.ConstantPool[cpIndex-1] = &ConstantFieldrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "interface ref":
				obj.ConstantPool[cpIndex-1] = &ConstantInterfaceMethodrefInfo{ConstantMemberrefInfo: ref.ConstantMemberrefInfo}
			case "method handle":
				obj.ConstantPool = append(obj.ConstantPool, &ConstantMethodHandleInfo{ReferenceIndex: uint16(cpIndex), ReferenceKind: 8})
			case "dynamic":
				obj.ConstantPool = append(obj.ConstantPool, &ConstantDynamicInfo{NameAndTypeIndex: uint16(index)})
			case "invoke dynamic":
				obj.ConstantPool = append(obj.ConstantPool, &ConstantInvokeDynamicInfo{NameAndTypeIndex: uint16(index)})
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "4096 duplicate pool entries":
				nt := obj.ConstantPool[index-1].(*ConstantNameAndTypeInfo)
				for i := 0; i < 4096; i++ {
					copy := *nt
					obj.ConstantPool = append(obj.ConstantPool, &copy)
					r := *ref
					r.NameAndTypeIndex = uint16(len(obj.ConstantPool))
					obj.ConstantPool = append(obj.ConstantPool, &r)
				}
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 40000})
			case "bad index":
				index = len(obj.ConstantPool) + 1
			}
			p := &nativeAnonymousFamily{owner: root.GetClassName(), bridges: bridges}
			if got := p.accessBridgeNameType(obj, index, work); got != (variant == "original" || variant == "4096 duplicate pool entries") {
				t.Fatalf("accepted=%v", got)
			}
		})
	}
}

func TestNativePrivateAccessFamilyRefusesAnUnprojectedAdditionalCall(t *testing.T) {
	javac, _ := t04Tools(t)
	for _, extra := range []bool{false, true} {
		dir := t.TempDir()
		source := nativePrivateAccessFixture
		if extra {
			source = strings.Replace(source, "Object token() { return capture; }", "Object token() { return capture; } PrivateAccessCapture extra(long x){return new PrivateAccessCapture(x);}", 1)
		}
		path := filepath.Join(dir, "PrivateAccessCapture.java")
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, path).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, out)
		}
		raw, _ := os.ReadFile(filepath.Join(dir, "PrivateAccessCapture.class"))
		obj, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		d := NewClassObjectDumper(obj)
		d.foldSiblingResolver = func(n string) ([]byte, bool) {
			raw, err := os.ReadFile(filepath.Join(dir, n+".class"))
			return raw, err == nil
		}
		if got := d.planNativeAnonymousFamily() != nil; got == extra {
			t.Fatalf("extra original use=%v family admitted=%v", extra, got)
		}
	}
}
