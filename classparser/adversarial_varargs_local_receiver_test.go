package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const localVarargsReceiverFixture = `import java.util.function.Consumer;
class LocalReceiverTarget {
 static String trace="";static Object[] last;static Object seen;static int made;
 static final RuntimeException failure=new IllegalArgumentException("payload");
 public Object[] append(Object...items){trace+="A";if(items.length!=1||items==last)throw new AssertionError("fresh pack");last=items;seen=items[0];if(items[0]==failure)throw failure;return items;}
 static LocalReceiverTarget create(int mode){made++;trace+="F";if(mode==2)throw failure;return mode==0?null:new LocalReceiverTarget();}
}
public class LocalReceiverOwner {
 public static Consumer<Object> make(int mode){$BODY}
}
class LocalReceiverDriver {public static void main(String[]args){Object token=new Object();int rows=0;
 for(int mode=0;mode<3;mode++)for(Object value:new Object[]{null,token,LocalReceiverTarget.failure}){
  LocalReceiverTarget.trace="";LocalReceiverTarget.made=0;LocalReceiverTarget.seen=null;boolean made=false;Throwable error=null;
  try{Consumer<Object> action=LocalReceiverOwner.make(mode);made=true;LocalReceiverTarget.trace+="M";action.accept(value);}catch(Throwable e){error=e;}
  if(LocalReceiverTarget.made!=1||made!=(mode==1)||!LocalReceiverTarget.trace.equals(mode==1?"FMA":"F"))throw new AssertionError("receiver evaluation/null timing",error);
  if(mode==0){if(!(error instanceof NullPointerException))throw new AssertionError("eager null",error);}
  else if(mode==2||value==LocalReceiverTarget.failure){if(error!=LocalReceiverTarget.failure)throw new AssertionError("exact failure identity",error);}
  else if(error!=null||LocalReceiverTarget.seen!=value)throw new AssertionError("value binding",error);rows++;
 }
 System.out.println(rows+":local:bound:reference:evaluation:identity");}}
`

func TestAdversarialVarargsLocalReceiverRetainsOriginalDefinition(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		compiler := modern
		if profile == NativeJavac8 {
			compiler = os.Getenv("JAVA8_JAVAC")
			if compiler == "" {
				t.Fatal("original native Java 8 compiler required")
			}
		}
		t.Run(string(profile), func(t *testing.T) {
			for _, named := range []bool{false, true} {
				label := "absent"
				if named {
					label = "named"
				}
				t.Run(label, func(t *testing.T) {
					for _, tc := range []struct{ name, body string }{
						{"call result", "LocalReceiverTarget receiver=LocalReceiverTarget.create(mode);return receiver::append;"},
						{"local alias", "LocalReceiverTarget value=LocalReceiverTarget.create(mode);LocalReceiverTarget receiver=value;return receiver::append;"},
						{"joined definition", "LocalReceiverTarget receiver;if(mode==1)receiver=LocalReceiverTarget.create(1);else receiver=LocalReceiverTarget.create(mode);return receiver::append;"},
					} {
						t.Run(tc.name, func(t *testing.T) {
							source := strings.Replace(localVarargsReceiverFixture, "$BODY", tc.body, 1)
							testNativeIndependentCompilerFamilyWithParameterFlag(t, func(debug string) map[string][]byte {
								return compileLambdaParameterMetadataFixture(t, compiler, profile, source, "LocalReceiverOwner", debug, named)
							}, profile, compiler, []string{"LocalReceiverOwner"}, "LocalReceiverDriver", "9:local:bound:reference:evaluation:identity\n", nil, func(t *testing.T, files map[string][]byte) *JarFS {
								return nativeEnumOwnUnitArchive(t, files, profile)
							}, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, named, nativeLexicalExactSignatures, func(t *testing.T, name string, a, b []byte) {
								if x, y := lambdaParameterMetadataTables(t, a), lambdaParameterMetadataTables(t, b); x != y {
									t.Fatalf("original adapter metadata changed %s\n%s\n%s", name, x, y)
								}
							})
						})
					}
				})
			}
		})
	}
}
