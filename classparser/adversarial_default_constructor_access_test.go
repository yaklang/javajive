package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// An implicit constructor inherits the enclosing declaration's exact access,
// whereas an explicit empty constructor may intentionally have different
// access. Exercise all 16 combinations against original JVM reflection and
// initialization effects, with no original owned class on the rebuild path.
func TestAdversarialDefaultConstructorPreservesEveryAccessRelation(t *testing.T) {
	for _, classAccess := range []string{"", "public", "protected", "private"} {
		for i, constructorAccess := range []string{"", "public", "private", "protected"} {
			t.Run(fmt.Sprintf("class=%s/constructor=%s", classAccess, constructorAccess), func(t *testing.T) {
				fixture := fmt.Sprintf(`class AccessBase {
 static String trace=""; static Object token; final Object observed;
 AccessBase(){trace+="P";observed=token;}
}
class AccessOwner {
 %s static class Child extends AccessBase {%s Child(){}}
 static Object make(){return new Child();}
}
class AccessDriver {
 public static void main(String[]args)throws Exception {
  Class<?> type=Class.forName("AccessOwner$Child");
  java.lang.reflect.Constructor<?> ctor=type.getDeclaredConstructor();
  if((ctor.getModifiers()&7)!=%d||ctor.getParameterTypes().length!=0)throw new AssertionError("constructor access");
  int visible=0;for(java.lang.reflect.Constructor<?> c:type.getDeclaredConstructors())if(!c.isSynthetic())visible++;
  if(visible!=1)throw new AssertionError("constructor universe");
  Object shared=new Object();int rows=0;
  for(Object token:new Object[]{null,shared,"value",new String("value")}) {
   AccessBase.trace="";AccessBase.token=token;Object result=AccessOwner.make();
   if(result.getClass()!=type||!(result instanceof AccessBase)||((AccessBase)result).observed!=token||!AccessBase.trace.equals("P"))throw new AssertionError("initialization identity/order");
   rows++;
  }
  System.out.println(rows+":constructor:access:identity:order");
 }
}`, classAccess, constructorAccess, []int{0, 1, 2, 4}[i])
				if classAccess == "protected" {
					fixture = strings.ReplaceAll(fixture, "AccessOwner", "RenamedOwner")
				}
				owner := "AccessOwner"
				if classAccess == "protected" {
					owner = "RenamedOwner"
				}
				testNativeIndependentFamilyFixture(t, fixture, []string{owner}, "AccessDriver", "4:constructor:access:identity:order\n")
			})
		}
	}
}
