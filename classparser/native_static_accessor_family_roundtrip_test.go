package javaclassparser

import (
	"strings"
	"testing"
)

const nativeStaticAccessorReadFixture = `class StaticReadEffects{static String trace="";static final Object token=new Object();static Object initialize(){trace+="I";return token;}}
class StaticReadOwner{private static Object token=StaticReadEffects.initialize();private static long number=Long.MIN_VALUE;static class Reader{static Object token(){return token;}static long number(){return number;}}}
class StaticReadDriver{public static void main(String[]args){StaticReadEffects.trace="";if(StaticReadOwner.Reader.token()!=StaticReadEffects.token||StaticReadOwner.Reader.number()!=Long.MIN_VALUE||!StaticReadEffects.trace.equals("I"))throw new AssertionError("class initialization/read/wide");if(StaticReadOwner.Reader.token()!=StaticReadEffects.token||!StaticReadEffects.trace.equals("I"))throw new AssertionError("class initialization repeated");System.out.println("static:read:original:initialization:once");}}`

func TestNativeStaticAccessorReadRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeStaticAccessorReadFixture, "StaticReadOwner", "StaticReadDriver", "static:read:original:initialization:once\n")
}
func TestNativeStaticAccessorReadRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeStaticAccessorReadFixture, "StaticReadOwner", "DifferentStaticReadScope")
	testNativePrivateSetterFixture(t, f, "DifferentStaticReadScope", "StaticReadDriver", "static:read:original:initialization:once\n")
}

func TestNativeStaticAccessorMethodFormalClassCollisionRoundTrip(t *testing.T) {
	f := strings.Replace(nativeStaticAccessorReadFixture, "static Object token()", "static <StaticReadOwner extends Number> Object token()", 1)
	testNativePrivateSetterFixture(t, f, "StaticReadOwner", "StaticReadDriver", "static:read:original:initialization:once\n")
}

func TestNativeStaticAccessorInitializationFailureRoundTrip(t *testing.T) {
	f := strings.Replace(nativeStaticAccessorReadFixture, `static Object initialize(){trace+="I";return token;}`, `static final RuntimeException error=new RuntimeException("original");static Object initialize(){trace+="I";throw error;}`, 1)
	f = f[:strings.Index(f, "class StaticReadDriver")] + `class StaticReadDriver{public static void main(String[]args){StaticReadEffects.trace="";try{StaticReadOwner.Reader.token();throw new AssertionError("missing initialization failure");}catch(ExceptionInInitializerError e){if(e.getCause()!=StaticReadEffects.error||!StaticReadEffects.trace.equals("I"))throw new AssertionError("original initialization cause/order");}try{StaticReadOwner.Reader.number();throw new AssertionError("erroneous class initialized again");}catch(NoClassDefFoundError e){if(!StaticReadEffects.trace.equals("I"))throw new AssertionError("initialization repeated");}System.out.println("static:read:initialization:failure:identity:once");}}`
	testNativePrivateSetterFixture(t, f, "StaticReadOwner", "StaticReadDriver", "static:read:initialization:failure:identity:once\n")
}
func TestNativeStaticAccessorMethodFormalLocalShadowRoundTrip(t *testing.T) {
	f := strings.Replace(nativeStaticAccessorReadFixture, "static Object token()", "static <StaticReadOwner extends Number> Object token(Object ignored)", 1)
	f = strings.ReplaceAll(f, "Reader.token()", "Reader.token(new Object())")
	f = strings.ReplaceAll(f, "token", "var0")
	testNativePrivateSetterFixture(t, f, "StaticReadOwner", "StaticReadDriver", "static:read:original:initialization:once\n")
}

func TestNativeStaticAccessorPrimitiveBitsAndArrayIdentityRoundTrip(t *testing.T) {
	fixture := `class StaticWordEffects{static final Object token=new Object();static float negativeZero(){return Float.intBitsToFloat(0x80000000);}static double nan(){return Double.longBitsToDouble(0x7ff0000000000001L);}}
class StaticWordOwner{private static boolean flag=true;private static byte small=Byte.MIN_VALUE;private static char letter=Character.MAX_VALUE;private static short narrow=Short.MIN_VALUE;private static int word=Integer.MIN_VALUE;private static long wide=Long.MAX_VALUE;private static float floating=StaticWordEffects.negativeZero();private static double decimal=StaticWordEffects.nan();private static Object[][] rows=new Object[][]{null,new Object[]{StaticWordEffects.token}};static class Reader{static boolean flag(){return flag;}static byte small(){return small;}static char letter(){return letter;}static short narrow(){return narrow;}static int word(){return word;}static long wide(){return wide;}static float floating(){return floating;}static double decimal(){return decimal;}static Object[][] rows(){return rows;}}}
class StaticWordDriver{public static void main(String[]args){if(!StaticWordOwner.Reader.flag()||StaticWordOwner.Reader.small()!=Byte.MIN_VALUE||StaticWordOwner.Reader.letter()!=Character.MAX_VALUE||StaticWordOwner.Reader.narrow()!=Short.MIN_VALUE||StaticWordOwner.Reader.word()!=Integer.MIN_VALUE||StaticWordOwner.Reader.wide()!=Long.MAX_VALUE)throw new AssertionError("computational word/return category");if(Float.floatToRawIntBits(StaticWordOwner.Reader.floating())!=0x80000000||Double.doubleToRawLongBits(StaticWordOwner.Reader.decimal())!=0x7ff0000000000001L)throw new AssertionError("raw floating bits");Object[][] rows=StaticWordOwner.Reader.rows();if(rows!=StaticWordOwner.Reader.rows()||rows.length!=2||rows[0]!=null||rows[1][0]!=StaticWordEffects.token)throw new AssertionError("array rank/identity");rows[1][0]=null;if(StaticWordOwner.Reader.rows()[1][0]!=null)throw new AssertionError("array was copied");System.out.println("static:read:all:words:raw:bits:array:identity");}}`
	testNativePrivateSetterFixture(t, fixture, "StaticWordOwner", "StaticWordDriver", "static:read:all:words:raw:bits:array:identity\n")
}
