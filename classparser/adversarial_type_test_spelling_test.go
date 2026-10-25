package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// Type-test operands and branch bindings come from the bytecode. Unrelated
// declaration spellings must never select a different test or constructor.
func TestAdversarialTypeTestRetainsTargetAcrossDeclarationSpellings(t *testing.T) {
	const fixture = `
class AnnotatedMethod {final int id;AnnotatedMethod(int id){this.id=id;}}
class AnnotatedField {final int id;AnnotatedField(int id){this.id=id;}}
abstract class Property {Object input;int id;}
class DispatchTrace {static String trace="";static boolean fail;static final RuntimeException FAILURE=new IllegalStateException("same");static void enter(String arm){trace+=arm;if(fail)throw FAILURE;}}
class MethodProperty extends Property {MethodProperty(AnnotatedMethod value){DispatchTrace.enter("M");input=value;id=value.id;}}
class FieldProperty extends Property {FieldProperty(AnnotatedField value){DispatchTrace.enter("F");input=value;id=value==null?-1:value.id;}}
public class TypeDispatch {
 public Property select(int a,int b,int c,int d,Object var5){Property result;if(var5 instanceof AnnotatedMethod){result=new MethodProperty((AnnotatedMethod)var5);}else{result=new FieldProperty((AnnotatedField)var5);}return result;}
}
class TypeDispatchDriver {public static void main(String[]args){TypeDispatch owner=new TypeDispatch();Object[] inputs={new AnnotatedMethod(7),new AnnotatedField(11),null,new Object()};int rows=0;
 for(int i=0;i<inputs.length;i++)for(boolean fail:new boolean[]{false,true}){DispatchTrace.trace="";DispatchTrace.fail=fail;Property result=null;Throwable error=null;try{result=owner.select(1,2,3,4,inputs[i]);}catch(Throwable t){error=t;}
  String trace=i==0?"M":i<3?"F":"";if(!DispatchTrace.trace.equals(trace))throw new AssertionError("effects "+i+":"+DispatchTrace.trace);
  if(i==3){if(!(error instanceof ClassCastException))throw new AssertionError("cast boundary",error);}else if(fail){if(error!=DispatchTrace.FAILURE)throw new AssertionError("failure identity",error);}else{if(error!=null||result.input!=inputs[i]||result.id!=(i==0?7:i==1?11:-1)||(i==0)!=(result instanceof MethodProperty))throw new AssertionError("binding/result",error);}rows++;
 }System.out.println(rows+":type-test:binding:identity:effects");}}
`
	modern, _ := t04Tools(t)
	for _, renamed := range []bool{false, true} {
		name := "collision"
		text := fixture
		if renamed {
			name = "renamed"
			text = strings.NewReplacer("AnnotatedMethod", "CallableToken", "AnnotatedField", "StoredToken", "MethodProperty", "CallableResult", "FieldProperty", "StoredResult").Replace(text)
		}
		t.Run(name, func(t *testing.T) {
			for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
				t.Run(string(profile), func(t *testing.T) {
					compiler := modern
					if profile == NativeJavac8 {
						compiler = os.Getenv("JAVA8_JAVAC")
						if compiler == "" {
							t.Fatal("native compiler required")
						}
					}
					compile := func(debug string) map[string][]byte {
						return compileLambdaParameterMetadataFixture(t, compiler, profile, text, "TypeDispatch", debug, false)
					}
					testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{"TypeDispatch"}, "TypeDispatchDriver", "8:type-test:binding:identity:effects\n", nil, nil, []string{"normal", "no-source-rewrites"})
				})
			}
		})
	}
}
