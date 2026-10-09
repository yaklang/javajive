package javaclassparser

import (
	"bytes"
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

const closedMethodEvidenceFixture = `class ClosedMethodEvidence {
 int word;Object self;static Object saved;
 private int change(int n){word=n;return word;}
 private Object independent(Object input){word=7;return input;}
 private Object retainSelf(Object input){self=this;word=7;return input;}
 private Object readSelf(){return self;}
 private Object exposeHeap(){self=this;return readSelf();}
 private Object returnSelf(){word=7;Object alias=this;return alias;}
 private void publish(){word=7;saved=this;}
 private int branchPublish(boolean safe){if(safe)return 1;saved=this;return 0;}
 private void cycle(){word=7;cycle();}
 private int loop(int n){int sum=0;for(int i=0;i<n;i++)sum+=i;word=sum;return sum;}
 private long wide(long n){word=7;return n;}
 private double floating(double n){word=7;return n;}
 private byte narrow(int n){word=n;return (byte)n;}
 private boolean booleanWord(int n){word=n;return n!=0;}
}
`

func closedMethodEvidence(t *testing.T, files map[string][]byte, name string) (*ClassObject, *MemberInfo, *CodeAttribute) {
	t.Helper()
	obj, err := Parse(bytes.Clone(files["ClosedMethodEvidence.class"]))
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range obj.Methods {
		n, _ := obj.getUtf8(method.NameIndex)
		if n == name {
			for _, attribute := range method.Attributes {
				if code, ok := attribute.(*CodeAttribute); ok {
					return obj, method, code
				}
			}
		}
	}
	t.Fatal("missing original closed method", name)
	return nil, nil, nil
}

func TestConstructorClosedMethodSharesReceiverStorageAndReturnInvariants(t *testing.T) {
	files := nativeCompileClasses(t, closedMethodEvidenceFixture)
	for _, tc := range []struct {
		name, desc string
		args       []constructorEffectValue
		want       bool
	}{
		{"change", "(I)I", []constructorEffectValue{{kind: 'I', knownInt: true, intWord: 9}}, true},
		{"independent", "(Ljava/lang/Object;)Ljava/lang/Object;", []constructorEffectValue{{kind: 'L'}}, true},
		{"retainSelf", "(Ljava/lang/Object;)Ljava/lang/Object;", []constructorEffectValue{{kind: 'L'}}, true},
		{"exposeHeap", "()Ljava/lang/Object;", nil, false},
		{"returnSelf", "()Ljava/lang/Object;", nil, false},
		{"publish", "()V", nil, false},
		{"branchPublish", "(Z)I", []constructorEffectValue{{kind: 'I'}}, false},
		{"cycle", "()V", nil, false},
		{"loop", "(I)I", []constructorEffectValue{{kind: 'I'}}, true},
		{"wide", "(J)J", []constructorEffectValue{{kind: 'J'}}, true},
		{"floating", "(D)D", []constructorEffectValue{{kind: 'D'}}, true},
		{"narrow", "(I)B", []constructorEffectValue{{kind: 'I', knownInt: true, intWord: 256}}, true},
		{"booleanWord", "(I)Z", []constructorEffectValue{{kind: 'I', knownInt: true, intWord: 2}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			obj, _, _ := closedMethodEvidence(t, files, tc.name)
			d := &ClassObjectDumper{obj: obj, constructorReceiverFinalizerSilent: true}
			remaining := 512
			aliases := &constructorSelfStorageProof{}
			value, accepted := d.constructorReceiverClosedMethod(obj, &values.JavaClassMember{Name: obj.GetClassName(), Member: tc.name, Description: tc.desc}, core.OP_INVOKESPECIAL, map[string]bool{}, map[string]bool{}, &remaining, 0, aliases, tc.args...)
			_, result, err := callbinding.Descriptor(tc.desc)
			if err != nil {
				t.Fatal(err)
			}
			if accepted != tc.want || accepted && (value.kind != constructorEffectType(result).kind || value.knownInt || value.receiver || value.allocation != 0) {
				t.Fatalf("accepted=%v returned=%+v aliases=%+v want=%v", accepted, value, aliases, tc.want)
			}
		})
	}
}

// Change one original evidence boundary per case. These are refusal checks,
// not semantic mutants: malformed classfiles are never counted as JVM kills.
func TestConstructorClosedMethodRejectsIncompleteBindingAndResourceEvidence(t *testing.T) {
	files := nativeCompileClasses(t, closedMethodEvidenceFixture)
	for _, variant := range []string{"private", "final", "open dispatch", "synchronized", "native", "abstract", "static", "wrong owner", "empty member", "wrong descriptor", "wrong opcode", "duplicate declaration", "nil declaration", "missing code", "nil code", "duplicate code", "empty body", "handler", "wrong return", "short locals", "short stack", "moved read", "moved write", "volatile storage", "receiver actual", "uninitialized actual", "open finalizer", "active cycle", "depth", "budget", "work", "memory", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			obj, method, code := closedMethodEvidence(t, files, "change")
			member := &values.JavaClassMember{Name: obj.GetClassName(), Member: "change", Description: "(I)I"}
			args := []constructorEffectValue{{kind: 'I'}}
			opcode, depth, remaining := core.OP_INVOKESPECIAL, 0, 512
			active, writes := map[string]bool{}, map[string]bool{}
			d := &ClassObjectDumper{obj: obj, constructorReceiverFinalizerSilent: true}
			switch variant {
			case "final":
				method.AccessFlags = 0x0010
				opcode = core.OP_INVOKEVIRTUAL
			case "open dispatch":
				method.AccessFlags = 0x0001
			case "synchronized":
				method.AccessFlags |= 0x0020
			case "native":
				method.AccessFlags |= 0x0100
			case "abstract":
				method.AccessFlags |= 0x0400
			case "static":
				method.AccessFlags |= 0x0008
			case "wrong owner":
				member.Name = "OtherOwner"
			case "empty member":
				member.Member = ""
			case "wrong descriptor":
				member.Description = "(I)J"
			case "wrong opcode":
				opcode = core.OP_INVOKESTATIC
			case "duplicate declaration":
				obj.Methods = append(obj.Methods, method)
			case "nil declaration":
				obj.Methods = append(obj.Methods, nil)
			case "missing code":
				method.Attributes = nil
			case "nil code":
				method.Attributes = []AttributeInfo{(*CodeAttribute)(nil)}
			case "duplicate code":
				method.Attributes = append(method.Attributes, code)
			case "empty body":
				code.Code = nil
			case "handler":
				code.ExceptionTable = append(code.ExceptionTable, &ExceptionTableEntry{})
			case "wrong return":
				code.Code[len(code.Code)-1] = byte(core.OP_LRETURN)
			case "short locals":
				code.MaxLocals = 1
			case "short stack":
				code.MaxStack = 0
			case "moved read", "moved write":
				// Distinguish the two effects using the original field's exact
				// CP reference. The value remains a valid JVM word in each body.
				if len(code.Code) != 10 || code.Code[2] != core.OP_PUTFIELD || code.Code[6] != core.OP_GETFIELD {
					t.Fatalf("unexpected original field instructions %v", code.Code)
				}
				if variant == "moved read" {
					code.Code = bytes.Clone(code.Code[5:])
				} else {
					code.Code = append(bytes.Clone(code.Code[:5]), core.OP_ILOAD_1, core.OP_IRETURN)
				}
				writes[obj.GetClassName()+"\x00word\x00I"] = true
			case "volatile storage":
				obj.Fields[0].AccessFlags |= 0x0040
			case "receiver actual":
				member.Description = "(Ljava/lang/Object;)I"
				args[0] = constructorEffectValue{kind: 'L', receiver: true}
			case "uninitialized actual":
				args[0].allocation = 1
			case "open finalizer":
				d.constructorReceiverFinalizerSilent = false
			case "active cycle":
				active[member.Name+"\x00"+member.Member+"\x00"+member.Description] = true
			case "depth":
				depth = 17
			case "budget":
				remaining = 0
			case "work":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
				nativeProofWork(d.Work, 1)
			case "memory":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			value, accepted := d.constructorReceiverClosedMethod(obj, member, opcode, writes, active, &remaining, depth, &constructorSelfStorageProof{}, args...)
			want := variant == "private" || variant == "final"
			if accepted != want || accepted && (value.kind != 'I' || value.knownInt) {
				t.Fatalf("accepted=%v returned=%+v want=%v", accepted, value, want)
			}
		})
	}
}
