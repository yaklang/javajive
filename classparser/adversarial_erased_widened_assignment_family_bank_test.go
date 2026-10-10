package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// Ten physical parameter tuples/static-dispatch choices crossed with ten local
// definition-web depths are 100 structures, rather than renamed copies. The
// independent original driver checks ten observations per structure; the
// helpers stay original, so a rebuilt target cannot rewrite its own assertions.
func TestAdversarialErasedWidenedAssignmentHundredStructureBank(t *testing.T) {
	var source strings.Builder
	source.WriteString("class WidenedBankOps {static String trace=\"\";\n")
	for tuple := 0; tuple < 10; tuple++ {
		static := ""
		if tuple%2 == 0 {
			static = "static "
		}
		var extras, sum strings.Builder
		for i := 0; i < tuple; i++ {
			fmt.Fprintf(&extras, ",int p%d", i)
			fmt.Fprintf(&sum, "sum+=%d*p%d;", i+1, i)
		}
		fmt.Fprintf(&source, "%s<N extends Number>N choose%d(Number n,Class<N> type%s){int sum=0;%strace=\"C:%d:\"+sum;return type.cast(n);}\n", static, tuple, extras.String(), sum.String(), tuple)
		fmt.Fprintf(&source, "%sNumber choose%d(Integer n,Object type%s){trace=\"WRONG\";return Integer.valueOf(-1000);}\n", static, tuple, extras.String())
	}
	source.WriteString("}\npublic class WidenedBankOwner {static final WidenedBankOps OPS=new WidenedBankOps();\n")
	for tuple := 0; tuple < 10; tuple++ {
		receiver := "OPS"
		if tuple%2 == 0 {
			receiver = "WidenedBankOps"
		}
		var extraArgs strings.Builder
		for i := 0; i < tuple; i++ {
			fmt.Fprintf(&extraArgs, ",%d", i+2)
		}
		for depth := 1; depth <= 10; depth++ {
			fmt.Fprintf(&source, "public static <T>Object convert%d_%d(Object input,Class<T> type,boolean bypass,boolean fallback,Object alternate){Object r0=input;if(!bypass&&r0 instanceof Number&&Number.class.isAssignableFrom(type))r0=%s.choose%d((Number)r0,(Class)type%s);\n", tuple, depth, receiver, tuple, extraArgs.String())
			for i := 1; i <= depth; i++ {
				fmt.Fprintf(&source, "Object r%d=r%d;if(fallback)r%d=alternate;\n", i, i-1, i)
			}
			fmt.Fprintf(&source, "return r%d;}\n", depth)
		}
	}
	source.WriteString(`}
class WidenedBankDriver {
 static void check(java.lang.reflect.Method m,Object input,Class type,boolean bypass,boolean fallback,Object alternate,Object expected,String trace,Class failure)throws Exception{
  WidenedBankOps.trace="";
  try{Object got=m.invoke(null,input,type,bypass,fallback,alternate);if(failure!=null||got!=expected||!WidenedBankOps.trace.equals(trace))throw new AssertionError("identity/lazy/binding:"+m+":"+WidenedBankOps.trace);}
  catch(java.lang.reflect.InvocationTargetException e){if(failure==null||e.getCause().getClass()!=failure||!WidenedBankOps.trace.equals(trace))throw new AssertionError("original check/order:"+m+":"+WidenedBankOps.trace,e.getCause());}
 }
 public static void main(String[]args)throws Exception{int rows=0;Number number=Integer.valueOf(42),decimal=Double.valueOf(3.5);Object marker=new Object();
  for(int tuple=0;tuple<10;tuple++)for(int depth=1;depth<=10;depth++){
   java.lang.reflect.Method m=WidenedBankOwner.class.getDeclaredMethod("convert"+tuple+"_"+depth,Object.class,Class.class,boolean.class,boolean.class,Object.class);
   String called="C:"+tuple+":"+(tuple*(tuple+1)*(tuple+2)/3);
   check(m,number,Integer.class,false,false,marker,number,called,null);rows++;
   check(m,number,Number.class,false,false,marker,number,called,null);rows++;
   check(m,number,Integer.class,false,true,marker,marker,called,null);rows++;
   check(m,number,Double.class,true,false,marker,number,"",null);rows++;
   check(m,marker,null,false,false,marker,marker,"",null);rows++;
   check(m,null,null,false,false,marker,null,"",null);rows++;
   check(m,number,String.class,false,false,marker,number,"",null);rows++;
   check(m,number,Double.class,false,false,marker,null,called,ClassCastException.class);rows++;
   check(m,number,null,false,false,marker,null,"",NullPointerException.class);rows++;
   check(m,decimal,Double.class,false,false,marker,decimal,called,null);rows++;
  }
  System.out.println(rows+":widened:100-structures:tuple:identity:lazy:original-check");
 }
}`)
	testNativePrivateSetterSourceFixture(t, map[string]string{"WidenedBankOwner.java": source.String()}, "WidenedBankOwner", "WidenedBankDriver", "1000:widened:100-structures:tuple:identity:lazy:original-check\n")
}
