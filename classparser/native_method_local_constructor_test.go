package javaclassparser

import (
	"context"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeMethodLocalDefaultConstructorRequiresPhysicalCaptureProtocol(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, `class LocalCtorOwner {Object make(final long seed,final Object token){class Entry {long get(){return seed;}Object token(){return token;}}return new Entry();}}`, debug)
			for _, scenario := range []string{"original", "wrong owner", "capture not synthetic", "capture not final", "wrong parameter category", "missing capture store", "duplicate capture field", "extra ordinary field", "missing enclosing parameter", "wrong super call", "missing return", "small stack", "small locals", "oversize frames", "handler", "budget", "canceled"} {
				t.Run(scenario, func(t *testing.T) {
					root, e := Parse(append([]byte(nil), files["LocalCtorOwner.class"]...))
					if e != nil {
						t.Fatal(e)
					}
					local, e := Parse(append([]byte(nil), files["LocalCtorOwner$1Entry.class"]...))
					if e != nil {
						t.Fatal(e)
					}
					var ctor *MemberInfo
					var code *CodeAttribute
					for _, m := range local.Methods {
						n, _ := sourceBridgeUTF8(local, m.NameIndex)
						if n == "<init>" {
							ctor = m
							for _, a := range m.Attributes {
								if c, ok := a.(*CodeAttribute); ok {
									code = c
								}
							}
						}
					}
					if ctor == nil || code == nil {
						t.Fatal("original constructor")
					}
					decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(local.ConstantPool, i) })
					if e := decoder.ParseOpcode(); e != nil {
						t.Fatal(e)
					}
					ops := constructorMotionOps(decoder)
					if len(ops) != 12 || len(local.Fields) != 3 {
						t.Fatalf("original three capture stores + default parent/return: %d ops %d fields", len(ops), len(local.Fields))
					}
					var work *workbudget.Budget
					switch scenario {
					case "wrong owner":
						root = local
					case "capture not synthetic":
						local.Fields[0].AccessFlags &^= 0x1000
					case "capture not final":
						local.Fields[0].AccessFlags &^= 0x10
					case "wrong parameter category":
						for _, op := range ops {
							if op.Instr.OpCode == core.OP_LLOAD_2 {
								code.Code[op.CurrentOffset] = byte(core.OP_ALOAD_2)
								break
							}
						}
					case "missing capture store":
						p := int(ops[2].CurrentOffset)
						for i := p; i < p+3; i++ {
							code.Code[i] = byte(core.OP_NOP)
						}
					case "duplicate capture field":
						local.Fields = append(local.Fields, local.Fields[0])
					case "extra ordinary field":
						copy := *local.Fields[0]
						copy.AccessFlags = 0x10
						local.Fields = append(local.Fields, &copy)
					case "missing enclosing parameter":
						p := int(ops[1].CurrentOffset)
						code.Code[p] = byte(core.OP_ACONST_NULL)
					case "wrong super call":
						p := int(ops[len(ops)-2].CurrentOffset)
						code.Code[p] = byte(core.OP_INVOKEVIRTUAL)
					case "missing return":
						code.Code[ops[len(ops)-1].CurrentOffset] = byte(core.OP_NOP)
					case "small stack":
						code.MaxStack = 1
					case "small locals":
						code.MaxLocals = 1
					case "oversize frames":
						code.MaxStack, code.MaxLocals = 65535, 65535
					case "handler":
						code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: uint16(len(code.Code)), HandlerPc: 0}}
					case "budget":
						work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
					case "canceled":
						ctx, cancel := context.WithCancel(context.Background())
						cancel()
						work = workbudget.New(ctx, workbudget.Limits{})
					}
					proof, known := originalMethodLocalDefaultConstructor(local, root, work)
					if known != (scenario == "original") {
						t.Fatalf("default capture protocol known=%v proof=%+v", known, proof)
					}
					if known && (len(proof.captures) != 3 || proof.enclosingField == "" || proof.captures[proof.enclosingField] != 0 || proof.delegateOwner != "java/lang/Object" || proof.delegatePC != int(ops[len(ops)-2].CurrentOffset)) {
						t.Fatalf("wrong physical default constructor %+v", proof)
					}
				})
			}
		})
	}
}

func TestNativeMethodLocalDefaultConstructorKeepsStaticAndFinalScopes(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, `class LocalScopeFactOwner<T>{static <T> Object make(final long n,final T token){class Entry{long n(){return n;}T token(){return token;}}return new Entry();}<T> Object finalMake(final long n,final T token){final class Entry{long n(){return n;}T token(){return token;}}return new Entry();}}`, debug)
			for _, spec := range []struct {
				binary   string
				static   bool
				captures int
			}{{"LocalScopeFactOwner$1Entry", true, 2}, {"LocalScopeFactOwner$2Entry", false, 3}} {
				t.Run(spec.binary, func(t *testing.T) {
					outer, e := Parse(append([]byte(nil), files["LocalScopeFactOwner.class"]...))
					if e != nil {
						t.Fatal(e)
					}
					obj, e := Parse(append([]byte(nil), files[spec.binary+".class"]...))
					if e != nil {
						t.Fatal(e)
					}
					proof, known := originalMethodLocalDefaultConstructor(obj, outer, nil)
					if !known || len(proof.captures) != spec.captures || (proof.enclosingField == "") != spec.static {
						t.Fatalf("wrong static/final/default capture role %+v known=%v", proof, known)
					}
				})
			}
		})
	}
}
