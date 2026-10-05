package javaclassparser

import (
	"regexp"
	"strings"
	"testing"
)

const nativeAnonymousMethodFormalShadow = `abstract class FormalShadowParent<T extends Number>{abstract int number();abstract Object captured();abstract <T extends CharSequence>T echo(T input);abstract <T>T unchecked();}
class FormalShadowOwner<T extends Number>{FormalShadowParent<T>make(final T value){return new FormalShadowParent<T>(){int number(){return value.intValue();}Object captured(){return value;}<T extends CharSequence>T echo(T input){return input;}<T>T unchecked(){return(T)value;}};}}
class FormalShadowDriver {public static void main(String[]args)throws Exception {Integer value=42;FormalShadowParent<Integer>p=new FormalShadowOwner<Integer>().make(value);String token=new String("method");if(p.number()!=42||p.captured()!=value||p.echo(token)!=token||p.echo((String)null)!=null||p.<Number>unchecked()!=value)throw new AssertionError("independent class/method formal binding");try{String wrong=p.<String>unchecked();throw new AssertionError("original caller cast did not fail:"+wrong);}catch(ClassCastException expected){}java.lang.reflect.Method echo=p.getClass().getDeclaredMethod("echo",CharSequence.class),unchecked=p.getClass().getDeclaredMethod("unchecked");if(echo.getTypeParameters().length!=1||echo.getTypeParameters()[0].getBounds()[0]!=CharSequence.class||unchecked.getTypeParameters().length!=1||unchecked.getTypeParameters()[0].getBounds()[0]!=Object.class||FormalShadowOwner.class.getTypeParameters()[0].getBounds()[0]!=Number.class)throw new AssertionError("formal declaration identities/bounds");System.out.println("formal:class:method:identity:cast");}}
`

func TestNativeAnonymousMethodFormalShadowRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeAnonymousMethodFormalShadow, "FormalShadowOwner", "FormalShadowDriver", "formal:class:method:identity:cast\n")
}
func TestNativeAnonymousMethodFormalShadowRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeAnonymousMethodFormalShadow, "FormalShadowOwner", "SeparateLexicalGenericOwner")
	f = regexp.MustCompile(`\bT\b`).ReplaceAllString(f, "Q")
	testNativePrivateSetterFixture(t, f, "SeparateLexicalGenericOwner", "FormalShadowDriver", "formal:class:method:identity:cast\n")
}

func TestNativeAnonymousMethodFormalShadowOverloadRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousMethodFormalShadow, "abstract <T>T unchecked();", "abstract <T>T unchecked();abstract <T extends CharSequence>String classify();abstract <T extends CharSequence>T checked();", 1)
	f = strings.Replace(f, "<T>T unchecked(){return(T)value;}", "<T>T unchecked(){return(T)value;}<T extends CharSequence>String classify(){return FormalShadowOverloads.kind(value);}<T extends CharSequence>T checked(){return(T)(Object)value;}", 1)
	f = strings.Replace(f, "String token=new String", "if(!p.<String>classify().equals(\"number\"))throw new AssertionError(\"lexical capture rebound to method formal\");try{p.<String>checked();throw new AssertionError(\"actual method CHECKCAST must fail\");}catch(ClassCastException expected){}String token=new String", 1)
	f += `class FormalShadowOverloads{static String kind(Number n){return "number";}static String kind(CharSequence c){return "sequence";}static String kind(Object o){return "object";}}`
	testNativePrivateSetterFixture(t, f, "FormalShadowOwner", "FormalShadowDriver", "formal:class:method:identity:cast\n")
}
func TestNativeAnonymousMethodFormalShadowArrayRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousMethodFormalShadow, "make(final T value)", "make(final T value,final T[] array)", 1)
	f = strings.Replace(f, "abstract <T>T unchecked();", "abstract <T>T unchecked();abstract <T extends CharSequence>Number[] array();", 1)
	f = strings.Replace(f, "<T>T unchecked(){return(T)value;}", "<T>T unchecked(){return(T)value;}<T extends CharSequence>Number[] array(){return array;}", 1)
	f = strings.Replace(f, "FormalShadowParent<Integer>p=new FormalShadowOwner<Integer>().make(value);", "Integer[]array={value};FormalShadowParent<Integer>p=new FormalShadowOwner<Integer>().make(value,array);if(p.<String>array()!=array||p.<String>array()[0]!=value)throw new AssertionError(\"outer capture array declaration identity\");", 1)
	testNativePrivateSetterFixture(t, f, "FormalShadowOwner", "FormalShadowDriver", "formal:class:method:identity:cast\n")
}

func TestNativeAnonymousMethodFormalShadowIndependentCaptureRoundTrip(t *testing.T) {
	f := strings.Replace(nativeAnonymousMethodFormalShadow, "class FormalShadowOwner<T extends Number>", "class FormalShadowOwner<T extends Number,U extends CharSequence>", 1)
	f = strings.Replace(f, "make(final T value)", "make(final T value,final U text)", 1)
	f = strings.Replace(f, "<T>T unchecked(){return(T)value;}", "<T>T unchecked(){return(T)value;}<T extends CharSequence>U text(){return text;}", 1)
	f = strings.Replace(f, "FormalShadowParent<Integer>p=new FormalShadowOwner<Integer>().make(value);", "FormalShadowParent<Integer>p=new FormalShadowOwner<Integer,String>().make(value,\"other\");", 1)
	testNativePrivateSetterFixture(t, f, "FormalShadowOwner", "FormalShadowDriver", "formal:class:method:identity:cast\n")
}

func TestNativeAnonymousMethodFormalShadowOptionalHintDisabledRoundTrip(t *testing.T) {
	t.Setenv("JDEC_TYPEVAR_BOUND_RECV_OFF", "1")
	testNativePrivateSetterFixture(t, nativeAnonymousMethodFormalShadow, "FormalShadowOwner", "FormalShadowDriver", "formal:class:method:identity:cast\n")
}

func TestNativeAnonymousMethodFormalShadowOuterMethodRoundTrip(t *testing.T) {
	f := `abstract class OuterMethodShadowParent<E extends Number>{abstract <E extends CharSequence>Number get();abstract <E>Object copy();}
 class OuterMethodShadowOwner{static <E extends Number>OuterMethodShadowParent<E> make(final java.util.List<E> list){return new OuterMethodShadowParent<E>(){<E extends CharSequence>Number get(){return list.get(0);}<E>Object copy(){return list;}};}}
 class OuterMethodShadowDriver{public static void main(String[]args)throws Exception{Integer number=42;java.util.List<Integer>list=java.util.Collections.singletonList(number);OuterMethodShadowParent<Integer>p=OuterMethodShadowOwner.make(list);if(p.<String>get()!=number||p.<String>copy()!=list)throw new AssertionError("outer method capture identity");java.lang.reflect.Method get=p.getClass().getDeclaredMethod("get"),make=OuterMethodShadowOwner.class.getDeclaredMethod("make",java.util.List.class);if(get.getTypeParameters()[0].getBounds()[0]!=CharSequence.class||make.getTypeParameters()[0].getBounds()[0]!=Number.class)throw new AssertionError("outer and current method declarations");try{OuterMethodShadowOwner.make(java.util.Collections.<Integer>emptyList()).get();throw new AssertionError("list failure missing");}catch(IndexOutOfBoundsException expected){}System.out.println("outer:method:formal:capture:list:identity");}}`
	testNativePrivateSetterFixture(t, f, "OuterMethodShadowOwner", "OuterMethodShadowDriver", "outer:method:formal:capture:list:identity\n")
}
