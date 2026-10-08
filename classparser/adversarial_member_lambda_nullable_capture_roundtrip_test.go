package javaclassparser

import "testing"

const memberLambdaNullableCaptureFixture = `
class NullableCaptureBox {int word;NullableCaptureBox(int word){this.word=word;}}
class NullableCaptureEffects {static String trace="";static final RuntimeException failure=new RuntimeException("original");
 static NullableCaptureBox create(int seed){trace+="A";if(seed==17)throw failure;return new NullableCaptureBox(seed);}}
class NullableCaptureOwner {
 static class View {
  java.util.function.Supplier<NullableCaptureBox> operation(int seed,boolean choose){final NullableCaptureBox value;if(choose){value=NullableCaptureEffects.create(seed);}else{value=null;}return ()->{NullableCaptureEffects.trace+="B";return value;};}
 }
 static View view(){return new View();}
}
class NullableCaptureDriver {
 public static void main(String[]args){int rows=0;NullableCaptureOwner.View owner=NullableCaptureOwner.view();
  if(!owner.getClass().isMemberClass()||owner.getClass().getDeclaringClass()!=NullableCaptureOwner.class)throw new AssertionError("member identity");
  for(int seed:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(boolean choose:new boolean[]{false,true}){
   NullableCaptureEffects.trace="";java.util.function.Supplier<NullableCaptureBox> operation=owner.operation(seed,choose);
   if(!NullableCaptureEffects.trace.equals(choose?"A":""))throw new AssertionError("eager selected producer");
   NullableCaptureEffects.trace="";NullableCaptureBox value=operation.get();if(!NullableCaptureEffects.trace.equals("B")||(choose?(value==null||value.word!=seed):value!=null))throw new AssertionError("null/value snapshot");
   if(value!=null)value.word^=0x654321ab;NullableCaptureEffects.trace="";if(operation.get()!=value||!NullableCaptureEffects.trace.equals("B")||(value!=null&&value.word!=(seed^0x654321ab)))throw new AssertionError("captured reference identity and mutable content");rows++;
  }
  NullableCaptureEffects.trace="";try{owner.operation(17,true);throw new AssertionError("missing producer failure");}catch(RuntimeException e){if(e!=NullableCaptureEffects.failure||!NullableCaptureEffects.trace.equals("A"))throw new AssertionError("original failure");rows++;}
  NullableCaptureEffects.trace="";java.util.function.Supplier<NullableCaptureBox> absent=owner.operation(17,false);if(!NullableCaptureEffects.trace.equals(""))throw new AssertionError("untaken producer");if(absent.get()!=null||!NullableCaptureEffects.trace.equals("B"))throw new AssertionError("null branch result");rows++;
  System.out.println(rows+":nullable:lambda:snapshot:identity:failure");
 }
}`

func TestAdversarialMemberLambdaNullableCaptureRetainsOriginalNullAndReference(t *testing.T) {
	testNativeIndependentFamilyFixture(t, memberLambdaNullableCaptureFixture, []string{"NullableCaptureOwner"}, "NullableCaptureDriver", "12:nullable:lambda:snapshot:identity:failure\n", nativeLexicalExactSignatures)
}
