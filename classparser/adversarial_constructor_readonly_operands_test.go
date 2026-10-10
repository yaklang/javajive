package javaclassparser

import (
	"bytes"
	"context"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

// The untouched parent constructor invokes a nonvirtual method on the fresh
// receiver. Parameters are original independent values; their category-2
// slots must never be confused with THIS or a field being moved across super.
const readonlyOperandsFixture = `
class OperandParent {long seed,value;Object reference;OperandParent(long n,Object token){seed=n;value=SELECT_VALUE;reference=same(3.0,n,token);noEffect(n,token);}SELECT_METHOD private Object same(double ignored,long unused,Object token){return token;}final void noEffect(long ignored,Object unused){}Object capture(){return null;}}
class OperandOwner {final Object token;OperandOwner(Object t){token=t;}final class Child extends OperandParent{Child(long n,Object arg){super(n,arg);}Object capture(){return OperandOwner.this.token;}}OperandParent make(long n,Object arg){return new Child(n,arg);}}
class OperandOracle{static void run(){Object token=new Object(),arg=new Object();int rows=0;for(Object t:new Object[]{null,token})for(Object a:new Object[]{null,arg})for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){OperandParent p=new OperandOwner(t).make(n,a);if(p.value!=n||p.reference!=a||p.capture()!=t)throw new AssertionError("wide operand/capture/identity");rows++;}System.out.println(rows);}}
public class OperandDriver{public static void main(String[] args){OperandOracle.run();}}
`

func TestAdversarialConstructorReadOnlyOperandsRetainOriginalBinding(t *testing.T) {
	for _, rename := range []string{"Operand", "Transport"} {
		for _, subject := range []struct{ name, call, method string }{
			{"wide parameter", "sameLong(7,-0.0,n,token)", "private long sameLong(int unused,double ignored,long x,Object token){return x;}"},
			{"field with independent arguments", "sameLong(token,n)", "final long sameLong(Object unused,long ignored){return seed;}"},
		} {
			t.Run(rename+"/"+subject.name, func(t *testing.T) {
				source := strings.ReplaceAll(readonlyOperandsFixture, "SELECT_VALUE", subject.call)
				source = strings.ReplaceAll(source, "SELECT_METHOD", subject.method)
				source = strings.ReplaceAll(source, "Operand", rename)
				roundTripGenericFlowUnitsClasspath(t, rename+"Driver", source, nil, []string{rename + "Owner", rename + "Owner$Child"}, true, Precision, Compatibility, "legacy")
			})
		}
	}
}

// Every mutated packet is original classfile evidence, never executed. Wide
// second words, THIS, uninitialized values, lost parameters and unknown entry
// effects cannot borrow a receiver-free transport proof.
func TestAdversarialConstructorReadOnlyOperandProofBoundaries(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		files := nativeCompileSourceReleaseClasses(t, map[string]string{"OperandEvidence.java": `class OperandEvidence {long seed;private long choose(int unused,double ignored,long value,Object token){return value;}private Object reference(double ignored,long unused,Object token){return token;}final void noEffect(long unused,Object ignored){}final long field(Object ignored,long unused){return seed;}private int scalar(int value){return value;}}`}, debug, "8")
		for _, variant := range []string{"original wide", "reference identity", "unused void arguments", "field read", "integer fact", "missing actual", "extra actual", "wrong category", "receiver actual", "uninitialized actual", "wrong slot", "receiver load", "wrong opcode", "wrong return", "insufficient locals", "insufficient stack", "virtual", "static", "synchronized", "native", "abstract", "foreign owner", "wrong descriptor", "duplicate method", "missing code", "duplicate code", "extra receiver use", "work", "canceled"} {
			t.Run(debug+"/"+variant, func(t *testing.T) {
				obj, e := Parse(bytes.Clone(files["OperandEvidence.class"]))
				if e != nil {
					t.Fatal(e)
				}
				name, desc, want := "choose", "(IDJLjava/lang/Object;)J", byte('J')
				args := []constructorEffectValue{{kind: 'I', knownInt: true, intWord: 7}, {kind: 'D'}, {kind: 'J'}, {kind: 'L'}}
				switch variant {
				case "reference identity":
					name, desc, want = "reference", "(DJLjava/lang/Object;)Ljava/lang/Object;", 'L'
					args = []constructorEffectValue{{kind: 'D'}, {kind: 'J'}, {kind: 'L'}}
				case "unused void arguments":
					name, desc, want = "noEffect", "(JLjava/lang/Object;)V", 0
					args = []constructorEffectValue{{kind: 'J'}, {kind: 'L'}}
				case "field read":
					name, desc = "field", "(Ljava/lang/Object;J)J"
					args = []constructorEffectValue{{kind: 'L'}, {kind: 'J'}}
				case "integer fact":
					name, desc, want = "scalar", "(I)I", 'I'
					args = []constructorEffectValue{{kind: 'I', knownInt: true, intWord: -17}}
				}
				var method *MemberInfo
				var code *CodeAttribute
				for _, m := range obj.Methods {
					n, _ := sourceBridgeUTF8(obj, m.NameIndex)
					if n == name {
						method = m
						for _, a := range m.Attributes {
							if c, ok := a.(*CodeAttribute); ok {
								code = c
							}
						}
					}
				}
				if method == nil || code == nil {
					t.Fatal("missing original packet")
				}
				member := &values.JavaClassMember{Name: obj.GetClassName(), Member: name, Description: desc}
				opcode := core.OP_INVOKEVIRTUAL
				remaining := 512
				reader := &ClassObjectDumper{obj: obj}
				switch variant {
				case "missing actual":
					args = args[:len(args)-1]
				case "extra actual":
					args = append(args, constructorEffectValue{kind: 'I'})
				case "wrong category":
					args[1].kind = 'J'
				case "receiver actual":
					args[len(args)-1].receiver = true
				case "uninitialized actual":
					args[len(args)-1].allocation = 3
				case "wrong slot", "receiver load", "wrong opcode":
					if len(code.Code) != 3 || code.Code[0] != byte(core.OP_LLOAD) || code.Code[1] != 4 {
						t.Fatalf("unexpected genuine wide operand code %v", code.Code)
					}
					if variant == "wrong slot" {
						code.Code[1] = 5
					}
					if variant == "receiver load" {
						code.Code[0] = byte(core.OP_ALOAD)
						code.Code[1] = 0
					}
					if variant == "wrong opcode" {
						code.Code[0] = byte(core.OP_DLOAD)
					}
				case "wrong return":
					code.Code[len(code.Code)-1] = byte(core.OP_IRETURN)
				case "insufficient locals":
					code.MaxLocals = 6
				case "insufficient stack":
					code.MaxStack = 1
				case "virtual":
					method.AccessFlags = 1
				case "static":
					method.AccessFlags |= 8
				case "synchronized":
					method.AccessFlags |= 0x20
				case "native":
					method.AccessFlags |= 0x100
				case "abstract":
					method.AccessFlags |= 0x400
				case "foreign owner":
					member.Name = "Other"
				case "wrong descriptor":
					member.Description = "(IJLjava/lang/Object;)J"
				case "duplicate method":
					obj.Methods = append(obj.Methods, method)
				case "missing code":
					method.Attributes = nil
				case "duplicate code":
					method.Attributes = append(method.Attributes, code)
				case "extra receiver use":
					code.Code = append([]byte{byte(core.OP_ALOAD_0), byte(core.OP_POP)}, code.Code...)
				case "work":
					reader.Work = workbudget.New(context.Background(), workbudget.Limits{MaxRequestWork: 1, MaxOutputBytes: 1 << 20})
				case "canceled":
					ctx, cancel := context.WithCancel(context.Background())
					cancel()
					reader.Work = workbudget.New(ctx, workbudget.Limits{MaxRequestWork: 10000, MaxOutputBytes: 1 << 20})
				}
				value, ok := reader.constructorReceiverReadOnlyMethod(obj, member, opcode, map[string]bool{}, &remaining, args...)
				positive := variant == "original wide" || variant == "reference identity" || variant == "unused void arguments" || variant == "field read" || variant == "integer fact"
				if ok != positive || ok && value.kind != want {
					t.Fatalf("proof=%v value=%+v want=%v/%c", ok, value, positive, want)
				}
				if variant == "integer fact" && (!value.knownInt || value.intWord != -17) {
					t.Fatalf("lost original actual fact %+v", value)
				}
			})
		}
	}
}
