package javaclassparser

import (
	"os"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

func TestAdversarialStaticReturnSignatureKeepsNullOperandResolution(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		compiler := modern
		if profile == NativeJavac8 {
			compiler = os.Getenv("JAVA8_JAVAC")
			if compiler == "" {
				t.Fatal("native compiler required")
			}
		}
		for _, owner := range []string{"ReturnSubject", "RenamedSubject"} {
			t.Run(string(profile)+"/"+owner, func(t *testing.T) {
				source := strings.ReplaceAll(`public class ReturnSubject {
 public static Class<?> plain(int x){return ReturnFactory.produce(ReturnFactory.first(x),null);}
 public static Class<?> checked(int x){return ReturnFactory.produce(ReturnFactory.first(x),(ReturnToken)null);}
}
class ReturnToken {}
class ReturnFactory {
 static String trace="";static final RuntimeException FAILURE=new IllegalArgumentException("same");
 static int first(int x){trace+="A";if(x<0)throw FAILURE;return x;}
 public static Class<?> produce(int x,ReturnToken token){trace+="B";if(token!=null)throw new AssertionError();if(x==0)throw FAILURE;return x==1?String.class:ReturnToken.class;}
}
class ReturnDriver {public static void main(String[] args){for(int i=-1;i<3;i++)for(boolean checked:new boolean[]{false,true}){ReturnFactory.trace="";try{Class<?> got=checked?ReturnSubject.checked(i):ReturnSubject.plain(i);if(i<1||got!=(i==1?String.class:ReturnToken.class))throw new AssertionError("result");}catch(RuntimeException failure){if(i>0||failure!=ReturnFactory.FAILURE)throw new AssertionError("identity");}if(!ReturnFactory.trace.equals(i<0?"A":"AB"))throw new AssertionError("order");}System.out.println("8:binding:effects:identity");}}
`, "ReturnSubject", owner)
				// The factory is public to give the complete cross-owner source
				// selection proof; keep its binary declaration independent.
				compile := func(debug string) map[string][]byte {
					files := compileLambdaParameterMetadataFixture(t, compiler, profile, source, owner, debug, false)
					factory := mustParseLowering(t, files["ReturnFactory.class"])
					factory.AccessFlags |= 1
					files["ReturnFactory.class"] = factory.Bytes()
					return files
				}
				testNativeIndependentCompilerFamilyFixture(t, compile, profile, compiler, []string{owner}, "ReturnDriver", "8:binding:effects:identity\n", nil, func(t *testing.T, name string, original, fresh []byte) {
					count := func(raw []byte) map[string]int {
						o := mustParseLowering(t, raw)
						out := map[string]int{}
						for _, m := range o.Methods {
							n, _ := sourceBridgeUTF8(o, m.NameIndex)
							if n != "plain" && n != "checked" {
								continue
							}
							c := constructorLoweringCode(m)
							if c == nil {
								t.Fatal("missing Code")
							}
							d := core.NewDecompiler(c.Code, nil)
							if e := d.ParseOpcode(); e != nil {
								t.Fatal(e)
							}
							out[n] = 0
							for _, op := range constructorMotionOps(d) {
								if op.Instr.OpCode == core.OP_CHECKCAST {
									out[n]++
								}
							}
						}
						return out
					}
					a, b := count(original), count(fresh)
					if a["plain"] != 0 || a["checked"] != 1 || b["plain"] != a["plain"] || b["checked"] != a["checked"] {
						t.Fatalf("original/fresh resolution effects %v / %v", a, b)
					}
				})
			})
		}
	}
}
