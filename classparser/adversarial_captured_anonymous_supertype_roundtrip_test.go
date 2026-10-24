package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

// The runtime producer is anonymous, its immediate parent is a generic class,
// and the captured local was declared as a more distant generic interface.
// That declared type controls both overload binding and the hidden capture ABI.
const capturedAnonymousSupertypeFixture = `
class ProjectionEffects{static String trace="";}
abstract class ProjectionCursor<Q> implements java.util.Iterator<Q>{
 enum Phase{START} private final Phase phase=Phase.START;
 ProjectionCursor(){ProjectionEffects.trace+="B";}
 public void remove(){throw new UnsupportedOperationException();}
}
abstract class ProjectionForward<Q> implements java.util.Iterator<Q>{
 final java.util.Iterator<Q> early;
 ProjectionForward(){ProjectionEffects.trace+="F";early=delegate();}
 protected abstract java.util.Iterator<Q> delegate();
 public boolean hasNext(){return delegate().hasNext();}
 public Q next(){return delegate().next();}
 public void remove(){delegate().remove();}
}
class ProjectionOwner {
 static String choose(java.util.Iterator<?> x){ProjectionEffects.trace+="W";return "wide";}
 static String choose(ProjectionCursor<?> x){throw new AssertionError("narrow overload");}
 java.util.Iterator<CharSequence> make(final CharSequence token){
  final java.util.Iterator<CharSequence> reader=new ProjectionCursor<CharSequence>(){
   private int count;
   public boolean hasNext(){return count==0;}
   public CharSequence next(){if(count++!=0)throw new java.util.NoSuchElementException();return token;}
  };
  if(!choose(reader).equals("wide"))throw new AssertionError("binding");
  return new ProjectionForward<CharSequence>(){protected java.util.Iterator<CharSequence> delegate(){return reader;}};
 }
}
class ProjectionDriver{
 public static void main(String[]args)throws Exception{
  int rows=0;
  for(CharSequence token:new CharSequence[]{null,new String("same"),new StringBuilder("\u20ac\ud83d\ude42")})for(int repeat=0;repeat<3;repeat++){
   ProjectionEffects.trace="";java.util.Iterator<CharSequence> value=new ProjectionOwner().make(token);
   if(!ProjectionEffects.trace.equals("BWF")||!value.hasNext()||!value.hasNext()||value.next()!=token||value.hasNext())throw new AssertionError("algorithm/binding/order/identity");
   try{value.next();throw new AssertionError("missing exhaustion");}catch(java.util.NoSuchElementException good){}
   try{value.remove();throw new AssertionError("missing unsupported mutation");}catch(UnsupportedOperationException good){}
   ProjectionForward<?> forward=(ProjectionForward<?>)value;
   java.lang.reflect.Field capture=value.getClass().getDeclaredField("val$reader");capture.setAccessible(true);
   if(capture.getType()!=java.util.Iterator.class||capture.get(value)!=forward.early||!forward.early.getClass().isAnonymousClass()||!value.getClass().isAnonymousClass()||value.getClass().getEnclosingClass()!=ProjectionOwner.class||!value.getClass().getEnclosingMethod().getName().equals("make"))throw new AssertionError("capture ABI/early observation/lexical identity");
   rows++;
  }
  System.out.println(rows+":anonymous:generic-supertype:capture:binding:early:identity");
 }
}`

