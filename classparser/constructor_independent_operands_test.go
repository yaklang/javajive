package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
)

func TestAdversarialConstructorIndependentOperandEvidenceAndFailureBoundary(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	source := `class IndependentStatic {static Object seed;Object value;IndependentStatic(){value=seed;}}
class IndependentCast {String value;IndependentCast(Object x){value=(String)x;}}
class IndependentInstance {boolean value;IndependentInstance(Object x){value=x instanceof String;}}
class IndependentPrimitiveArray {int[] value;IndependentPrimitiveArray(int n){value=new int[n];}}
class IndependentReferenceArray {Object[] value;IndependentReferenceArray(int n){value=new Object[n];}}
class IndependentFresh {Object value;IndependentFresh(Object x){value=new StringBuilder();}}
class IndependentFreshArgument {Object value;IndependentFreshArgument(Object x){value=new StringBuilder((String)x);}}
class IndependentFreshRoot {IndependentFreshRoot(Object x){}}
class IndependentFreshBefore extends IndependentFreshRoot {IndependentFreshBefore(){super(new StringBuilder());}}
class IndependentRoot {IndependentRoot(String x){}}
class IndependentBefore extends IndependentRoot {IndependentBefore(Object x){super((String)x);}}
`
	file := filepath.Join(dir, "IndependentEvidence.java")
	if err := os.WriteFile(file, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	for _, debug := range []string{"-g", "-g:none"} {
		if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", dir, file).CombinedOutput(); err != nil {
			t.Fatalf("compile: %v\n%s", err, out)
		}
		resolve := func(name string) ([]byte, bool) {
			raw, err := os.ReadFile(filepath.Join(dir, name+".class"))
			return raw, err == nil
		}
		for _, tc := range []struct {
			name, desc string
			opcode     int
		}{
			{"IndependentFresh", "(Ljava/lang/Object;)V", core.OP_NEW}, {"IndependentFreshBefore", "()V", core.OP_NEW}, {"IndependentFreshArgument", "(Ljava/lang/Object;)V", core.OP_NEW}, {"IndependentStatic", "()V", core.OP_GETSTATIC}, {"IndependentCast", "(Ljava/lang/Object;)V", core.OP_CHECKCAST},
			{"IndependentInstance", "(Ljava/lang/Object;)V", core.OP_INSTANCEOF}, {"IndependentPrimitiveArray", "(I)V", core.OP_NEWARRAY},
			{"IndependentReferenceArray", "(I)V", core.OP_ANEWARRAY}, {"IndependentBefore", "(Ljava/lang/Object;)V", core.OP_CHECKCAST},
		} {
			for _, variant := range []string{"closed finalizer", "open finalizer", "bad operand bytes", "bad constant kind", "receiver operand", "wrong new owner", "fresh local alias", "receiver passed to fresh constructor", "budget"} {
				t.Run(debug+"/"+tc.name+"/"+variant, func(t *testing.T) {
					raw, _ := resolve(tc.name)
					obj, err := Parse(raw)
					if err != nil {
						t.Fatal(err)
					}
					d := &ClassObjectDumper{obj: obj, foldSiblingResolver: resolve, FuncCtx: &class_context.ClassContext{}, constructorReceiverFinalizerSilent: variant != "open finalizer"}
					d.FuncCtx.InvocationMetadata = d.buildInvocationMetadata()
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
						t.Fatal("missing original constructor")
					}
					decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
					if err := decoder.ParseOpcode(); err != nil {
						t.Fatal(err)
					}
					ops := constructorMotionOps(decoder)
					found := false
					for index, op := range ops {
						if op.Instr.OpCode != tc.opcode {
							continue
						}
						found = true
						switch variant {
						case "bad operand bytes":
							op.Data = []byte{0}
						case "bad constant kind":
							if tc.opcode == core.OP_NEWARRAY {
								op.Data = []byte{12}
							} else {
								cp := core.Convert2bytesToInt(op.Data)
								obj.ConstantPool[cp-1] = &ConstantUtf8Info{Value: "not a symbolic field or class"}
							}
						case "fresh local alias":
							if tc.name != "IndependentFresh" {
								t.Skip("needs reference parameter slot")
							}
							// NEW, DUP, ASTORE_1, ALOAD_1, <init>: initialization
							// must clear both the stack alias and the local alias.
							extra := []*core.OpCode{
								{Instr: &core.Instruction{OpCode: core.OP_ASTORE_1}, CurrentOffset: 60000},
								{Instr: &core.Instruction{OpCode: core.OP_ALOAD_1}, CurrentOffset: 60001},
							}
							at := index + 2
							ops = append(ops[:at], append(extra, ops[at:]...)...)
						case "receiver passed to fresh constructor":
							if tc.name != "IndependentFreshArgument" {
								t.Skip("needs fresh constructor argument")
							}
							for j := index + 1; j < len(ops); j++ {
								if ops[j].Instr.OpCode == core.OP_ALOAD_1 {
									copy := *ops[j].Instr
									copy.OpCode = core.OP_ALOAD_0
									ops[j].Instr = &copy
									break
								}
							}
						case "wrong new owner":
							if tc.opcode != core.OP_NEW {
								t.Skip("requires distinct allocation")
							}
							op.Data = []byte{byte(obj.ThisClass >> 8), byte(obj.ThisClass)}
						case "receiver operand":
							if tc.opcode == core.OP_GETSTATIC || tc.opcode == core.OP_NEW {
								t.Skip("static read has no receiver operand")
							}
							if index == 0 {
								t.Fatal("missing operand load")
							}
							load := *ops[0].Instr
							load.OpCode = core.OP_ALOAD_0
							ops[index-1].Instr = &load
							ops[index-1].Data = nil
						}
						break
					}
					if !found {
						t.Fatal("missing original operation")
					}
					remaining := 512
					if variant == "budget" {
						remaining = 0
					}
					want := variant == "fresh local alias" || variant == "closed finalizer" || variant == "open finalizer" && (tc.name == "IndependentBefore" || tc.name == "IndependentFreshBefore")
					if got := d.constructorReceiverEffects(obj, code, ops, tc.desc, map[string]bool{}, map[string]bool{}, &remaining, 0); got != want {
						t.Fatalf("proof=%v want=%v", got, want)
					}
				})
			}
		}
	}
}
