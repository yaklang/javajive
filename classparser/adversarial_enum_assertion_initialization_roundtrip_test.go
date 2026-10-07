package javaclassparser

import (
	"strings"
	"testing"
)

const enumAssertionInitializationFixture = `class EnumAssertionEffects{static String trace="";static final RuntimeException payload=new RuntimeException("identity");static boolean condition(int width){trace+="C";return width>0&&width<=64;}static Object message(){trace+="M";return payload;}static long finish(){trace+="S";return 17;}}
class EnumAssertionOwner {enum Mode {MASK(1){long apply(long x,int width){return x&mask(width);}},XOR(2){long apply(long x,int width){return x^mask(width);}};final int id;static final long stamp=EnumAssertionEffects.finish();Mode(int id){this.id=id;EnumAssertionEffects.trace+="P"+id;}abstract long apply(long x,int width);long mask(int width){assert EnumAssertionEffects.condition(width):EnumAssertionEffects.message();return width==64?-1L:(1L<<width)-1;}}}
class EnumAssertionDriver{public static void main(String[]args)throws Exception{boolean enabled=false;ClassLoader loader=ClassLoader.getSystemClassLoader();loader.setClassAssertionStatus("EnumAssertionOwner",enabled);loader.setClassAssertionStatus("EnumAssertionOwner$Mode",!enabled);EnumAssertionOwner.Mode[] choices=EnumAssertionOwner.Mode.values();if(!EnumAssertionEffects.trace.equals("P1P2S")||EnumAssertionOwner.Mode.stamp!=17||choices[0]!=EnumAssertionOwner.Mode.MASK||choices[1]!=EnumAssertionOwner.Mode.XOR||EnumAssertionOwner.Mode.valueOf("MASK")!=choices[0]||choices[0].id!=1||choices[1].id!=2)throw new AssertionError("initialization order/enum identity");java.lang.reflect.Field flag=EnumAssertionOwner.Mode.class.getDeclaredField("$assertionsDisabled");if(!flag.isSynthetic()||flag.getModifiers()!=0x1018)throw new AssertionError("assertion field metadata");int rows=0;for(EnumAssertionOwner.Mode mode:choices)for(long x:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(int width:new int[]{Integer.MIN_VALUE,-1,0,1,7,31,32,63,64,65,70,Integer.MAX_VALUE}){EnumAssertionEffects.trace="";boolean fails=enabled&&(width<=0||width>64);try{long actual=mode.apply(x,width);java.math.BigInteger mask=width==64?java.math.BigInteger.valueOf(-1):java.math.BigInteger.ONE.shiftLeft(width&63).subtract(java.math.BigInteger.ONE);long expected=(mode==choices[0]?java.math.BigInteger.valueOf(x).and(mask):java.math.BigInteger.valueOf(x).xor(mask)).longValue();if(fails||actual!=expected)throw new AssertionError("bit word/oracle");}catch(AssertionError e){if(!fails||e.getCause()!=EnumAssertionEffects.payload)throw new AssertionError("assertion payload identity",e);}if(!EnumAssertionEffects.trace.equals(enabled?(fails?"CM":"C"):""))throw new AssertionError("assertion evaluation/message order");rows++;}System.out.println(rows+":enum:assertions:"+enabled+":word:identity:initialization");}}`

func TestAdversarialEnumAssertionMixedInitializationRoundTrip(t *testing.T) {
	testSourceTargetReleaseFamilyFixture(t, enumAssertionInitializationFixture, "EnumAssertionOwner", "EnumAssertionDriver", "120:enum:assertions:false:word:identity:initialization\n", "8", []int{8, 11})
}
func TestAdversarialEnumEnabledAssertionMixedInitializationRoundTrip(t *testing.T) {
	fixture := strings.Replace(enumAssertionInitializationFixture, "boolean enabled=false;", "boolean enabled=true;", 1)
	testSourceTargetReleaseFamilyFixture(t, fixture, "EnumAssertionOwner", "EnumAssertionDriver", "120:enum:assertions:true:word:identity:initialization\n", "8", []int{8, 11})
}

// Assertions inside the retained static suffix must remain after the constants
// and stamp effect. Proving the prefix never permits dropping that suffix.
func TestAdversarialEnumInitializerAssertionAfterConstantsRoundTrip(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name, trace, expected := "disabled", "P1P2ST", "120:enum:assertions:false:word:identity:initialization\n"
		fixture := enumAssertionInitializationFixture
		if enabled {
			name, trace, expected = "enabled", "P1P2SCT", "120:enum:assertions:true:word:identity:initialization\n"
			fixture = strings.Replace(fixture, "boolean enabled=false;", "boolean enabled=true;", 1)
		}
		fixture = strings.Replace(fixture, "Mode(int id)", `static {assert EnumAssertionEffects.condition(1):EnumAssertionEffects.message();EnumAssertionEffects.trace+="T";}Mode(int id)`, 1)
		fixture = strings.Replace(fixture, `trace.equals("P1P2S")`, `trace.equals("`+trace+`")`, 1)
		t.Run(name, func(t *testing.T) {
			testSourceTargetReleaseFamilyFixture(t, fixture, "EnumAssertionOwner", "EnumAssertionDriver", expected, "8", []int{8, 11})
		})
	}
}
