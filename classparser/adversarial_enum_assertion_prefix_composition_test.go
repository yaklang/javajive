package javaclassparser

import (
	"fmt"
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The constructor observes the assertion cache, so a source spelling that moves
// its initialization behind the constants cannot pass by merely renaming fields
// and removing a fresh values helper. The untouched original driver observes
// independent lexical assertion status, initialization effects and arithmetic.
const enumAssertionPrefixCompositionFixture = `public class PortOwner {
 public enum Mode {
  FIRST(1){long apply(long x){return x*3+id;}},
  SECOND(2){long apply(long x){return x^id;}};
  final int id;
  Mode(int id){this.id=id;assert Effects.condition(id):Effects.message();Effects.trace+="P"+id;}
  abstract long apply(long x);
  long checked(long x,int width){assert Effects.condition(width):Effects.message();return apply(x);}
 }
}
class Effects {static String trace="";static final RuntimeException payload=new RuntimeException("identity");
 static boolean condition(int width){trace+="C";return width>0&&width<=64;}
 static Object message(){trace+="M";return payload;}}
class PrefixDriver {public static void main(String[] args)throws Exception {
 boolean enabled=Boolean.parseBoolean(args[0]);ClassLoader loader=ClassLoader.getSystemClassLoader();
 loader.setClassAssertionStatus("PortOwner",enabled);loader.setClassAssertionStatus("PortOwner$Mode",!enabled);
 PortOwner.Mode[] values=PortOwner.Mode.values();
 if(!Effects.trace.equals(enabled?"CP1CP2":"P1P2"))throw new AssertionError("assertion cache must precede constant constructors: "+Effects.trace);
 java.lang.reflect.Field flag=PortOwner.Mode.class.getDeclaredField("$assertionsDisabled");
 if(!flag.isSynthetic()||flag.getModifiers()!=0x1018)throw new AssertionError("field identity");
 int rows=0;for(PortOwner.Mode mode:values)for(long x:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(int width:new int[]{-1,0,1,64,65}){
  Effects.trace="";boolean fails=enabled&&(width<=0||width>64);Throwable actual=null;long value=0;
  try{value=mode.checked(x,width);}catch(Throwable e){actual=e;}
  if(fails){if(!(actual instanceof AssertionError)||actual.getCause()!=Effects.payload)throw new AssertionError("exception identity",actual);}
  else {java.math.BigInteger v=java.math.BigInteger.valueOf(x);long expected=(mode==values[0]?v.multiply(java.math.BigInteger.valueOf(3)).add(java.math.BigInteger.ONE):v.xor(java.math.BigInteger.valueOf(2))).longValue();if(actual!=null||value!=expected)throw new AssertionError("arithmetic",actual);}
  if(!Effects.trace.equals(enabled?(fails?"CM":"C"):""))throw new AssertionError("conditional effects");rows++;
 }
 PortOwner.Mode[] copy=PortOwner.Mode.values();copy[0]=null;if(PortOwner.Mode.values()[0]!=PortOwner.Mode.FIRST||PortOwner.Mode.valueOf("SECOND")!=values[1])throw new AssertionError("enum identity");
 System.out.println(rows+":enum:prefix:storage:constructors:arithmetic");
}}
`

func TestAdversarialEnumAssertionCachePrecedesConstantConstructors(t *testing.T) {
	testEnumAssertionPrefixComposition(t, false)
}
func TestAdversarialEnumAssertionStatusWideningKeepsPrefixAndConstructorFrames(t *testing.T) {
	testEnumAssertionPrefixComposition(t, true)
}
func testEnumAssertionPrefixComposition(t *testing.T, wide bool) {
	t.Helper()
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			run := func(dir string) {
				t.Helper()
				for _, mode := range []string{"-ea", "-da"} {
					for _, enabled := range []string{"false", "true"} {
						out, err := enumFreshOracleRun(t, java, mode, dir, "PrefixDriver", enabled)
						if err != nil || string(out) != "50:enum:prefix:storage:constructors:arithmetic\n" {
							t.Fatalf("original-first prefix oracle %s/%s: %v\n%s", mode, enabled, err, out)
						}
					}
				}
			}
			var original map[string][]byte
			owned, fresh, artifact := enumFlatTestInputs(t, debug, enumAssertionPrefixCompositionFixture, func(t *testing.T, files map[string][]byte) {
				original = files
				run(sourceArtifactWriteFiles(t, files))
			})
			if wide {
				root := mustParseLowering(t, fresh["PortOwner$Mode.class"])
				if constructorLoweringCode(enumAssertionInitializer(t, root)).Code[0] != core.OP_LDC {
					t.Fatal("expected narrow compiler status literal")
				}
				for len(root.ConstantPool) < 300 {
					root.ConstantPoolManager.AddUtf8Info(fmt.Sprintf("unused-root-padding-%d", len(root.ConstantPool)))
				}
				fresh["PortOwner$Mode.class"] = root.Bytes()
			}
			names := make([]string, 0, len(owned))
			for name := range owned {
				names = append(names, name)
			}
			sort.Strings(names)
			for _, reverse := range []bool{false, true} {
				t.Run(fmt.Sprintf("reverse=%v", reverse), func(t *testing.T) {
					files := map[string][]byte{}
					for name, raw := range original {
						if !strings.HasPrefix(name, "PortOwner$Mode") {
							files[name] = raw
						}
					}
					out := sourceArtifactWriteFiles(t, files)
					for i := range names {
						at := i
						if reverse {
							at = len(names) - 1 - i
						}
						name := names[at]
						finished, err := artifact.FinishClass(name, fresh[name])
						if err != nil {
							t.Fatalf("complete enum proof composition %s: %v", name, err)
						}
						if wide && name == "PortOwner$Mode.class" {
							result := mustParseLowering(t, finished)
							if constructorLoweringCode(enumAssertionInitializer(t, result)).Code[0] != core.OP_LDC_W {
								t.Fatal("wide status owner relocation not exercised")
							}
						}
						nativeLexicalExactSignatures(t, name, owned[name], finished)
						if err := os.WriteFile(filepath.Join(out, name), finished, 0600); err != nil {
							t.Fatal(err)
						}
					}
					run(out)
				})
			}
		})
	}
}
