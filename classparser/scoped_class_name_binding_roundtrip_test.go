package javaclassparser

import (
	"regexp"
	"strings"
	"testing"
)

const scopedClassNameBindingFixture = `class ScopedClassBindingOwner<String extends Number> {
 java.lang.String cast(Object input){return(java.lang.String)input;}
 java.lang.String print(Object input){return java.lang.String.valueOf(input);}
 java.lang.String[] array(int count){return new java.lang.String[count];}
 Class<?> token(){return java.lang.String.class;}
}
class ScopedClassBindingDriver{public static void main(String[]args){ScopedClassBindingOwner<Integer>owner=new ScopedClassBindingOwner<Integer>();java.lang.String text=new java.lang.String("typed");if(owner.cast(text)!=text||owner.cast(null)!=null||!owner.print(42).equals("42")||!owner.print(null).equals("null")||owner.array(2).getClass()!=java.lang.String[].class||owner.token()!=java.lang.String.class)throw new AssertionError("class declaration binding");try{owner.cast(42);throw new AssertionError("original CLASS cast");}catch(ClassCastException expected){}System.out.println("scope:class:cast:array:token:call");}}
`

func TestScopedClassNameBindingClassFormalRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, scopedClassNameBindingFixture, "ScopedClassBindingOwner", "ScopedClassBindingDriver", "scope:class:cast:array:token:call\n")
}
func TestScopedClassNameBindingMethodFormalRoundTrip(t *testing.T) {
	f := strings.Replace(scopedClassNameBindingFixture, "class ScopedClassBindingOwner<String extends Number>", "class ScopedClassBindingOwner", 1)
	f = strings.ReplaceAll(f, "ScopedClassBindingOwner<Integer>", "ScopedClassBindingOwner ")
	f = strings.Replace(f, "java.lang.String cast(Object input)", "<String extends Number>java.lang.String cast(Object input)", 1)
	f = strings.Replace(f, "java.lang.String print(Object input)", "<String extends Number>java.lang.String print(Object input)", 1)
	f = strings.Replace(f, "java.lang.String[] array(int count)", "<String extends Number>java.lang.String[] array(int count)", 1)
	f = strings.Replace(f, "Class<?> token()", "<String extends Number>Class<?> token()", 1)
	testNativePrivateSetterFixture(t, f, "ScopedClassBindingOwner", "ScopedClassBindingDriver", "scope:class:cast:array:token:call\n")
}
func TestScopedClassNameBindingImportedClassRoundTrip(t *testing.T) {
	f := strings.Replace(scopedClassNameBindingFixture, "class ScopedClassBindingOwner<String extends Number>", "class ScopedClassBindingOwner<ArrayList extends Number>", 1)
	f = strings.Replace(f, "java.lang.String[] array(int count){return new java.lang.String[count];}", "java.lang.String[] array(int count){return new java.lang.String[count];}java.util.ArrayList<Object>list(){return new java.util.ArrayList<Object>();}", 1)
	f = strings.Replace(f, "java.lang.String text=new java.lang.String", "if(!owner.list().isEmpty()||owner.list().getClass()!=java.util.ArrayList.class)throw new AssertionError(\"imported CLASS binding\");java.lang.String text=new java.lang.String", 1)
	testNativePrivateSetterFixture(t, f, "ScopedClassBindingOwner", "ScopedClassBindingDriver", "scope:class:cast:array:token:call\n")
}
func TestScopedClassNameBindingRenamedRoundTrip(t *testing.T) {
	f := regexp.MustCompile(`\bScopedClassBindingOwner\b`).ReplaceAllString(scopedClassNameBindingFixture, "OtherDeclarationScope")
	f = strings.ReplaceAll(f, "String extends Number", "String extends java.lang.Number")
	testNativePrivateSetterFixture(t, f, "OtherDeclarationScope", "ScopedClassBindingDriver", "scope:class:cast:array:token:call\n")
}

func TestScopedClassNameBindingKeepsOwnFormalRoundTrip(t *testing.T) {
	f := strings.Replace(scopedClassNameBindingFixture, "java.lang.String cast(Object input)", "String identity(String input){return input;}java.lang.String cast(Object input)", 1)
	f = strings.Replace(f, "java.lang.String text=new java.lang.String", `Integer number=1001;if(owner.identity(number)!=number)throw new AssertionError("bare formal binding");try{java.lang.reflect.Method identity=ScopedClassBindingOwner.class.getDeclaredMethod("identity",Number.class);java.lang.reflect.TypeVariable<?>variable=(java.lang.reflect.TypeVariable<?>)identity.getGenericReturnType();if(!variable.getName().equals("String")||variable.getGenericDeclaration()!=ScopedClassBindingOwner.class||variable.getBounds()[0]!=Number.class)throw new AssertionError("formal declaration identity");}catch(ReflectiveOperationException failure){throw new AssertionError(failure);}java.lang.String text=new java.lang.String`, 1)
	testNativePrivateSetterFixture(t, f, "ScopedClassBindingOwner", "ScopedClassBindingDriver", "scope:class:cast:array:token:call\n")
}
func TestScopedClassNameBindingNestedClassOwnerRoundTrip(t *testing.T) {
	f := `class NestedClassBindingOwner<Map extends Number>{java.util.Map.Entry<Object,Object>entry(Object key,Object value){return new java.util.AbstractMap.SimpleEntry<Object,Object>(key,value);}Class<?>token(){return java.util.Map.Entry.class;}}
 class NestedClassBindingDriver{public static void main(String[]args){NestedClassBindingOwner<Integer>owner=new NestedClassBindingOwner<Integer>();Object key=new Object(),value=new Object();java.util.Map.Entry<Object,Object>entry=owner.entry(key,value);if(entry.getKey()!=key||entry.getValue()!=value||owner.token()!=java.util.Map.Entry.class)throw new AssertionError("nested class owner binding");System.out.println("scope:nested:owner:binding:identity");}}`
	testNativePrivateSetterFixture(t, f, "NestedClassBindingOwner", "NestedClassBindingDriver", "scope:nested:owner:binding:identity\n")
}
