package javaclassparser

import (
	"strings"
	"testing"
)

func enumSwitchTableConstructorMarkerFixture(enabled bool) string {
	f := enumAssertionInitializationFixture
	f = f[:strings.Index(f, "\nclass EnumAssertionDriver")]
	f = strings.Replace(f, "class EnumAssertionOwner {", `class EnumAssertionOwner {static long choose(TableRoleMode selector,long x,int width){switch(selector){case XOR:return Mode.XOR.apply(x,width);case MASK:return Mode.MASK.apply(x,width);default:return Mode.MASK.apply(x,width);}}`, 1)
	f += `
enum TableRoleMode {MASK,XOR,OTHER;static {EnumAssertionEffects.trace+="E";}}
class TableRoleDriver {public static void main(String[]args)throws Exception{boolean enabled=false;ClassLoader loader=ClassLoader.getSystemClassLoader();loader.setClassAssertionStatus("EnumAssertionOwner",enabled);loader.setClassAssertionStatus("EnumAssertionOwner$Mode",!enabled);EnumAssertionOwner.Mode[] modes=EnumAssertionOwner.Mode.values();if(!EnumAssertionEffects.trace.equals("P1P2S")||modes[0]!=EnumAssertionOwner.Mode.MASK||modes[1]!=EnumAssertionOwner.Mode.XOR)throw new AssertionError("unused constructor marker must not initialize switch table");EnumAssertionEffects.trace="";try{EnumAssertionOwner.choose(null,1,1);throw new AssertionError("null selector");}catch(NullPointerException expected){}if(!EnumAssertionEffects.trace.equals("E"))throw new AssertionError("table initialization must precede selector null dereference");int rows=0;for(TableRoleMode selector:TableRoleMode.values())for(long x:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(int width:new int[]{Integer.MIN_VALUE,-1,0,1,7,31,32,63,64,65,70,Integer.MAX_VALUE}){EnumAssertionEffects.trace="";boolean fails=enabled&&(width<=0||width>64);try{long actual=EnumAssertionOwner.choose(selector,x,width);java.math.BigInteger mask=width==64?java.math.BigInteger.valueOf(-1):java.math.BigInteger.ONE.shiftLeft(width&63).subtract(java.math.BigInteger.ONE);long expected=(selector==TableRoleMode.XOR?java.math.BigInteger.valueOf(x).xor(mask):java.math.BigInteger.valueOf(x).and(mask)).longValue();if(fails||actual!=expected)throw new AssertionError("algorithm oracle");}catch(AssertionError e){if(!fails||e.getCause()!=EnumAssertionEffects.payload)throw new AssertionError("assertion identity",e);}if(!EnumAssertionEffects.trace.equals(enabled?(fails?"CM":"C"):""))throw new AssertionError("assertion order");rows++;}System.out.println(rows+":table:constructor-marker:assertions:"+enabled+":initialization:word:identity");}}
`
	if enabled {
		f = strings.Replace(f, "boolean enabled=false;", "boolean enabled=true;", 1)
	}
	return f
}

func TestAdversarialEnumSwitchTableConstructorMarkerDisabledRoundTrip(t *testing.T) {
	testSourceTargetReleaseFamilyFixture(t, enumSwitchTableConstructorMarkerFixture(false), "EnumAssertionOwner", "TableRoleDriver", "180:table:constructor-marker:assertions:false:initialization:word:identity\n", "8", []int{8, 11})
}
func TestAdversarialEnumSwitchTableConstructorMarkerEnabledRoundTrip(t *testing.T) {
	testSourceTargetReleaseFamilyFixture(t, enumSwitchTableConstructorMarkerFixture(true), "EnumAssertionOwner", "TableRoleDriver", "180:table:constructor-marker:assertions:true:initialization:word:identity\n", "8", []int{8, 11})
}
