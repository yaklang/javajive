package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestAdversarialConstructorCallAndAllocationOperandEvidence(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "OperandProofOwner.java")
	source := `interface OperandProofAPI {Object read(int n);}class OperandProofBox {Object read(){return this;}}class OperandProofCarrier {OperandProofCarrier(Object x){}}class OperandProofParent {OperandProofParent(Object a,Object b,int n){}}class OperandProofOwner {class Child extends OperandProofParent {Child(OperandProofAPI a,OperandProofBox b,Object[] x,int n){super(a.read(n),new OperandProofCarrier(b.read()),x.length);}}}`
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("original %v\n%s", err, out)
	}
	raw := readClassBytes(t, dir, "OperandProofOwner$Child")
	variants := []string{"original", "THIS receiver", "missing provider", "unknown invocation", "incomplete members", "wrong metadata identity", "wrong static kind", "duplicate method", "interface owner kind", "interface count", "interface reserved", "wrong invocation tag", "truncated invocation", "method budget", "hierarchy cycle", "primitive array", "truncated NEW", "wrong NEW tag", "interface NEW", "NEW owner mismatch", "uninitialized cast", "missing allocation duplicate", "wrong init tag", "truncated init", "instruction budget"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			d := &ClassObjectDumper{obj: obj, foldSiblingResolver: resolverFromClasses(classMapFromDir(t, dir)), FuncCtx: &class_context.ClassContext{}}
			baseProvider := d.buildInvocationMetadata()
			var code *CodeAttribute
			var desc string
			for _, m := range obj.Methods {
				n, _ := obj.getUtf8(m.NameIndex)
				if n != "<init>" {
					continue
				}
				desc, _ = obj.getUtf8(m.DescriptorIndex)
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						code = c
					}
				}
			}
			if code == nil {
				t.Fatal("original code absent")
			}
			decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			ops := constructorMotionOps(decoder)
			params, _, err := callbinding.Descriptor(desc)
			if err != nil {
				t.Fatal(err)
			}
			var iface, newOp, dup, innerInit, array *core.OpCode
			for _, op := range ops {
				switch op.Instr.OpCode {
				case core.OP_INVOKEINTERFACE:
					iface = op
				case core.OP_NEW:
					newOp = op
				case core.OP_DUP:
					dup = op
				case core.OP_ARRAYLENGTH:
					array = op
				case core.OP_INVOKESPECIAL:
					if m := constructorMotionMember(obj, op, core.OP_INVOKESPECIAL); m != nil && m.Name == "OperandProofCarrier" {
						innerInit = op
					}
				}
			}
			if iface == nil || newOp == nil || dup == nil || innerInit == nil || array == nil {
				t.Fatal("fixture lost invocation/allocation")
			}
			provider := callbinding.Provider(func(name string) (callbinding.Class, bool) {
				decl, known := baseProvider(name)
				decl.Methods = append([]callbinding.Method(nil), decl.Methods...)
				if name == "OperandProofAPI" {
					switch variant {
					case "unknown invocation":
						return decl, false
					case "incomplete members":
						decl.MembersComplete = false
					case "wrong metadata identity":
						decl.Name = "other"
					case "wrong static kind":
						decl.Methods[0].Static = true
					case "duplicate method":
						decl.Methods = append(decl.Methods, decl.Methods[0])
					case "interface owner kind":
						decl.IsInterface = false
					case "method budget":
						decl.Methods = append(make([]callbinding.Method, 513), decl.Methods...)
					case "hierarchy cycle":
						decl.Methods = nil
						decl.Parents = []string{name}
					}
				}
				return decl, known
			})
			replaceOpcode := func(op *core.OpCode, opcode int) { copy := *op.Instr; copy.OpCode = opcode; op.Instr = &copy }
			switch variant {
			case "THIS receiver":
				replaceOpcode(ops[4], core.OP_ALOAD_0)
			case "missing provider":
				provider = nil
			case "interface count":
				iface.Data[2] = 1
			case "interface reserved":
				iface.Data[3] = 1
			case "wrong invocation tag":
				obj.ConstantPool[core.Convert2bytesToInt(iface.Data[:2])-1] = &ConstantUtf8Info{Value: "read"}
			case "truncated invocation":
				iface.Data = iface.Data[:2]
			case "primitive array":
				for i, op := range ops {
					if op == array {
						replaceOpcode(ops[i-1], core.OP_ICONST_0)
						ops[i-1].Data = nil
					}
				}
			case "truncated NEW":
				newOp.Data = newOp.Data[:1]
			case "wrong NEW tag":
				obj.ConstantPool[core.Convert2bytesToInt(newOp.Data)-1] = &ConstantUtf8Info{Value: "new"}
			case "interface NEW", "NEW owner mismatch":
				cp := NewConstantPoolWithConstant(&obj.ConstantPool)
				wanted := "OperandProofBox"
				if variant == "interface NEW" {
					wanted = "OperandProofAPI"
				}
				found := false
				for i, item := range obj.ConstantPool {
					if c, ok := item.(*ConstantClassInfo); ok && cp.GetUtf8(int(c.NameIndex)).Value == wanted {
						newOp.Data = []byte{byte((i + 1) >> 8), byte(i + 1)}
						found = true
						break
					}
				}
				if !found {
					t.Fatal("missing class", wanted)
				}
			case "uninitialized cast":
				replaceOpcode(dup, core.OP_CHECKCAST)
				dup.Data = append([]byte(nil), newOp.Data...)
			case "missing allocation duplicate":
				replaceOpcode(dup, core.OP_NOP)
			case "wrong init tag":
				obj.ConstantPool[core.Convert2bytesToInt(innerInit.Data)-1] = &ConstantUtf8Info{Value: "init"}
			case "truncated init":
				innerInit.Data = innerInit.Data[:1]
			case "instruction budget":
				copies := make([]*core.OpCode, 513)
				for i := range copies {
					copy := *dup
					replaceOpcode(&copy, core.OP_NOP)
					copies[i] = &copy
				}
				ops = append(append(append([]*core.OpCode{}, ops[:4]...), copies...), ops[4:]...)
			}
			next, call := constructorMotionDelegation(obj, ops, 3, params, constructorParameterSlots(params), provider)
			if got := next != 0 && call != nil; got != (variant == "original") {
				t.Fatalf("proof=%v next=%d target=%#v", got, next, call)
			}
		})
	}
	// Inherited signatures are resolved from exact declarations, not member names.
	root := callbinding.Class{Name: "Child", MembersComplete: true, ParentsComplete: true, Parents: []string{"Base"}}
	base := callbinding.Class{Name: "Base", MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "f", Desc: "()I"}}}
	q := newConstructorWideningQuery(func(n string) (callbinding.Class, bool) {
		if n == root.Name {
			return root, true
		}
		if n == base.Name {
			return base, true
		}
		return callbinding.Class{}, false
	})
	if !q.invocation(&values.JavaClassMember{Name: "Child", Member: "f", Description: "()I"}, core.OP_INVOKEVIRTUAL) {
		t.Fatal("exact inherited declaration refused")
	}
	for _, kind := range []int{core.OP_INVOKEVIRTUAL, core.OP_INVOKESTATIC, core.OP_INVOKEINTERFACE} {
		q := newConstructorWideningQuery(nil)
		if q.invocation(&values.JavaClassMember{Name: "Missing", Member: "f", Description: "()I"}, kind) {
			t.Fatal("missing declaration certified")
		}
	}
}

