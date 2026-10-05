package javaclassparser

import (
	"strings"
	"testing"
)

const nativeRootAccessorFamilyFixture = `abstract class MemberPhaseBase{MemberPhaseBase(){if(read()!=9)throw new AssertionError("original enclosing capture before parent callback");}abstract int read();}
class MemberPhaseResult{final MemberPhaseBase base;MemberPhaseResult(MemberPhaseBase b){base=b;}MemberPhaseResult(Object b){throw new AssertionError("wrong constructor binding");}}
class MemberPhaseOwner{private final int number;MemberPhaseOwner(int n){number=n;}class Left{MemberPhaseResult result(){return new MemberPhaseResult(new Right());}}class Right extends MemberPhaseBase{Right(){super();}int read(){return number;}}MemberPhaseResult result(){return new Left().result();}}
class MemberPhaseDriver{public static void main(String[]args){MemberPhaseOwner owner=new MemberPhaseOwner(9);MemberPhaseResult result=owner.result();if(result.base.read()!=9||!result.base.getClass().getName().equals("MemberPhaseOwner$Right"))throw new AssertionError("enclosing receiver binding");System.out.println("member:enclosing:phase:callback:binding");}}`

func TestNativeRootAccessorFamilyRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeRootAccessorFamilyFixture, "MemberPhaseOwner", "MemberPhaseDriver", "member:enclosing:phase:callback:binding\n")
}
func TestNativeRootAccessorFamilyRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeRootAccessorFamilyFixture, "MemberPhaseOwner", "OtherMemberPreparationScope")
	testNativePrivateSetterFixture(t, f, "OtherMemberPreparationScope", "MemberPhaseDriver", "member:enclosing:phase:callback:binding\n")
}

const nativeRootAccessorValuesFixture = `class AccessValuesOwner{private Object token;private long number;AccessValuesOwner(Object value,long n){token=value;number=n;}class Reader{Object token(AccessValuesOwner owner){return owner.token;}long number(AccessValuesOwner owner){return owner.number;}}Reader reader(){return new Reader();}}
class AccessValuesDriver{public static void main(String[]args){int rows=0;for(Object value:new Object[]{null,new Object()})for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){AccessValuesOwner owner=new AccessValuesOwner(value,n);AccessValuesOwner.Reader reader=owner.reader();if(reader.token(owner)!=value||reader.number(owner)!=n)throw new AssertionError("erasure/wide/value");try{reader.token(null);throw new AssertionError("missing dereference");}catch(NullPointerException expected){}rows++;}System.out.println(rows+":original:accessors:values");}}`

func TestNativeRootAccessorValuesRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeRootAccessorValuesFixture, "AccessValuesOwner", "AccessValuesDriver", "10:original:accessors:values\n")
}
func TestNativeRootAccessorStaticMemberRoundTrip(t *testing.T) {
	f := strings.Replace(nativeRootAccessorValuesFixture, "class Reader{", "static class Reader{", 1)
	testNativePrivateSetterFixture(t, f, "AccessValuesOwner", "AccessValuesDriver", "10:original:accessors:values\n")
}

const nativeRootAccessorWriteFixture = `class AccessWriteEffects{static String trace="";static int fail;static final RuntimeException error=new RuntimeException("original");static AccessWriteOwner receiver(AccessWriteOwner owner){trace+="R";if(fail==1)throw error;return owner;}static Object value(Object value){trace+="V";if(fail==2)throw error;return value;}}
class AccessWriteOwner{private Object token;class Writer{Object put(AccessWriteOwner owner,Object value){return AccessWriteEffects.receiver(owner).token=AccessWriteEffects.value(value);}Object read(AccessWriteOwner owner){return owner.token;}}Writer writer(){return new Writer();}}
class AccessWriteDriver{public static void main(String[]args){AccessWriteOwner owner=new AccessWriteOwner();AccessWriteOwner.Writer writer=owner.writer();Object value=new Object();AccessWriteEffects.trace="";if(writer.put(owner,value)!=value||writer.read(owner)!=value||!AccessWriteEffects.trace.equals("RV"))throw new AssertionError("assignment result/order");for(int fail:new int[]{0,1,2}){AccessWriteEffects.fail=fail;AccessWriteEffects.trace="";try{writer.put(null,value);throw new AssertionError("missing failure");}catch(RuntimeException e){if(fail==0?!(e instanceof NullPointerException):e!=AccessWriteEffects.error)throw new AssertionError("failure identity");}if(!AccessWriteEffects.trace.equals(fail==1?"R":"RV"))throw new AssertionError("failure order");}System.out.println("original:accessors:write:order:identity");}}`

func TestNativeRootAccessorWriteRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeRootAccessorWriteFixture, "AccessWriteOwner", "AccessWriteDriver", "original:accessors:write:order:identity\n")
}

const nativeRootAccessorCallFixture = `class AccessCallEffects{static String trace="";static final java.io.IOException error=new java.io.IOException("original");}
class AccessCallOwner{private Object prepare(Object value,long n)throws java.io.IOException{AccessCallEffects.trace+="P";if(value==null)throw AccessCallEffects.error;if(n!=Long.MIN_VALUE)throw new AssertionError("wide");return value;}private Object prepare(String value,long n){throw new AssertionError("wrong overload");}class Reader{Object get(Object value,long n)throws java.io.IOException{return prepare(value,n);}}Reader reader(){return new Reader();}}
class AccessCallDerived extends AccessCallOwner{public Object prepare(Object value,long n){throw new AssertionError("wrong virtual target");}}
class AccessCallDriver{public static void main(String[]args)throws Exception{AccessCallOwner owner=new AccessCallDerived();AccessCallOwner.Reader reader=owner.reader();Object token=new Object();AccessCallEffects.trace="";if(reader.get(token,Long.MIN_VALUE)!=token||!AccessCallEffects.trace.equals("P"))throw new AssertionError("result/once");AccessCallEffects.trace="";try{reader.get(null,Long.MIN_VALUE);throw new AssertionError("missing checked failure");}catch(java.io.IOException e){if(e!=AccessCallEffects.error||!AccessCallEffects.trace.equals("P"))throw new AssertionError("checked identity/binding");}System.out.println("original:accessors:call:nonvirtual:overload:checked");}}`

func TestNativeRootAccessorCallRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeRootAccessorCallFixture, "AccessCallOwner", "AccessCallDriver", "original:accessors:call:nonvirtual:overload:checked\n")
}

func TestNativeRootAccessorRequiresClosedArchiveReferences(t *testing.T) {
	testNativeMemberPrivateGetterArchiveReferences(t, nativeRootAccessorValuesFixture, "AccessValuesOwner", "AccessValuesOwner", "AccessValuesOwner$Reader", "token", false)
}
func TestNativeRootAccessorStaticMemberRequiresClosedArchiveReferences(t *testing.T) {
	f := strings.Replace(nativeRootAccessorValuesFixture, "class Reader{", "static class Reader{", 1)
	testNativeMemberPrivateGetterArchiveReferences(t, f, "AccessValuesOwner", "AccessValuesOwner", "AccessValuesOwner$Reader", "token", false)
}

func TestNativeRootAccessorGenericBindingRoundTrip(t *testing.T) {
	fixture := `class AccessGenericProbe{static String pick(Number x){return "number";}static String pick(CharSequence x){return "text";}static String pick(Object x){return "object";}}
class AccessGenericOwner<T extends Number>{private final T value;private final CharSequence text;AccessGenericOwner(T value,CharSequence text){this.value=value;this.text=text;}class Reader{<T extends CharSequence> Number value(){return value;}<T extends Number> CharSequence text(){return text;}<T extends CharSequence> String kinds(){return AccessGenericProbe.pick(value)+":"+AccessGenericProbe.pick(text);}}Reader reader(){return new Reader();}}
class AccessGenericDriver{public static void main(String[]args){Integer number=Integer.valueOf(17);String text=new String("original");AccessGenericOwner<Integer> owner=new AccessGenericOwner<Integer>(number,text);AccessGenericOwner<Integer>.Reader reader=owner.reader();if(reader.value()!=number||reader.text()!=text||!reader.kinds().equals("number:text"))throw new AssertionError("original erasure/lexical formal/overload binding");System.out.println("original:accessors:generic:binding");}}`
	testNativePrivateSetterFixture(t, fixture, "AccessGenericOwner", "AccessGenericDriver", "original:accessors:generic:binding\n")
}
