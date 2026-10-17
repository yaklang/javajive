package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const nativeMemberLexicalArgumentFixture = `
class LexicalEntryParent {final Object observed;LexicalEntryParent(){observed=owner();}Object owner(){return null;}}
class LexicalEntryOwner<Alpha,Beta> {
 class Entry extends LexicalEntryParent {final Alpha key;final Beta value;Entry(Alpha key,Beta value){this.key=key;this.value=value;}Object owner(){return LexicalEntryOwner.this;}}
 java.util.concurrent.Callable<Entry> make(final Object key,final Object value){return new java.util.concurrent.Callable<Entry>(){public Entry call(){return new Entry((Alpha)key,(Beta)value);}};}
}
class LexicalEntryDriver {public static void main(String[]args)throws Exception{
 LexicalEntryOwner<Object,Object> owner=new LexicalEntryOwner<Object,Object>();Object token=new Object();int rows=0;
 Object[] values={null,token,"text",new int[]{1,2}};
 for(Object key:values)for(Object value:values){
  java.util.concurrent.Callable<LexicalEntryOwner<Object,Object>.Entry> call=owner.make(key,value);
  for(boolean cleared:new boolean[]{false,true}){
   if(cleared){int count=0;for(java.lang.reflect.Field field:call.getClass().getDeclaredFields())if(field.getType()==LexicalEntryOwner.class){field.setAccessible(true);field.set(call,null);count++;}if(count!=1)throw new AssertionError("original enclosing storage");}
   LexicalEntryOwner<Object,Object>.Entry entry=call.call();Object expected=cleared?null:owner;
   if(entry.key!=key||entry.value!=value||entry.owner()!=expected||entry.observed!=expected)throw new AssertionError("identity, nullable enclosing or pre-super callback");
   if(entry.getClass().getDeclaringClass()!=LexicalEntryOwner.class||!call.getClass().isAnonymousClass())throw new AssertionError("lexical declaration identity");rows++;
  }
 }
 System.out.println(rows+":lexical:arguments:nullable:callback");
}}
`

func TestNativeMemberConstructorLexicalArgumentsRoundTrip(t *testing.T) {
	testNativeIndependentFamilyFixture(t, nativeMemberLexicalArgumentFixture, []string{"LexicalEntryOwner"}, "LexicalEntryDriver", "32:lexical:arguments:nullable:callback\n", nativeLexicalExactSignatures)
}

func TestNativeMemberConstructorLexicalArgumentsRenamedRoundTrip(t *testing.T) {
	f := strings.NewReplacer("LexicalEntryOwner", "OtherArgumentScope", "Alpha", "Left", "Beta", "Right").Replace(nativeMemberLexicalArgumentFixture)
	testNativeIndependentFamilyFixture(t, f, []string{"OtherArgumentScope"}, "LexicalEntryDriver", "32:lexical:arguments:nullable:callback\n", nativeLexicalExactSignatures)
}

func TestNativeMemberConstructorLexicalArgumentsBoundedRoundTrip(t *testing.T) {
	f := strings.NewReplacer(
		"LexicalEntryOwner<Alpha,Beta>", "LexicalEntryOwner<Alpha extends Number,Beta extends CharSequence>",
		"make(final Object key,final Object value)", "make(final Number key,final CharSequence value)",
		"LexicalEntryOwner<Object,Object>", "LexicalEntryOwner<Number,CharSequence>",
		"for(Object key:values)for(Object value:values)", "for(Number key:new Number[]{null,Integer.MIN_VALUE,Integer.MAX_VALUE,new java.math.BigDecimal(\"0.125\")})for(CharSequence value:new CharSequence[]{null,\"\",\"text\",new StringBuilder(\"live\")})",
	).Replace(nativeMemberLexicalArgumentFixture)
	testNativeIndependentFamilyFixture(t, f, []string{"LexicalEntryOwner"}, "LexicalEntryDriver", "32:lexical:arguments:nullable:callback\n", nativeLexicalExactSignatures)
}

func TestNativeMemberConstructorLexicalArgumentsNativeRoundTrip(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Fatal("JAVA8_JAVAC required")
	}
	testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
		return nativePrivateEnumCompile(t, nativeMemberLexicalArgumentFixture, "LexicalEntryOwner", debug)
	}, NativeJavac8, javac, []string{"LexicalEntryOwner"}, "LexicalEntryDriver", "32:lexical:arguments:nullable:callback\n", nil, nativeLexicalExactSignatures)
}

func nativeMemberLexicalLocalArgumentFixture() string {
	original := "java.util.concurrent.Callable<Entry> make(final Object key,final Object value){return new java.util.concurrent.Callable<Entry>(){public Entry call(){return new Entry((Alpha)key,(Beta)value);}};}"
	local := "java.util.concurrent.Callable<Entry> make(final Object key,final Object value){class Local implements java.util.concurrent.Callable<Entry>{public Entry call(){return new Entry((Alpha)key,(Beta)value);}}return new Local();}"
	return strings.Replace(strings.Replace(nativeMemberLexicalArgumentFixture, original, local, 1), "!call.getClass().isAnonymousClass()", "!call.getClass().isLocalClass()", 1)
}
func TestAdversarialMethodLocalMemberConstructorRetainsLexicalFormalPorts(t *testing.T) {
	for _, rename := range []string{"original", "renamed"} {
		t.Run(rename, func(t *testing.T) {
			source := nativeMemberLexicalLocalArgumentFixture()
			owner := "LexicalEntryOwner"
			if rename == "renamed" {
				source = strings.NewReplacer("LexicalEntryOwner", "OtherArgumentRoot", "Alpha", "Left", "Beta", "Right").Replace(source)
				owner = "OtherArgumentRoot"
			}
			testNativeIndependentFamilyFixture(t, source, []string{owner}, "LexicalEntryDriver", "32:lexical:arguments:nullable:callback\n", nativeLexicalExactSignatures)
		})
	}
}

