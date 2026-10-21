package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// A local captured by one factory and a parameter captured by its sibling
// must keep their own original source bindings through the final writer.
// The binary caller observes both factory operands and invocation effects.
const localLambdaEmissionFixture = `import java.util.function.Function;
import java.util.function.Predicate;
class EmissionEffects {
 static String trace="";static final RuntimeException failure=new IllegalStateException("seed");
 static Object require(Object seed){trace+="R";if(seed==null)throw failure;return seed;}
 static void mark(String name){trace+=name;}
}
public class LocalEmissionOwner {
 public static Object read(Object seed,Object token,boolean first){
  Object selected=EmissionEffects.require(seed);
  Function<Object,Object> value=input->{EmissionEffects.mark("F");return input==selected?token:selected;};
  Predicate<Object> same=input->{EmissionEffects.mark("P");return input==token;};
  if(first){return same.test(selected)?value.apply(selected):value.apply(token);}
  return same.test(token)?value.apply(token):value.apply(selected);
 }
}
class EmissionDriver {
 static final boolean CHECKED=false,ARRAY=false,ALLOW_NULL=false;
 public static void main(String[]args){int rows=0;Object a=new Object(),b=new Object();
  for(Object seed:new Object[]{null,a,b,new Object(),new int[]{1}})for(Object token:new Object[]{null,a,b})for(boolean first:new boolean[]{false,true}){
   EmissionEffects.trace="";
   boolean seedFailure=seed==null&&!ALLOW_NULL,castFailure=CHECKED&&seed!=null&&!(ARRAY?seed instanceof Object[]:seed instanceof String);
   try{Object result=LocalEmissionOwner.read(seed,token,first);Object input=first&&seed==token?seed:token;Object expected=input==seed?token:seed;if(seedFailure||castFailure||result!=expected)throw new AssertionError("capture identity");}
   catch(RuntimeException error){if(seedFailure?error!=EmissionEffects.failure:!castFailure||!(error instanceof ClassCastException))throw new AssertionError("failure identity",error);}
   if(!EmissionEffects.trace.equals(seedFailure||castFailure?"R":"RPF"))throw new AssertionError("capture evaluation order:"+EmissionEffects.trace);rows++;
  }
  System.out.println(rows+":local:factory:identity:effects");
 }
}`

func TestAdversarialLambdaLocalEmissionKeepsFactoryScope(t *testing.T) {
	compiler := os.Getenv("JAVA8_JAVAC")
	if compiler == "" {
		t.Skip("JAVA8_JAVAC required for original implementation parameter metadata")
	}
	for _, shape := range []string{"straight", "guarded", "checkcast", "array checkcast", "nullable checkcast"} {
		for _, owner := range []string{"LocalEmissionOwner", "ChangedEmissionOwner"} {
			t.Run(shape+"/"+owner, func(t *testing.T) {
				fixture := strings.ReplaceAll(localLambdaEmissionFixture, "LocalEmissionOwner", owner)
				if shape == "guarded" {
					fixture = strings.Replace(fixture, "Object selected=EmissionEffects.require(seed);", "Object selected=EmissionEffects.require(seed);if(selected==token){EmissionEffects.mark(\"M\");}else{EmissionEffects.mark(\"N\");}", 1)
					fixture = strings.Replace(fixture, `seedFailure||castFailure?"R":"RPF"`, `seedFailure||castFailure?"R":seed==token?"RMPF":"RNPF"`, 1)
				}
				if strings.Contains(shape, "checkcast") {
					declaration, seeds := "String selected=(String)EmissionEffects.require(seed);", `Object a=new String("a"),b=new String("b");`
					if shape == "array checkcast" {
						declaration, seeds = "Object[] selected=(Object[])EmissionEffects.require(seed);", "Object a=new Object[]{new Object()},b=new Object[]{new Object()};"
						fixture = strings.Replace(fixture, "ARRAY=false", "ARRAY=true", 1)
					}
					fixture = strings.Replace(fixture, "Object selected=EmissionEffects.require(seed);", declaration, 1)
					fixture = strings.Replace(fixture, "Object a=new Object(),b=new Object();", seeds, 1)
					fixture = strings.Replace(fixture, "CHECKED=false", "CHECKED=true", 1)
					if shape == "nullable checkcast" {
						fixture = strings.Replace(fixture, "if(seed==null)throw failure;", "", 1)
						fixture = strings.Replace(fixture, "ALLOW_NULL=false", "ALLOW_NULL=true", 1)
					}
				}
				testNativeIndependentCompilerFamilyWithParameterFlag(t, func(debug string) map[string][]byte {
					return compileLambdaParameterMetadataFixture(t, compiler, NativeJavac8, fixture, owner, debug)
				}, NativeJavac8, compiler, []string{owner}, "EmissionDriver", "30:local:factory:identity:effects\n", nil, nil, []string{"normal"}, true, nativeLexicalExactSignatures, func(t *testing.T, name string, original, rebuilt []byte) {
					if want, got := lambdaParameterMetadataTables(t, original), lambdaParameterMetadataTables(t, rebuilt); want != got {
						t.Fatalf("factory parameter names or modifiers changed in %s\n%s\n%s", name, want, got)
					}
				})
			})
		}
	}
}
