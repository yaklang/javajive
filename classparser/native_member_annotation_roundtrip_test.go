package javaclassparser

import "testing"

// An annotation is a static scope cut with an implicit Annotation interface.
// Original reflection checks defaults, actual use-site values and binary owner
// identity separately from constructor callback and generic-capture behavior.
const nativeMemberAnnotationFixture = `public enum NativeArchiveOwner {RED,BLUE}
abstract class NamedAnnParent {final Object observed;NamedAnnParent(){observed=observe();}abstract Object observe();}
class NamedAnnOwner<T> {
 private final T token;NamedAnnOwner(T token){this.token=token;}
 @java.lang.annotation.Retention(java.lang.annotation.RetentionPolicy.RUNTIME)
 public @interface Meta {String value() default "original";}
 @java.lang.annotation.Retention(java.lang.annotation.RetentionPolicy.RUNTIME)
 @java.lang.annotation.Target({java.lang.annotation.ElementType.TYPE,java.lang.annotation.ElementType.METHOD,java.lang.annotation.ElementType.PARAMETER})
 public @interface Spec {
  boolean enabled() default true;int count() default -7;char character() default '\uD800';
  String text() default "quoted\"\\\n";int[] numbers() default {1,-2,3};
  byte tiny() default -128;short shorty() default 32767;long wide() default Long.MIN_VALUE;
  float nan32() default Float.NaN;double nan64() default Double.NaN;
  float positive() default Float.POSITIVE_INFINITY;double negativeZero() default -0.0D;
  Class<?>[] types() default {void.class,int[].class,String[][].class};String[] words() default {};
  Class<?> type() default String.class;NativeArchiveOwner choice() default NativeArchiveOwner.RED;
  Meta nested() default @Meta;Meta[] many() default {@Meta("left"),@Meta("right")};
 }
 @Spec(count=17,choice=NativeArchiveOwner.BLUE,nested=@Meta("actual"))
 class Layer extends NamedAnnParent {
  private final T value;Layer(T value){this.value=value;}Object observe(){return NamedAnnOwner.this.token;}
  class Leaf {@Spec(enabled=false) T read(){return Layer.this.value;}}
 }
}
class NamedAnnDriver {public static void main(String[]args)throws Exception {
 Object token=new Object(),payload=new Object();NamedAnnOwner<Object> o=new NamedAnnOwner<Object>(token);NamedAnnOwner<Object>.Layer layer=o.new Layer(payload);NamedAnnOwner<Object>.Layer.Leaf leaf=layer.new Leaf();
 if(layer.observed!=token||leaf.read()!=payload)throw new AssertionError("constructor/capture identity");
 NamedAnnOwner.Spec a=layer.getClass().getAnnotation(NamedAnnOwner.Spec.class);
 if(a==null||!a.enabled()||a.count()!=17||a.character()!='\uD800'||!a.text().equals("quoted\"\\\n")||!java.util.Arrays.equals(a.numbers(),new int[]{1,-2,3})||a.type()!=String.class||a.choice()!=NativeArchiveOwner.BLUE||!a.nested().value().equals("actual")||a.many().length!=2||!a.many()[0].value().equals("left")||!a.many()[1].value().equals("right"))throw new AssertionError("annotation values/defaults");
 if(Float.floatToRawIntBits(a.nan32())!=0x7fc00000||Double.doubleToRawLongBits(a.nan64())!=0x7ff8000000000000L)throw new AssertionError("NaN default bits");
 if(a.tiny()!=-128||a.shorty()!=32767||a.wide()!=Long.MIN_VALUE||a.positive()!=Float.POSITIVE_INFINITY||Double.doubleToRawLongBits(a.negativeZero())!=Long.MIN_VALUE||!java.util.Arrays.equals(a.types(),new Class<?>[]{void.class,int[].class,String[][].class})||a.words().length!=0)throw new AssertionError("typed scalar/class/empty defaults");
 NamedAnnOwner.Spec b=leaf.getClass().getDeclaredMethod("read").getAnnotation(NamedAnnOwner.Spec.class);
 if(b==null||b.enabled()||b.count()!=-7||b.choice()!=NativeArchiveOwner.RED||!b.nested().value().equals("original"))throw new AssertionError("method defaults");
 for(Class<?> c:new Class<?>[]{NamedAnnOwner.Meta.class,NamedAnnOwner.Spec.class}){if(!c.isAnnotation()||c.getDeclaringClass()!=NamedAnnOwner.class||c.getTypeParameters().length!=0||!java.lang.reflect.Modifier.isStatic(c.getModifiers())||!java.util.Arrays.equals(c.getInterfaces(),new Class<?>[]{java.lang.annotation.Annotation.class}))throw new AssertionError("annotation binary owner/scope");}
 System.out.println("defaults:annotations:scope:callback:identity");
}}`

func TestNativeMemberAnnotationDefaultsAndStaticScopeRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeMemberAnnotationFixture, "NamedAnnOwner", "NamedAnnDriver", "defaults:annotations:scope:callback:identity\n")
}
