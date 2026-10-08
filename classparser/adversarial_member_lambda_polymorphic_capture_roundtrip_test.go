package javaclassparser

import (
	"strings"
	"testing"
)

const memberLambdaPolymorphicCaptureFixture = `
abstract class PolyCaptureValue {int word;final Object token;PolyCaptureValue(int word,Object token){this.word=word;this.token=token;}abstract int kind();}
class PolyCaptureLeft extends PolyCaptureValue {PolyCaptureLeft(int word,Object token){super(word,token);}int kind(){return 1;}}
class PolyCaptureRight extends PolyCaptureValue {PolyCaptureRight(int word,Object token){super(word,token);}int kind(){return 2;}}
class PolyCaptureEffects {static String trace="";static final RuntimeException failure=new RuntimeException("original");
 static PolyCaptureLeft left(int seed,Object token){trace+="L";if(seed==17)throw failure;return new PolyCaptureLeft(seed^0x76543210,token);}
 static PolyCaptureRight right(int seed,Object token){trace+="R";if(seed==17)throw failure;return new PolyCaptureRight(seed^0x13579bdf,token);}}
class PolyCaptureOwner {static class View {
 java.util.function.Supplier<PolyCaptureValue> operation(int seed,Object token,boolean choose){final PolyCaptureValue value;if(choose){value=PolyCaptureEffects.left(seed,token);}else{value=PolyCaptureEffects.right(seed,token);}return ()->{PolyCaptureEffects.trace+="B";return value;};}}
 static View view(){return new View();}}
class PolyCaptureDriver {public static void main(String[]args){int rows=0;Object marker=new Object();PolyCaptureOwner.View owner=PolyCaptureOwner.view();
 if(!owner.getClass().isMemberClass()||owner.getClass().getDeclaringClass()!=PolyCaptureOwner.class)throw new AssertionError("member identity");
 for(int seed:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(Object token:new Object[]{null,marker,"token"})for(boolean choose:new boolean[]{false,true}){
  PolyCaptureEffects.trace="";java.util.function.Supplier<PolyCaptureValue> operation=owner.operation(seed,token,choose);
  if(!PolyCaptureEffects.trace.equals(choose?"L":"R"))throw new AssertionError("selected producer once before factory");
  PolyCaptureEffects.trace="";PolyCaptureValue value=operation.get();if(!PolyCaptureEffects.trace.equals("B")||value.token!=token||value.word!=(seed^(choose?0x76543210:0x13579bdf))||value.kind()!=(choose?1:2)||value.getClass()!=(choose?PolyCaptureLeft.class:PolyCaptureRight.class))throw new AssertionError("selected subtype, value, token and lazy body");
  value.word^=0x654321ab;PolyCaptureEffects.trace="";if(operation.get()!=value||!PolyCaptureEffects.trace.equals("B")||value.word!=(seed^(choose?0x76543210:0x13579bdf)^0x654321ab))throw new AssertionError("reference identity and mutable content");rows++;
 }
 for(boolean choose:new boolean[]{false,true}){PolyCaptureEffects.trace="";try{owner.operation(17,marker,choose);throw new AssertionError("missing producer failure");}catch(RuntimeException error){if(error!=PolyCaptureEffects.failure||!PolyCaptureEffects.trace.equals(choose?"L":"R"))throw new AssertionError("selected producer failure order/identity");rows++;}}
 System.out.println(rows+":polymorphic:lambda:capture:identity:failure");}}
`

func TestAdversarialMemberLambdaJoinedCapturePreservesDifferentReferenceSubtypes(t *testing.T) {
	testNativeIndependentFamilyFixture(t, memberLambdaPolymorphicCaptureFixture, []string{"PolyCaptureOwner"}, "PolyCaptureDriver", "32:polymorphic:lambda:capture:identity:failure\n", nativeLexicalExactSignatures)
}

