package javaclassparser

import (
	"strings"
	"testing"
)

const nativeAnonymousInitializerFixture = `class AnonymousInitEffects {static String trace="";static final RuntimeException error=new RuntimeException("original");}
abstract class AnonymousInitParent {final Object seed;AnonymousInitParent(Object seed){AnonymousInitEffects.trace+="P"+first();if(seed==null)throw AnonymousInitEffects.error;this.seed=seed;}abstract int first();abstract double wide();abstract Object alias();abstract int character();abstract boolean enabled();}
class AnonymousInitOwner {AnonymousInitParent make(Object seed,Object captured,double wide){return new AnonymousInitParent(seed){int initial=-1;final double saved=wide;Object ref=captured;char letter='\uffff';boolean flag=true;int first(){return initial;}double wide(){return saved;}Object alias(){return ref;}int character(){return letter;}boolean enabled(){return flag;}};}}
class AnonymousInitDriver{public static void main(String[]args){Object seed=new Object();Object token=new Object();int rows=0;for(Object v:new Object[]{null,token})for(double d:new double[]{-0.0,Double.NaN,Double.POSITIVE_INFINITY}){AnonymousInitEffects.trace="";AnonymousInitParent p=new AnonymousInitOwner().make(seed,v,d);if(p.seed!=seed||p.first()!=-1||p.alias()!=v||p.character()!=65535||!p.enabled()||Double.doubleToRawLongBits(p.wide())!=Double.doubleToRawLongBits(d)||!AnonymousInitEffects.trace.equals("P0")||!p.getClass().getName().equals("AnonymousInitOwner$1")||p.getClass().getEnclosingClass()!=AnonymousInitOwner.class)throw new AssertionError("initialization/value/callback/order/owner");rows++;}AnonymousInitEffects.trace="";try{new AnonymousInitOwner().make(null,token,0);throw new AssertionError("missing parent failure");}catch(RuntimeException e){if(e!=AnonymousInitEffects.error||!AnonymousInitEffects.trace.equals("P0"))throw new AssertionError("parent failure identity/order");}System.out.println(rows+":anonymous:initializer:callback:identity:owner");}}
`

func TestNativeAnonymousPostSuperFieldInitializersRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeAnonymousInitializerFixture, "AnonymousInitOwner", "AnonymousInitDriver", "6:anonymous:initializer:callback:identity:owner\n")
}

func TestNativeAnonymousPostSuperTypedLiteralInitializersRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeAnonymousInitializerFixture, `boolean flag=true;`, `boolean flag=true;byte small=(byte)255;short narrow=(short)65535;float negativeFloat=-0.0f;long huge=Long.MIN_VALUE;double negativeDouble=-0.0;String quoted="null";String missing=null;`, 1)
	fixture = strings.Replace(fixture, `main(String[]args){`, `main(String[]args)throws Exception{`, 1)
	fixture = strings.Replace(fixture, `rows++;`, `Class<?> c=p.getClass();String[] names={"small","narrow","negativeFloat","huge","negativeDouble","quoted","missing"};Object[] expected={Byte.valueOf((byte)-1),Short.valueOf((short)-1),Float.valueOf(-0.0f),Long.valueOf(Long.MIN_VALUE),Double.valueOf(-0.0),"null",null};for(int i=0;i<names.length;i++){java.lang.reflect.Field f=c.getDeclaredField(names[i]);f.setAccessible(true);Object actual=f.get(p);if(!java.util.Objects.equals(actual,expected[i]))throw new AssertionError("literal field "+names[i]);}rows++;`, 1)
	testNativePrivateSetterFixture(t, fixture, "AnonymousInitOwner", "AnonymousInitDriver", "6:anonymous:initializer:callback:identity:owner\n")
}

func TestNativeAnonymousPostSuperInitializersIgnoreSourceSpelling(t *testing.T) {
	fixture := strings.ReplaceAll(nativeAnonymousInitializerFixture, "AnonymousInitOwner", "IndependentInitializationScope")
	fixture = strings.ReplaceAll(fixture, "captured", "keptValue")
	testNativePrivateSetterFixture(t, fixture, "IndependentInitializationScope", "AnonymousInitDriver", "6:anonymous:initializer:callback:identity:owner\n")
}

const nativeAnonymousGenericInitializerFixture = `class GenericAnonInitEffects{static String trace="";static final java.io.IOException error=new java.io.IOException("original");}
abstract class GenericAnonInitParent<T>{final Object seed;GenericAnonInitParent(Object seed)throws java.io.IOException{GenericAnonInitEffects.trace+="P"+(values()==null);if(seed==null)throw GenericAnonInitEffects.error;this.seed=seed;}abstract T[] values();}
class GenericAnonInitOwner {static <Q> GenericAnonInitParent<Q> make(Object seed,Q[] values)throws java.io.IOException{return new GenericAnonInitParent<Q>(seed){final Q[] copy=values;Q[] values(){return copy;}};}}
class GenericAnonInitDriver{public static void main(String[]args)throws Exception{Object seed=new Object();Object[]objects=new Object[]{seed,null};String[]strings={"original"};int rows=0;for(Object[]a:new Object[][]{null,objects,strings}){GenericAnonInitEffects.trace="";GenericAnonInitParent<?>p=GenericAnonInitOwner.make(seed,a);if(p.values()!=a||p.seed!=seed||!GenericAnonInitEffects.trace.equals("Ptrue")||p.getClass().getEnclosingClass()!=GenericAnonInitOwner.class||!p.getClass().getName().equals("GenericAnonInitOwner$1"))throw new AssertionError("generic identity/callback/owner");rows++;}GenericAnonInitEffects.trace="";try{GenericAnonInitOwner.make(null,objects);throw new AssertionError("missing checked failure");}catch(java.io.IOException e){if(e!=GenericAnonInitEffects.error||!GenericAnonInitEffects.trace.equals("Ptrue"))throw new AssertionError("checked failure identity/order");}System.out.println(rows+":generic:array:initializer:checked:owner");}}
`

func TestNativeAnonymousPostSuperGenericArrayInitializerRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeAnonymousGenericInitializerFixture, "GenericAnonInitOwner", "GenericAnonInitDriver", "3:generic:array:initializer:checked:owner\n")
}

func TestNativeAnonymousPostSuperCapturedFieldMutationRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeAnonymousInitializerFixture, "AnonymousInitEffects.trace+=\"P\"+first();", "AnonymousInitEffects.trace+=\"P\"+first();try{for(java.lang.reflect.Field f:getClass().getDeclaredFields()){if(f.isSynthetic()&&f.getType()==double.class){f.setAccessible(true);f.setDouble(this,42.0);}}}catch(ReflectiveOperationException e){throw new AssertionError(e);}", 1)
	fixture = strings.Replace(fixture, "Double.doubleToRawLongBits(d)", "Double.doubleToRawLongBits(42.0)", 1)
	testNativePrivateSetterFixture(t, fixture, "AnonymousInitOwner", "AnonymousInitDriver", "6:anonymous:initializer:callback:identity:owner\n")
}
