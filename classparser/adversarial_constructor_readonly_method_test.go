package javaclassparser

import (
	"bytes"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const readonlyConstructorFixture = `
class ReadonlyParent {long seed,value;ReadonlyParent(long n){seed=n;value=arbitraryName();empty();}private long arbitraryName(){return seed;}final void empty(){}Object token(){return null;}}
class ReadonlyOwner {final Object token;ReadonlyOwner(Object t){token=t;}final class Child extends ReadonlyParent {Child(long n){super(n);}Object token(){return ReadonlyOwner.this.token;}}ReadonlyParent make(long n){return new Child(n);}}
class ReadonlyOracle {static void run()throws Exception{Object token=new Object();int rows=0;for(Object t:new Object[]{null,token})for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){ReadonlyParent p=new ReadonlyOwner(t).make(n);if(p.value!=n||p.token()!=t)throw new AssertionError("private/final read and capture");rows++;}System.out.println(rows);}}
public class ReadonlyDriver {public static void main(String[]args)throws Exception{ReadonlyOracle.run();}}
`

func TestAdversarialConstructorReadOnlyPrivateFinalCallsPreserveCapture(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "ReadonlyDriver", readonlyConstructorFixture, nil, []string{"ReadonlyOwner", "ReadonlyOwner$Child"}, true, Precision, Compatibility, "legacy")
}

// Mutations change original evidence, rather than teaching the proof a fixture
// name. Each incomplete, observable or dynamically dispatched body must refuse.
// Clone before Parse because decoded Code aliases the supplied class bytes;
// each negative must be refused for its own mutation, not an earlier failure.
func TestAdversarialConstructorReadOnlyMethodEvidenceBoundaries(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "ReadonlyEvidence.java")
	if err := os.WriteFile(file, []byte(`class ReadonlyEvidence {long seed;ReadonlyEvidence(long n){seed=n;seed=probe();}private long probe(){return seed;}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "ReadonlyEvidence.class"))
	if err != nil {
		t.Fatal(err)
	}
	for _, variant := range []string{"private", "final", "virtual", "synchronized", "native", "abstract", "static", "wrong owner", "wrong descriptor", "wrong opcode", "duplicate declaration", "missing code", "duplicate code", "wrong return", "harmless nop", "extra body", "volatile field", "moved field", "budget"} {
		t.Run(variant, func(t *testing.T) {
			obj, err := Parse(bytes.Clone(raw))
			if err != nil {
				t.Fatal(err)
			}
			var target *MemberInfo
			var code *CodeAttribute
			for _, m := range obj.Methods {
				name, _ := obj.getUtf8(m.NameIndex)
				if name == "probe" {
					target = m
					for _, a := range m.Attributes {
						if c, ok := a.(*CodeAttribute); ok {
							code = c
						}
					}
				}
			}
			if target == nil || code == nil {
				t.Fatal("original declaration absent")
			}
			member := &values.JavaClassMember{Name: obj.GetClassName(), Member: "probe", Description: "()J"}
			opcode := core.OP_INVOKESPECIAL
			writes := map[string]bool{}
			remaining := 1024
			switch variant {
			case "final":
				target.AccessFlags = 0x0010
				opcode = core.OP_INVOKEVIRTUAL
			case "virtual":
				target.AccessFlags = 0x0001
				opcode = core.OP_INVOKEVIRTUAL
			case "synchronized":
				target.AccessFlags |= 0x0020
			case "native":
				target.AccessFlags |= 0x0100
			case "abstract":
				target.AccessFlags |= 0x0400
			case "static":
				target.AccessFlags |= 0x0008
			case "wrong owner":
				member.Name = "AnotherOwner"
			case "wrong descriptor":
				member.Description = "()I"
			case "wrong opcode":
				opcode = core.OP_INVOKESTATIC
			case "duplicate declaration":
				obj.Methods = append(obj.Methods, target)
			case "missing code":
				target.Attributes = nil
			case "duplicate code":
				target.Attributes = append(target.Attributes, code)
			case "wrong return":
				code.Code[len(code.Code)-1] = byte(core.OP_IRETURN)
			case "harmless nop":
				code.Code = append([]byte{byte(core.OP_NOP)}, code.Code...)
			case "extra body":
				// A real storage effect distinguishes this negative from the
				// inert NOP removed by the shared original-opcode decoder.
				if len(code.Code) != 5 || code.Code[1] != byte(core.OP_GETFIELD) {
					t.Fatalf("unexpected original field read %v", code.Code)
				}
				code.Code = append([]byte{byte(core.OP_ALOAD_0), byte(core.OP_LCONST_0), byte(core.OP_PUTFIELD), code.Code[2], code.Code[3]}, code.Code...)
				code.MaxStack = 3
			case "volatile field":
				obj.Fields[0].AccessFlags |= 0x0040
			case "moved field":
				writes[obj.GetClassName()+"\x00seed\x00J"] = true
			case "budget":
				remaining = 0
			}
			d := &ClassObjectDumper{obj: obj}
			value, ok := d.constructorReceiverReadOnlyMethod(obj, member, opcode, writes, &remaining)
			want := variant == "private" || variant == "final" || variant == "harmless nop"
			if ok != want || ok && value.kind != 'J' {
				t.Fatalf("accepted=%v value=%+v want %v", ok, value, want)
			}
		})
	}
}

func TestAdversarialConstructorReadOnlyCallStillRequiresClosedFinalizer(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "ReadonlyCall.java")
	source := `class ReadonlyCall {long seed;ReadonlyCall(long n){seed=n;seed=probe();}private long probe(){return seed;}}`
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "ReadonlyCall.class"))
	if err != nil {
		t.Fatal(err)
	}
	for _, closed := range []bool{false, true} {
		obj, err := Parse(bytes.Clone(raw))
		if err != nil {
			t.Fatal(err)
		}
		var code *CodeAttribute
		for _, m := range obj.Methods {
			name, _ := obj.getUtf8(m.NameIndex)
			if name == "<init>" {
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						code = c
					}
				}
			}
		}
		if code == nil {
			t.Fatal("original constructor absent")
		}
		decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
		if err := decoder.ParseOpcode(); err != nil {
			t.Fatal(err)
		}
		d := &ClassObjectDumper{obj: obj, constructorReceiverFinalizerSilent: closed, FuncCtx: &class_context.ClassContext{}}
		d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
		remaining := 8192
		if got := d.constructorReceiverEffects(obj, code, constructorMotionOps(decoder), "(J)V", map[string]bool{}, map[string]bool{}, &remaining, 0); got != closed {
			t.Fatalf("closedfinalizer=%v proof=%v", closed, got)
		}
	}
}