func TestAdversarialConstructorCallMethodIndexBudgetAndSnapshot(t *testing.T) {
	methods := []callbinding.Method{{Name: "f", Desc: "()I"}, {Name: "<init>", Desc: "()V"}}
	for i := 0; i < 510; i++ {
		methods = append(methods, callbinding.Method{Name: fmt.Sprintf("other%d", i), Desc: "()V"})
	}
	calls := 0
	q := newConstructorWideningQuery(func(n string) (callbinding.Class, bool) {
		calls++
		if n == "Owner" {
			return callbinding.Class{Name: n, MembersComplete: true, ParentsComplete: true, Methods: methods}, true
		}
		return callbinding.Class{Name: n, MembersComplete: true, ParentsComplete: true, Methods: []callbinding.Method{{Name: "f", Desc: "()I"}}}, true
	})
	member := &values.JavaClassMember{Name: "Owner", Member: "f", Description: "()I"}
	if !q.invocation(member, core.OP_INVOKEVIRTUAL) || q.remainingMethods != 0 {
		t.Fatal("exact entry budget did not admit original method")
	}
	// Freeze member records as well as the hierarchy. A repeated lookup must not
	// rescan a provider-owned slice or charge the same table again.
	methods[0].Static = true
	for i := 0; i < 8; i++ {
		if !q.invocation(member, core.OP_INVOKEVIRTUAL) {
			t.Fatal("frozen exact declaration changed")
		}
	}
	if !q.constructor(&values.JavaClassMember{Name: "Owner", Member: "<init>", Description: "()V"}) || calls != 1 || q.remainingMethods != 0 {
		t.Fatal("declaration index was not shared with nested initialization")
	}
	if q.invocation(&values.JavaClassMember{Name: "Extra", Member: "f", Description: "()I"}, core.OP_INVOKEVIRTUAL) || !q.exhausted {
		t.Fatal("method-entry budget did not fail closed across owners")
	}
	if q.invocation(member, core.OP_INVOKEVIRTUAL) {
		t.Fatal("exhausted query reused a cached positive as a proof")
	}
}
