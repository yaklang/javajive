package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/internal/workbudget"
)

const checkedInputFixture = `class InputOwner {
 final Long value; InputOwner(Long value){this.value=value;}
 private Long read(){InputEffects.calls++;if(value==null)throw InputEffects.failure;return value;}
 static class Caller {static java.util.function.Function<InputOwner,Long> factory(){return InputOwner::read;}}
}
class InputEffects {static int calls;static final IllegalStateException failure=new IllegalStateException("identity");}
class InputDriver {public static void main(String[]args)throws Exception{
 java.util.function.Function raw=InputOwner.Caller.factory();
 if(InputEffects.calls!=0)throw new AssertionError("factory effect");
 Long value=Long.valueOf(123456789123L);InputOwner owner=new InputOwner(value);
 if(raw.apply(owner)!=value||InputEffects.calls!=1)throw new AssertionError("result identity");
 try{raw.apply(new Object());throw new AssertionError("missing cast");}catch(ClassCastException expected){}
 if(InputEffects.calls!=1)throw new AssertionError("effect before cast");
 try{raw.apply(null);throw new AssertionError("missing receiver failure");}catch(NullPointerException expected){}
 if(InputEffects.calls!=1)throw new AssertionError("effect before null check");
 try{raw.apply(new InputOwner(null));throw new AssertionError("missing user failure");}catch(IllegalStateException expected){if(expected!=InputEffects.failure)throw new AssertionError("exception identity");}
 java.lang.reflect.Method impl=null;for(java.lang.reflect.Method m:InputOwner.Caller.class.getDeclaredMethods()){if(m.isSynthetic()){if(impl!=null)throw new AssertionError("extra implementation");impl=m;}}
 if(impl==null||impl.getParameterTypes().length!=1||impl.getParameterTypes()[0]!=Object.class)throw new AssertionError("physical entry ABI");impl.setAccessible(true);
 if(impl.invoke(null,owner)!=value)throw new AssertionError("direct implementation identity");
 try{impl.invoke(null,new Object());throw new AssertionError("missing implementation cast");}catch(java.lang.reflect.InvocationTargetException e){if(!(e.getCause() instanceof ClassCastException))throw e;}
 if(InputEffects.calls!=3)throw new AssertionError("call count");System.out.println("input:cast:null:effect:identity:physical-ABI");
}}`

func TestAdversarialLambdaCheckedInputCompletionPreservesInvocationAndRejectsDrift(t *testing.T) {
	javac, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original required")
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			originals := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, checkedInputFixture, "InputOwner", debug, false)
			source := strings.Replace(checkedInputFixture, "return InputOwner::read;", "return (InputOwner receiver) -> receiver.read();", 1)
			fresh := compileLambdaParameterMetadataFixture(t, javac, ModernJavac, source, "InputOwner", debug, false)
			const key = "InputOwner$Caller.class"
			original := originals[key]
			compiled := fresh[key]
			run := func(t *testing.T, files map[string][]byte) {
				t.Helper()
				dir := sourceArtifactWriteFiles(t, files)
				log, err := enumFreshOracleRun(t, java, "-ea", dir, "InputDriver")
				if err != nil || string(log) != "input:cast:null:effect:identity:physical-ABI\n" {
					t.Fatal("original binary oracle", err, string(log))
				}
			}
			run(t, originals)
			find := func(o *ClassObject) *MemberInfo {
				for _, m := range o.Methods {
					n, _ := sourceBridgeUTF8(o, m.NameIndex)
					if strings.HasPrefix(n, "lambda$") {
						return m
					}
				}
				t.Fatal("missing original implementation")
				return nil
			}
			want, got := mustParseLowering(t, original), mustParseLowering(t, compiled)
			om, fm := find(want), find(got)
			on, _ := sourceBridgeUTF8(want, om.NameIndex)
			od, _ := sourceBridgeUTF8(want, om.DescriptorIndex)
			fn, _ := sourceBridgeUTF8(got, fm.NameIndex)
			fd, _ := sourceBridgeUTF8(got, fm.DescriptorIndex)
			plan := &sourceLambdaMetadataPlan{owner: want.GetClassName(), originalHash: sha256.Sum256(original), sourceHash: sha256.Sum256([]byte(source)), methods: []sourceLambdaMetadataMethod{{sourceLambdaMethodIdentity{on, od}, sourceLambdaMethodIdentity{fn, fd}}}}
			for _, variant := range []string{"control", "stale original", "unpublished source", "wrong descriptor", "original missing cast", "receiver", "extra effect", "handler", "max stack", "max locals", "changed target", "renamed factory", "work", "output"} {
				t.Run(variant, func(t *testing.T) {
					o := mustParseLowering(t, bytes.Clone(original))
					g := mustParseLowering(t, bytes.Clone(compiled))
					m := find(g)
					code := constructorLoweringCode(m)
					p := *plan
					p.methods = append([]sourceLambdaMetadataMethod(nil), plan.methods...)
					var work *workbudget.Budget
					switch variant {
					case "stale original":
						p.originalHash = [32]byte{}
					case "unpublished source":
						p.sourceHash = [32]byte{}
					case "wrong descriptor":
						p.methods[0].generated.descriptor = "(Ljava/lang/String;)Ljava/lang/Long;"
					case "original missing cast":
						c := constructorLoweringCode(find(o))
						c.Code[1] = core.OP_INSTANCEOF
					case "receiver":
						code.Code[0] = core.OP_ACONST_NULL
					case "extra effect":
						code.Code = append([]byte{core.OP_ACONST_NULL, core.OP_POP}, code.Code...)
					case "handler":
						code.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 1, HandlerPc: uint16(len(code.Code) - 1)}}
					case "max stack":
						code.MaxStack++
					case "max locals":
						code.MaxLocals++
					case "changed target":
						code.Code[len(code.Code)-4] = core.OP_INVOKEVIRTUAL
					case "renamed factory":
						for _, method := range g.Methods {
							n, _ := sourceBridgeUTF8(g, method.NameIndex)
							if n == "factory" {
								method.NameIndex = sourceBridgePoolString(t, g, "elsewhere")
							}
						}
					case "work":
						work = workbudget.New(nil, workbudget.Limits{MaxRequestWork: 1})
					case "output":
						work = workbudget.New(nil, workbudget.Limits{MaxOutputBytes: 1})
					}
					enumPortFixCodeLength(t, g, code)
					inputOriginal, inputCompiled := o.Bytes(), g.Bytes()
					if variant == "original missing cast" {
						p.originalHash = sha256.Sum256(inputOriginal)
					}
					before := bytes.Clone(inputCompiled)
					out, normalized, err := lowerLambdaCheckedInputs(inputOriginal, inputCompiled, &p, work)
					if !bytes.Equal(before, inputCompiled) {
						t.Fatal("mutated caller bytes")
					}
					if variant != "control" {
						if err == nil || out != nil || normalized != nil {
							t.Fatal("unproved input rewrite", err)
						}
						return
					}
					if err != nil || normalized == nil {
						t.Fatal("checked lowering", err)
					}
					out, err = restoreLambdaMethodMetadata(inputOriginal, out, normalized, nil)
					if err != nil {
						t.Fatal("metadata completion", err)
					}
					files := map[string][]byte{}
					for n, b := range originals {
						files[n] = b
					}
					files[key] = out
					run(t, files)
				})
			}
		})
	}
}
