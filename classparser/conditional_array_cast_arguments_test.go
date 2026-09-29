package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

func TestAdversarialConditionalArrayCastArgumentsRoundTrip(t *testing.T) {
	t.Parallel()
	var source strings.Builder
	source.WriteString("import java.util.*; public class ArrayCastArguments {\n")
	for i, kind := range []string{"boolean", "byte", "short", "char", "int", "long", "float", "double", "Object"} {
		fmt.Fprintf(&source, "static boolean eq%d(Object a,Object b){return b instanceof %s[] && Arrays.equals((%s[])(Cloneable)a,(%s[])(Cloneable)b);}\n", i, kind, kind, kind)
	}
	source.WriteString(`static boolean eq(int kind,Object a,Object b) { switch(kind) {
case 0:return eq0(a,b);case 1:return eq1(a,b);case 2:return eq2(a,b);
case 3:return eq3(a,b);case 4:return eq4(a,b);case 5:return eq5(a,b);
case 6:return eq6(a,b);case 7:return eq7(a,b);default:return eq8(a,b);
} }
static String trace="";
static Object read(Object value) { trace+="R";return value; }
static String join(String a,long value,CharSequence b) { trace+="J";return a+":"+value+":"+b; }
static String choose(boolean selected,Object a,long value,Object b) {
 return selected ? join((String)(CharSequence)read(a),value,(CharSequence)b) : "skip";
}
public static void main(String[] args) {
 Object[] samples={new boolean[]{true},new byte[]{1},new short[]{2},new char[]{'x'},new int[]{3},new long[]{4},new float[]{Float.NaN,-0.0f},new double[]{Double.NaN,-0.0},new Object[]{"x"}};
 for(int i=0;i<samples.length;i++)for(Object left:new Object[]{samples[i],null,"wrong"})for(Object right:new Object[]{samples[i],null,"wrong"}) {
   try { System.out.print(eq(i,left,right)+";"); }
   catch(ClassCastException e) { System.out.print("cast;"); }
 }
 for(boolean selected:new boolean[]{false,true})for(Object left:new Object[]{"ok",new StringBuilder("bad"),null})for(Object right:new Object[]{"end",Integer.valueOf(4)}) {
   trace="";
   try { System.out.print(choose(selected,left,9L,right)+":"+trace+";"); }
   catch(ClassCastException e) { System.out.print(e.getMessage()+":"+trace+";"); }
 }
}
}`)
	roundTripGenericFlow(t, "ArrayCastArguments", source.String(), Precision, Compatibility, "legacy")
}
