package javaclassparser

import (
	"os"
	"testing"
)

const nullableVarargsReferenceFixture = `import java.util.function.Consumer;
class NullableVarargsTarget {
 static String trace="";static Object[] previous;static Object seen;
 public Object[] append(Object...values){trace+="A";if(values.length!=1||values==previous)throw new AssertionError("one fresh ordered pack");previous=values;seen=values[0];return values;}
}
public class NullableReferenceOwner {
 public static Consumer<Object> lazy(NullableVarargsTarget target){return value->target.append(value);}
 public static Consumer<Object> eager(NullableVarargsTarget target){return target::append;}
}
class NullableReferenceDriver {
 public static void main(String[]args){Object shared=new Object();int rows=0;
  for(NullableVarargsTarget target:new NullableVarargsTarget[]{null,new NullableVarargsTarget(),new NullableVarargsTarget()})for(Object value:new Object[]{null,shared})for(int route=0;route<2;route++){
   NullableVarargsTarget.trace="";NullableVarargsTarget.seen=null;Throwable caught=null;boolean made=false;
   try{Consumer<Object> action=route==0?NullableReferenceOwner.lazy(target):NullableReferenceOwner.eager(target);made=true;NullableVarargsTarget.trace+="M";action.accept(value);}catch(Throwable error){caught=error;}
   boolean wantMade=route==0||target!=null;String want=(wantMade?"M":"")+(target!=null?"A":"");
   if(made!=wantMade||!NullableVarargsTarget.trace.equals(want))throw new AssertionError("bound-reference eager check vs nullable captured lambda: "+route+":"+made+":"+NullableVarargsTarget.trace,caught);
   if(target==null){if(!(caught instanceof NullPointerException))throw new AssertionError("original NPE category",caught);}else if(caught!=null||NullableVarargsTarget.seen!=value)throw new AssertionError("value identity",caught);rows++;
  }
  System.out.println(rows+":nullable-lambda:bound-reference:pack:identity");
 }
}`

func TestAdversarialVarargsReferenceMustNotEagerlyCheckNullableLambdaCapture(t *testing.T) {
	modern, _ := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		t.Run(string(profile), func(t *testing.T) {
			compiler := modern
			if profile == NativeJavac8 {
				compiler = os.Getenv("JAVA8_JAVAC")
				if compiler == "" {
					t.Skip("JAVA8_JAVAC required")
				}
			}
			for _, named := range []bool{false, true} {
				label := "absent"
				if named {
					label = "named"
				}
				t.Run(label, func(t *testing.T) {
					testNativeIndependentCompilerFamilyWithParameterFlag(t, func(debug string) map[string][]byte {
						return compileLambdaParameterMetadataFixture(t, compiler, profile, nullableVarargsReferenceFixture, "NullableReferenceOwner", debug, named)
					}, profile, compiler, []string{"NullableReferenceOwner"}, "NullableReferenceDriver", "12:nullable-lambda:bound-reference:pack:identity\n", nil, func(t *testing.T, files map[string][]byte) *JarFS { return nativeEnumOwnUnitArchive(t, files, profile) }, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, named, nativeLexicalExactSignatures, func(t *testing.T, name string, a, b []byte) {
						if x, y := lambdaParameterMetadataTables(t, a), lambdaParameterMetadataTables(t, b); x != y {
							t.Fatalf("original reference/lambda parameter roles changed %s\n%s\n%s", name, x, y)
						}
					})
				})
			}
		})
	}
}