func nativeMemberLexicalErasedLocalArgumentFixture() string {
	original := "java.util.concurrent.Callable<Entry> make(final Object key,final Object value){return new java.util.concurrent.Callable<Entry>(){public Entry call(){return new Entry((Alpha)key,(Beta)value);}};}"
	local := "java.util.concurrent.Callable<Object> make(final Object key,final Object value){class Local implements java.util.concurrent.Callable<Object>{public Object call(){return new Entry((Alpha)key,(Beta)value);}}return new Local();}"
	source := strings.Replace(nativeMemberLexicalArgumentFixture, original, local, 1)
	source = strings.Replace(source, "java.util.concurrent.Callable<LexicalEntryOwner<Object,Object>.Entry>", "java.util.concurrent.Callable<Object>", 1)
	source = strings.Replace(source, ".Entry entry=call.call()", ".Entry entry=(LexicalEntryOwner<Object,Object>.Entry)call.call()", 1)
	return strings.Replace(source, "!call.getClass().isAnonymousClass()", "!call.getClass().isLocalClass()", 1)
}
func TestAdversarialMethodLocalErasedMemberConstructorKeepsFormalPorts(t *testing.T) {
	for _, rename := range []string{"original", "renamed"} {
		t.Run(rename, func(t *testing.T) {
			source := nativeMemberLexicalErasedLocalArgumentFixture()
			owner := "LexicalEntryOwner"
			if rename == "renamed" {
				source = strings.NewReplacer("LexicalEntryOwner", "OtherArgumentRoot", "Alpha", "Left", "Beta", "Right").Replace(source)
				owner = "OtherArgumentRoot"
			}
			testNativeIndependentFamilyFixture(t, source, []string{owner}, "LexicalEntryDriver", "32:lexical:arguments:nullable:callback\n", nativeLexicalExactSignatures)
		})
	}
}

func nativeMemberLexicalBoundedLocalArgumentFixture() string {
	return strings.NewReplacer(
		"LexicalEntryOwner<Alpha,Beta>", "LexicalEntryOwner<Alpha extends Number,Beta extends CharSequence>",
		"make(final Object key,final Object value)", "make(final Number key,final CharSequence value)",
		"LexicalEntryOwner<Object,Object>", "LexicalEntryOwner<Number,CharSequence>",
		"for(Object key:values)for(Object value:values)", "for(Number key:new Number[]{null,Integer.MIN_VALUE,Integer.MAX_VALUE,new java.math.BigDecimal(\"0.125\")})for(CharSequence value:new CharSequence[]{null,\"\",\"text\",new StringBuilder(\"live\")})",
	).Replace(nativeMemberLexicalLocalArgumentFixture())
}

func TestAdversarialMethodLocalMemberConstructorKeepsBoundedErasure(t *testing.T) {
	for _, rename := range []string{"original", "renamed"} {
		t.Run(rename, func(t *testing.T) {
			source, owner := nativeMemberLexicalBoundedLocalArgumentFixture(), "LexicalEntryOwner"
			if rename == "renamed" {
				source = strings.NewReplacer("LexicalEntryOwner", "OtherBoundedScope", "Alpha", "Left", "Beta", "Right").Replace(source)
				owner = "OtherBoundedScope"
			}
			testNativeIndependentFamilyFixture(t, source, []string{owner}, "LexicalEntryDriver", "32:lexical:arguments:nullable:callback\n", nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialMethodLocalMemberConstructorKeepsUnrelatedFactoryFormal(t *testing.T) {
	source := strings.Replace(nativeMemberLexicalLocalArgumentFixture(), "java.util.concurrent.Callable<Entry> make(", "<FactoryValue extends Number> java.util.concurrent.Callable<Entry> make(", 1)
	testNativeIndependentFamilyFixture(t, source, []string{"LexicalEntryOwner"}, "LexicalEntryDriver", "32:lexical:arguments:nullable:callback\n", nativeLexicalExactSignatures)
}

func TestAdversarialMethodLocalMemberConstructorOriginalJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Fatal("JAVA8_JAVAC required for the original compiler and independent ABI oracle")
	}
	for _, fixture := range []struct{ name, source string }{
		{"typed", nativeMemberLexicalLocalArgumentFixture()},
		{"erased", nativeMemberLexicalErasedLocalArgumentFixture()},
		{"bounded", nativeMemberLexicalBoundedLocalArgumentFixture()},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
				return nativePrivateEnumCompile(t, fixture.source, "LexicalEntryOwner", debug)
			}, NativeJavac8, javac, []string{"LexicalEntryOwner"}, "LexicalEntryDriver", "32:lexical:arguments:nullable:callback\n", nil, nativeLexicalExactSignatures)
		})
	}
}
