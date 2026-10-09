package javaclassparser

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestAdversarialInitializerTypeScopeRequiresConsistentOriginalContexts(t *testing.T) {
	files := nativeCompileClasses(t, initializerTypeScopeFixture)
	for _, kind := range []string{"two constructors", "one constructor", "missing metadata", "duplicate metadata", "bad packet", "wrong owner", "method context", "old version", "missing target constructor", "duplicate target constructor", "duplicate owner method", "duplicate code", "nil code", "bad code", "foreign new", "wrong invocation descriptor", "ordinary method context", "static constructor", "mixed contexts", "bad static initializer descriptor", "no creation", "budget", "memory", "canceled"} {
		t.Run(kind, func(t *testing.T) {
			owner, err := Parse(bytes.Clone(files["InitializerScopeOwner.class"]))
			if err != nil {
				t.Fatal(err)
			}
			child, err := Parse(bytes.Clone(files["InitializerScopeOwner$1.class"]))
			if err != nil {
				t.Fatal(err)
			}
			var raw *UnparsedAttribute
			var method, ctor *MemberInfo
			var code *CodeAttribute
			for _, a := range child.Attributes {
				if r, ok := a.(*UnparsedAttribute); ok && r.Name == "EnclosingMethod" {
					raw = r
				}
			}
			for _, m := range child.Methods {
				n, _ := sourceBridgeUTF8(child, m.NameIndex)
				if n == "<init>" {
					ctor = m
				}
			}
			for _, m := range owner.Methods {
				n, _ := sourceBridgeUTF8(owner, m.NameIndex)
				d, _ := sourceBridgeUTF8(owner, m.DescriptorIndex)
				if n == "<init>" && d == "()V" {
					method = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if raw == nil || method == nil || ctor == nil || code == nil {
				t.Fatal("original initializer declarations")
			}
			pool := NewConstantPoolWithConstant(&owner.ConstantPool)
			switch kind {
			case "one constructor":
				owner.Methods = []*MemberInfo{method}
			case "missing metadata":
				raw.Name = "unknown"
			case "duplicate metadata":
				child.Attributes = append(child.Attributes, raw)
			case "bad packet":
				raw.Info = raw.Info[:3]
			case "wrong owner":
				binary.BigEndian.PutUint16(raw.Info[:2], child.ThisClass)
			case "method context":
				binary.BigEndian.PutUint16(raw.Info[2:], child.ThisClass)
			case "old version":
				child.MajorVersion = 48
			case "missing target constructor":
				child.Methods = nil
			case "duplicate target constructor":
				child.Methods = append(child.Methods, ctor)
			case "duplicate owner method":
				owner.Methods = append(owner.Methods, method)
			case "duplicate code":
				method.Attributes = append(method.Attributes, code)
			case "nil code":
				method.Attributes = append(method.Attributes, (*CodeAttribute)(nil))
			case "bad code":
				code.Code = []byte{byte(core.OP_NEW), 0}
			case "ordinary method context":
				method.NameIndex = uint16(pool.AddUtf8Info("factory"))
			case "static constructor":
				method.AccessFlags |= StaticFlag
			case "mixed contexts", "bad static initializer descriptor":
				method.NameIndex = uint16(pool.AddUtf8Info("<clinit>"))
				method.AccessFlags |= StaticFlag
				if kind == "bad static initializer descriptor" {
					method.DescriptorIndex = uint16(pool.AddUtf8Info("(I)V"))
				}
			case "no creation":
				owner.Methods = nil
			case "foreign new", "wrong invocation descriptor":
				decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(owner.ConstantPool, i) })
				if err := decoder.ParseOpcode(); err != nil {
					t.Fatal(err)
				}
				pc := -1
				if kind == "foreign new" {
					for _, op := range constructorMotionOps(decoder) {
						if op.Instr.OpCode != core.OP_NEW || len(op.Data) != 2 {
							continue
						}
						target, known := sourceBridgeClassName(owner, core.Convert2bytesToInt(op.Data))
						if known && target == child.GetClassName() {
							pc = int(op.CurrentOffset)
							break
						}
					}
					if pc < 0 {
						t.Fatal("original NEW")
					}
					binary.BigEndian.PutUint16(code.Code[pc+1:], owner.ThisClass)
				} else {
					for _, op := range constructorMotionOps(decoder) {
						member := constructorMotionMember(owner, op, core.OP_INVOKESPECIAL)
						if member != nil && member.Name == child.GetClassName() && member.Member == "<init>" {
							index := core.Convert2bytesToInt(op.Data)
							ref := owner.ConstantPool[index-1].(*ConstantMethodrefInfo)
							nt := owner.ConstantPool[ref.NameAndTypeIndex-1].(*ConstantNameAndTypeInfo)
							nt.DescriptorIndex = uint16(pool.AddUtf8Info("()V"))
							pc = int(op.CurrentOffset)
							break
						}
					}
					if pc < 0 {
						t.Fatal("original target invocation")
					}
				}
			}
			var work *workbudget.Budget
			switch kind {
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "memory":
				work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			static, known := originalInitializerTypeScopeStatic(owner, child, work)
			want := kind == "two constructors" || kind == "one constructor"
			if known != want || static {
				t.Fatalf("static=%v known=%v", static, known)
			}
			if kind == "two constructors" {
				if _, sourcePlacement := nativeAnonymousOriginalContext(owner, child, "", nil); sourcePlacement {
					t.Fatal("type context licensed a single source placement")
				}
			}
		})
	}
}

func TestAdversarialInitializerStaticTypeScopeCutsOriginalClassFormals(t *testing.T) {
	files := nativeCompileClasses(t, `interface StaticTypeSupplier<T>{T get();}class StaticTypeScopeOwner<T extends Number>{static final StaticTypeSupplier<String>supplier=new StaticTypeSupplier<String>(){public String get(){return "fixed";}};}`)
	owner, err := Parse(files["StaticTypeScopeOwner.class"])
	if err != nil {
		t.Fatal(err)
	}
	child, err := Parse(files["StaticTypeScopeOwner$1.class"])
	if err != nil {
		t.Fatal(err)
	}
	static, known := originalInitializerTypeScopeStatic(owner, child, nil)
	if !static || !known {
		t.Fatal(static, known)
	}
	d := NewClassObjectDumper(child)
	d.foldSiblingResolver = func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return b, ok }
	_, _, _, applies, err := d.projectFlattenedMethodFormals([]string{"T"})
	if !applies || err == nil {
		t.Fatal("static initializer borrowed original class formal", applies, err)
	}
}