func memberLambdaInterfaceCaptureFixture() string {
	return strings.NewReplacer(
		"abstract class PolyCaptureValue {", "interface PolyCaptureContract {int kind();int read();Object token();void write(int n);}\nabstract class PolyCaptureValue implements PolyCaptureContract {public int read(){return word;}public Object token(){return token;}public void write(int n){word=n;}",
		"abstract int kind();", "public abstract int kind();",
		"int kind(){", "public int kind(){",
		"Supplier<PolyCaptureValue>", "Supplier<PolyCaptureContract>",
		"final PolyCaptureValue value;", "final PolyCaptureContract value;",
		"PolyCaptureValue value=operation.get();", "PolyCaptureContract value=operation.get();",
		"value.token!=token", "value.token()!=token",
		"value.word^=0x654321ab;", "value.write(value.read()^0x654321ab);",
		"value.word!=", "value.read()!=",
	).Replace(memberLambdaPolymorphicCaptureFixture)
}

func TestAdversarialMemberLambdaJoinedCapturePreservesInterfaceSubtypes(t *testing.T) {
	testNativeIndependentFamilyFixture(t, memberLambdaInterfaceCaptureFixture(), []string{"PolyCaptureOwner"}, "PolyCaptureDriver", "32:polymorphic:lambda:capture:identity:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberLambdaSelectedCapturePreservesInterfaceSubtypes(t *testing.T) {
	fixture := strings.Replace(memberLambdaInterfaceCaptureFixture(),
		"final PolyCaptureContract value;if(choose){value=PolyCaptureEffects.left(seed,token);}else{value=PolyCaptureEffects.right(seed,token);}",
		"final PolyCaptureContract value=choose?PolyCaptureEffects.left(seed,token):PolyCaptureEffects.right(seed,token);", 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"PolyCaptureOwner"}, "PolyCaptureDriver", "32:polymorphic:lambda:capture:identity:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberLambdaJoinedCapturePreservesNullableSubtypeResult(t *testing.T) {
	fixture := strings.NewReplacer(
		"return new PolyCaptureRight(seed^0x13579bdf,token);", "return seed==0?null:new PolyCaptureRight(seed^0x13579bdf,token);",
		"PolyCaptureValue value=operation.get();", "PolyCaptureValue value=operation.get();if(!choose&&seed==0){if(value!=null||!PolyCaptureEffects.trace.equals(\"B\")||operation.get()!=null||!PolyCaptureEffects.trace.equals(\"BB\"))throw new AssertionError(\"selected nullable subtype and lazy identity\");rows++;continue;}",
	).Replace(memberLambdaPolymorphicCaptureFixture)
	testNativeIndependentFamilyFixture(t, fixture, []string{"PolyCaptureOwner"}, "PolyCaptureDriver", "32:polymorphic:lambda:capture:identity:failure\n", nativeLexicalExactSignatures)
}

func TestAdversarialMemberLambdaSelectedCapturePreservesDifferentReferenceSubtypes(t *testing.T) {
	fixture := strings.Replace(memberLambdaPolymorphicCaptureFixture,
		"final PolyCaptureValue value;if(choose){value=PolyCaptureEffects.left(seed,token);}else{value=PolyCaptureEffects.right(seed,token);}",
		"final PolyCaptureValue value=choose?PolyCaptureEffects.left(seed,token):PolyCaptureEffects.right(seed,token);", 1)
	testNativeIndependentFamilyFixture(t, fixture, []string{"PolyCaptureOwner"}, "PolyCaptureDriver", "32:polymorphic:lambda:capture:identity:failure\n", nativeLexicalExactSignatures)
}

const memberLambdaPolymorphicArrayCaptureFixture = `
interface ArrayCaptureContract {int word();Object token();}
class ArrayCaptureLeft implements ArrayCaptureContract {int word;final Object token;ArrayCaptureLeft(int word,Object token){this.word=word;this.token=token;}public int word(){return word;}public Object token(){return token;}}
class ArrayCaptureRight implements ArrayCaptureContract {int word;final Object token;ArrayCaptureRight(int word,Object token){this.word=word;this.token=token;}public int word(){return word;}public Object token(){return token;}}
class ArrayCaptureEffects {static String trace="";static final RuntimeException failure=new RuntimeException("original");
 static ArrayCaptureLeft[] left(int seed,Object token){trace+="L";if(seed==17)throw failure;return new ArrayCaptureLeft[]{new ArrayCaptureLeft(seed^0x76543210,token)};}
 static ArrayCaptureRight[] right(int seed,Object token){trace+="R";if(seed==17)throw failure;return new ArrayCaptureRight[]{new ArrayCaptureRight(seed^0x13579bdf,token)};}}
class ArrayCaptureOwner {static class View {
 java.util.function.Supplier<ArrayCaptureContract[]> operation(int seed,Object token,boolean choose){final ArrayCaptureContract[] value;if(choose)value=ArrayCaptureEffects.left(seed,token);else value=ArrayCaptureEffects.right(seed,token);return ()->{ArrayCaptureEffects.trace+="B";return value;};}}
 static View view(){return new View();}}
class ArrayCaptureDriver {public static void main(String[]args){int rows=0;Object marker=new Object();ArrayCaptureOwner.View owner=ArrayCaptureOwner.view();
 if(!owner.getClass().isMemberClass()||owner.getClass().getDeclaringClass()!=ArrayCaptureOwner.class)throw new AssertionError("member identity");
 for(int seed:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(Object token:new Object[]{null,marker,"token"})for(boolean choose:new boolean[]{false,true}){
  ArrayCaptureEffects.trace="";java.util.function.Supplier<ArrayCaptureContract[]> operation=owner.operation(seed,token,choose);
  if(!ArrayCaptureEffects.trace.equals(choose?"L":"R"))throw new AssertionError("selected producer once before factory");
  ArrayCaptureEffects.trace="";ArrayCaptureContract[] value=operation.get();if(!ArrayCaptureEffects.trace.equals("B")||value.length!=1||value[0].token()!=token||value[0].word()!=(seed^(choose?0x76543210:0x13579bdf))||value.getClass()!=(choose?ArrayCaptureLeft[].class:ArrayCaptureRight[].class))throw new AssertionError("array covariance, subtype, token and lazy body");
  ArrayCaptureContract first=value[0];ArrayCaptureEffects.trace="";if(operation.get()!=value||operation.get()[0]!=first||!ArrayCaptureEffects.trace.equals("BB"))throw new AssertionError("array reference identity");
  try{value[0]=choose?new ArrayCaptureRight(0,token):new ArrayCaptureLeft(0,token);throw new AssertionError("missing array store failure");}catch(ArrayStoreException error){if(value[0]!=first)throw new AssertionError("array store failure preserves content");}
  value[0]=null;ArrayCaptureEffects.trace="";if(operation.get()[0]!=null||!ArrayCaptureEffects.trace.equals("B"))throw new AssertionError("captured array content mutation");rows++;
 }
 for(boolean choose:new boolean[]{false,true}){ArrayCaptureEffects.trace="";try{owner.operation(17,marker,choose);throw new AssertionError("missing producer failure");}catch(RuntimeException error){if(error!=ArrayCaptureEffects.failure||!ArrayCaptureEffects.trace.equals(choose?"L":"R"))throw new AssertionError("selected producer failure order/identity");rows++;}}
 System.out.println(rows+":polymorphic:array:capture:identity:failure");}}
`

func TestAdversarialMemberLambdaJoinedCapturePreservesCovariantArraySubtypes(t *testing.T) {
	testNativeIndependentFamilyFixture(t, memberLambdaPolymorphicArrayCaptureFixture, []string{"ArrayCaptureOwner"}, "ArrayCaptureDriver", "32:polymorphic:array:capture:identity:failure\n", nativeLexicalExactSignatures)
}
