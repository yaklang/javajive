package javaclassparser

import (
	"strings"
	"testing"
)

// The original driver deliberately supplies a heap-polluted array. Element's
// first bound is Object: a source-only Element[] view must not add a
// CharSequence[] runtime check, change the selected consumer or copy the array.
const genericArrayParameterFixture = `interface ArrayArgumentContract<Element,Self>{Self consume(Element... values);}
class ArrayArgumentEffects {
 static Object[] payload; static String trace=""; static boolean fail;
 static final RuntimeException error=new IllegalStateException("original");
 static <X> X[] produce(Iterable<? extends X> input){trace+="P";if(fail)throw error;return (X[])payload;}
}
public class ArrayArgumentOwner<Element extends Object & CharSequence> {
 Object[] consume(Element[] values){ArrayArgumentEffects.trace+="C";return values;}
 Object[] consume(Iterable<?> values){throw new AssertionError("wrong overload");}
 Object[] forward(Iterable<? extends Element> input){return consume(ArrayArgumentEffects.produce(input));}
}
class ArrayArgumentDriver {
 public static void main(String[]args){int rows=0;for(Object[] value:new Object[][]{null,new CharSequence[]{"same",new StringBuilder("mutable"),null},new Integer[]{7,null}})for(boolean fail:new boolean[]{false,true}){
  ArrayArgumentEffects.payload=value;ArrayArgumentEffects.trace="";ArrayArgumentEffects.fail=fail;
  ArrayArgumentOwner raw=new ArrayArgumentOwner();
  try{Object[] got=raw.forward(null);if(fail||got!=value||!ArrayArgumentEffects.trace.equals("PC"))throw new AssertionError("identity/order/once");}
  catch(RuntimeException actual){if(!fail||actual!=ArrayArgumentEffects.error||!ArrayArgumentEffects.trace.equals("P"))throw new AssertionError("exception identity/order");}
  rows++;
 }System.out.println(rows+":array-argument:binding:identity:once:pollution");}
}`

func TestAdversarialGenericArrayParameterKeepsErasureAndBinding(t *testing.T) {
	for _, shape := range []string{"plain", "packed", "self", "self-packed", "self-packed-intersection", "self-packed-renamed"} {
		t.Run(shape, func(t *testing.T) {
			fixture := genericArrayParameterFixture
			if strings.Contains(shape, "self") {
				formals := "<Element,Self extends ArrayArgumentOwner<Element,Self>>"
				if strings.Contains(shape, "intersection") {
					formals = "<Element extends Object & CharSequence,Self extends ArrayArgumentOwner<Element,Self>>"
				}
				fixture = strings.Replace(fixture, "<Element extends Object & CharSequence>", formals, 1)
				fixture = strings.Replace(fixture, "public class ArrayArgumentOwner"+formals+" {", "public class ArrayArgumentOwner"+formals+" implements ArrayArgumentContract<Element,Self> {", 1)
				fixture = strings.Replace(fixture, "Object[] consume(Element[] values){ArrayArgumentEffects.trace+=\"C\";return values;}", "Object[] last; public final Self consume(Element[] values){ArrayArgumentEffects.trace+=\"C\";last=values;return (Self)this;}", 1)
				fixture = strings.Replace(fixture, "Object[] forward(", "Self forward(", 1)
				fixture = strings.Replace(fixture, "Object[] got=raw.forward(null);if(fail||got!=value", "Object got=raw.forward(null);if(fail||got!=raw||raw.last!=value", 1)
			}
			if strings.Contains(shape, "packed") {
				fixture = strings.Replace(fixture, "consume(Element[] values)", "consume(Element... values)", 1)
			}
			owner := "ArrayArgumentOwner"
			if strings.Contains(shape, "renamed") {
				owner = "DifferentArrayConsumer"
				fixture = strings.NewReplacer("ArrayArgumentOwner", owner, "Element", "Payload", "Self", "Fluent", "input", "incoming", "values", "items").Replace(fixture)
			}
			testNativePrivateSetterCompiledFixtureWithPolicies(t, owner, "ArrayArgumentDriver", "6:array-argument:binding:identity:once:pollution\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
			}, nil, []string{"normal"}, nativeLexicalExactSignatures)
		})
	}
}
