package javaclassparser

import "testing"

// The parent callback observes the lexical capture before the constructor body.
// Annotation values separately observe original Class identities and bounds.
const annotationClassBoundsFixture = `class BoundEffects {static String trace="";static final RuntimeException error=new RuntimeException("identity");static boolean fail;}
interface BoundMarker {}
class BoundBase implements BoundMarker {}
class BoundLeaf extends BoundBase {}
abstract class BoundParent {final Object observed;BoundParent(){BoundEffects.trace+="P";observed=observe();if(BoundEffects.fail)throw BoundEffects.error;}abstract Object observe();}
class ClassBoundOwner<T> {
 final T token;ClassBoundOwner(T token){this.token=token;}
 @java.lang.annotation.Retention(java.lang.annotation.RetentionPolicy.RUNTIME)
 @interface Spec {
  Class<? extends BoundBase> upper() default BoundLeaf.class;
  Class<? super BoundLeaf> lower() default BoundBase.class;
  Class<BoundLeaf> exact() default BoundLeaf.class;
  Class<? extends BoundMarker> viaInterface() default BoundLeaf.class;
  Class<? extends Number> primitive() default int.class;
  Class<? extends Object[]> array() default String[][].class;
  Class<? extends BoundBase>[] many() default {BoundBase.class,BoundLeaf.class};
  Class<?> any() default void.class;
 }
 @Spec(upper=BoundBase.class,lower=Object.class,many={BoundLeaf.class})
 class Reader extends BoundParent {final T value;Reader(T value){this.value=value;BoundEffects.trace+="B";}Object observe(){return ClassBoundOwner.this.token;}T read(){return value;}}
 Reader make(T value){return new Reader(value);}
}
class ClassBoundDriver {public static void main(String[]args)throws Exception{
 int rows=0;Object identity=new Object();for(Object token:new Object[]{null,identity,"word"})for(Object value:new Object[]{null,identity,new Object()})for(boolean fail:new boolean[]{false,true}){
 ClassBoundOwner<Object> owner=new ClassBoundOwner<Object>(token);BoundEffects.trace="";BoundEffects.fail=fail;
 try{ClassBoundOwner<Object>.Reader reader=owner.make(value);if(fail||reader.observed!=token||reader.read()!=value||!BoundEffects.trace.equals("PB"))throw new AssertionError("capture/identity/order");
 ClassBoundOwner.Spec a=reader.getClass().getAnnotation(ClassBoundOwner.Spec.class);if(a==null||a.upper()!=BoundBase.class||a.lower()!=Object.class||a.exact()!=BoundLeaf.class||a.viaInterface()!=BoundLeaf.class||a.primitive()!=int.class||a.array()!=String[][].class||a.any()!=void.class||!java.util.Arrays.equals(a.many(),new Class<?>[]{BoundLeaf.class}))throw new AssertionError("use site/default identity");
 }catch(RuntimeException e){if(!fail||e!=BoundEffects.error||!BoundEffects.trace.equals("P"))throw new AssertionError("failure identity/order",e);}rows++;
 }
 for(java.lang.reflect.Method m:ClassBoundOwner.Spec.class.getDeclaredMethods()){Object d=m.getDefaultValue();String n=m.getName();if(n.equals("upper")||n.equals("exact")||n.equals("viaInterface")){if(d!=BoundLeaf.class)throw new AssertionError(n);}else if(n.equals("lower")){if(d!=BoundBase.class)throw new AssertionError(n);}else if(n.equals("primitive")){if(d!=int.class)throw new AssertionError(n);}else if(n.equals("array")){if(d!=String[][].class)throw new AssertionError(n);}else if(n.equals("many")){if(!java.util.Arrays.equals((Class<?>[])d,new Class<?>[]{BoundBase.class,BoundLeaf.class}))throw new AssertionError(n);}else if(n.equals("any")){if(d!=void.class)throw new AssertionError(n);}else throw new AssertionError("unexpected method");
 String actual=m.getGenericReturnType().getTypeName();String expected=n.equals("upper")?"java.lang.Class<? extends BoundBase>":n.equals("lower")?"java.lang.Class<? super BoundLeaf>":n.equals("exact")?"java.lang.Class<BoundLeaf>":n.equals("viaInterface")?"java.lang.Class<? extends BoundMarker>":n.equals("primitive")?"java.lang.Class<? extends java.lang.Number>":n.equals("array")?"java.lang.Class<? extends java.lang.Object[]>":n.equals("many")?"java.lang.Class<? extends BoundBase>[]":"java.lang.Class<?>";if(!actual.equals(expected))throw new AssertionError("generic return:"+actual+":"+expected);
 }
 if(!ClassBoundOwner.Spec.class.isAnnotation()||ClassBoundOwner.Spec.class.getDeclaringClass()!=ClassBoundOwner.class||!ClassBoundOwner.Reader.class.getName().endsWith("$Reader"))throw new AssertionError("source/binary scope");
 System.out.println(rows+":class-bounds:defaults:identity:callback:failure:order");
}}`

func TestAdversarialAnnotationClassBoundsRoundTrip(t *testing.T) {
	testSourceTargetReleaseFamilyFixture(t, annotationClassBoundsFixture, "ClassBoundOwner", "ClassBoundDriver", "18:class-bounds:defaults:identity:callback:failure:order\n", "8", []int{8, 11})
}
func TestAdversarialModernAnnotationClassBoundsRoundTrip(t *testing.T) {
	testSourceTargetReleaseFamilyFixture(t, annotationClassBoundsFixture, "ClassBoundOwner", "ClassBoundDriver", "18:class-bounds:defaults:identity:callback:failure:order\n", "11", []int{11, 16})
}
