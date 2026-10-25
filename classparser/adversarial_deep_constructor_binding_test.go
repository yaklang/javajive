package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// A nested constructor's hidden instance is identified by the original member
// edge and its initialization packet, never by javac's depth-dependent field
// spelling. The immutable driver checks both generic reflection and effects.
const deepConstructorBindingFixture = `class SuffixEffects {static int calls;static boolean fail;static final RuntimeException error=new IllegalStateException("identity");}
class SuffixOwner {
 OPEN
 class Member {
  final java.util.List<? extends Number> items;
  protected Member(java.util.List<? extends Number> items){BODY}
  DELEGATE
  Object items(){return items;}
 }
 CLOSE
}
class SuffixDriver {public static void main(String[]args)throws Exception {
 Class<?> member=Class.forName("BINARY");Class<?> outer=Class.forName("OUTER");
 java.lang.reflect.Constructor<?> ctor=member.getDeclaredConstructor(outer,java.util.List.class);
 java.lang.reflect.Type[] ps=ctor.getGenericParameterTypes();
 if(ps.length!=1||!(ps[0] instanceof java.lang.reflect.ParameterizedType))throw new AssertionError("hidden parameter or lost payload signature");
 java.lang.reflect.ParameterizedType list=(java.lang.reflect.ParameterizedType)ps[0];
 java.lang.reflect.WildcardType wildcard=(java.lang.reflect.WildcardType)list.getActualTypeArguments()[0];
 if(list.getRawType()!=java.util.List.class||wildcard.getLowerBounds().length!=0||wildcard.getUpperBounds().length!=1||wildcard.getUpperBounds()[0]!=Number.class)throw new AssertionError("payload binding");
 int rows=0;for(java.util.List<? extends Number> input:java.util.Arrays.<java.util.List<? extends Number>>asList(null,java.util.Arrays.<Number>asList(null,Long.MIN_VALUE,Double.NaN))){
  for(boolean fail:new boolean[]{false,true}){SuffixEffects.calls=0;SuffixEffects.fail=fail;Throwable error=null;Object value=null;
   try{value=NEW_MEMBER;}catch(Throwable e){error=e;}
   if(SuffixEffects.calls!=1||error!=(fail?SuffixEffects.error:null))throw new AssertionError("effects/order/identity",error);
   if(error==null&&member.getDeclaredMethod("items").invoke(value)!=input)throw new AssertionError("payload identity");rows++;
  }
 }System.out.println(rows+":deep:constructor:binding:effects");}}
`

func TestAdversarialDeepMemberConstructorRetainsDeclaredGenericPayload(t *testing.T) {
	t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
	for _, depth := range []int{1, 2, 3} {
		for _, delegate := range []bool{false, true} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(fmt.Sprintf("depth%d/delegate%v/%s", depth, delegate, debug), func(t *testing.T) {
					open, close, outer, construction := "", "", "SuffixOwner", "new SuffixOwner()"
					for i := 1; i < depth; i++ {
						name := fmt.Sprintf("Layer%d", i)
						open += "class " + name + " {"
						close += "}"
						outer += "$" + name
						construction += ".new " + name + "()"
					}
					body := "this.items=items;SuffixEffects.calls++;if(SuffixEffects.fail)throw SuffixEffects.error;"
					delegation := ""
					if delegate {
						delegation = "private Member(java.util.List<? extends Number> items,long seed){" + body + "}"
						body = "this(items,7L);"
					}
					source := strings.NewReplacer("OPEN", open, "CLOSE", close, "BODY", body, "DELEGATE", delegation, "BINARY", outer+"$Member", "OUTER", outer, "NEW_MEMBER", construction+".new Member(input)").Replace(deepConstructorBindingFixture)
					constructorSuffixFamilyRoundTrip(t, source, debug, []string{outer + "$Member"}, "4:deep:constructor:binding:effects\n")
				})
			}
		}
	}
}
