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