func capturedAnonymousSupertypeShape(shape string) string {
	source := capturedAnonymousSupertypeFixture
	switch shape {
	case "class-variable", "hidden-bound":
		source = strings.Replace(source, "class ProjectionOwner {", "class ProjectionOwner<E extends CharSequence> {", 1)
		source = strings.Replace(source, "java.util.Iterator<CharSequence> make(final CharSequence token)", "java.util.Iterator<E> make(final E token)", 1)
		source = strings.Replace(source, "final java.util.Iterator<CharSequence> reader=new ProjectionCursor<CharSequence>()", "final java.util.Iterator<E> reader=new ProjectionCursor<E>()", 1)
		source = strings.Replace(source, "public CharSequence next(){if(count++", "public E next(){if(count++", 1)
		source = strings.Replace(source, "new ProjectionForward<CharSequence>(){protected java.util.Iterator<CharSequence> delegate()", "new ProjectionForward<E>(){protected java.util.Iterator<E> delegate()", 1)
		source = strings.Replace(source, "new ProjectionOwner().make(token)", "new ProjectionOwner<CharSequence>().make(token)", 1)
	case "composed", "diamond":
		intermediate := "interface ProjectionHop<A,B> extends java.util.Iterator<B>{}\nabstract class ProjectionLayer<A,B> implements ProjectionHop<A,B>{}\n"
		if shape == "diamond" {
			intermediate = "interface ProjectionPath<X> extends java.util.Iterator<X>{}\ninterface ProjectionHop<A,B> extends ProjectionPath<B>{}\nabstract class ProjectionLayer<A,B> implements ProjectionHop<A,B>,ProjectionPath<B>{}\n"
		}
		source = strings.Replace(source, "abstract class ProjectionCursor<Q> implements java.util.Iterator<Q>{", intermediate+"abstract class ProjectionCursor<Q> extends ProjectionLayer<Long,Q>{", 1)
	case "renamed":
		source = strings.NewReplacer("ProjectionOwner", "IndependentCaptureNamespace", "reader", "heldCursor").Replace(source)
	case "method-variable", "method-shadow":
		source = strings.Replace(source, "java.util.Iterator<CharSequence> make(final CharSequence token)", "<E extends CharSequence> java.util.Iterator<E> make(final E token)", 1)
		source = strings.Replace(source, "final java.util.Iterator<CharSequence> reader=new ProjectionCursor<CharSequence>()", "final java.util.Iterator<E> reader=new ProjectionCursor<E>()", 1)
		source = strings.Replace(source, "public CharSequence next(){if(count++", "public E next(){if(count++", 1)
		source = strings.Replace(source, "new ProjectionForward<CharSequence>(){protected java.util.Iterator<CharSequence> delegate()", "new ProjectionForward<E>(){protected java.util.Iterator<E> delegate()", 1)
	}
	if shape == "hidden-bound" {
		source = strings.Replace(source, "class ProjectionOwner<E extends CharSequence> {", "class ProjectionOwner<K extends CharSequence,E extends K> {", 1)
		source = strings.Replace(source, "java.util.Iterator<E> make(final E token)", "<K extends Number> java.util.Iterator<E> make(final E token)", 1)
		source = strings.Replace(source, "new ProjectionOwner<CharSequence>().make(token)", "new ProjectionOwner<CharSequence,CharSequence>().<Integer>make(token)", 1)
	}
	if shape == "method-shadow" {
		source = strings.Replace(source, "class ProjectionOwner {", "class ProjectionOwner<E extends Number> {", 1)
		source = strings.Replace(source, "new ProjectionOwner().make(token)", "new ProjectionOwner<Integer>().make(token)", 1)
	}
	return source
}

func TestAdversarialCapturedAnonymousSupertypeKeepsGenericDeclarationAndABI(t *testing.T) {
	javac, java := t04Tools(t)
	for _, shape := range []string{"closed", "class-variable", "method-variable", "method-shadow", "hidden-bound", "composed", "diamond", "renamed"} {
		for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
			t.Run(shape+"/"+string(profile), func(t *testing.T) {
				compiler := javac
				if profile == NativeJavac8 {
					compiler = os.Getenv("JAVA8_JAVAC")
					if compiler == "" {
						t.Fatal("actual javac8 required")
					}
				}
				source := capturedAnonymousSupertypeShape(shape)
				owner := "ProjectionOwner"
				if shape == "renamed" {
					owner = "IndependentCaptureNamespace"
				}
				compile := func(debug string) map[string][]byte {
					if profile == NativeJavac8 {
						return nativePrivateEnumCompile(t, source, owner, debug)
					}
					return nativeCompileDebugClasses(t, source, debug)
				}
				original := t04RunJava(t, java, adversarialAssertionClasses(t, compile("none")), "ProjectionDriver")
				if original != "9:anonymous:generic-supertype:capture:binding:early:identity\n" {
					t.Fatal("original independent capture oracle", original)
				}
				testNativeIndependentCompilerFamilyWithPolicies(t, compile, profile, compiler, []string{owner}, "ProjectionDriver", original, nil, nil, []string{"normal", "no-source-rewrites", "no-core-cleanups"}, nativeLexicalExactSignatures)
			})
		}
	}
}
