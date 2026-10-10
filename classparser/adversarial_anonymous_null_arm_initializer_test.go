package javaclassparser

import (
	"strings"
	"testing"
)

const nativeAnonymousNullArmInitializerFixture = `interface NullArmRead {char[] get();int stamp();}
class NullArmEffects {
 static String trace="";static final RuntimeException error=new RuntimeException("original");
 static String inspect(String value){trace+="P";if("boom".equals(value))throw error;return value;}
 static int finish(){trace+="D";return 7;}
}
class NullArmOwner {
 private String text;NullArmOwner(String value){text=value;}
 void change(){text="changed after allocation";}
 NullArmRead make(){return new NullArmRead(){
  final char[] characters=NullArmEffects.inspect(text)==null?null:text.toCharArray();
  final int completed=NullArmEffects.finish();
  public char[] get(){return characters;}public int stamp(){return completed;}
 };}
}
class NullArmDriver {public static void main(String[]args){int rows=0;
 for(String text:new String[]{null,"","x","\u0000","\ud800\udfff","boom"}){
  NullArmEffects.trace="";NullArmOwner owner=new NullArmOwner(text);
  try{
   NullArmRead value=owner.make();owner.change();
   if("boom".equals(text)||!value.getClass().isAnonymousClass())throw new AssertionError("null arm original ownership/abrupt completion");
   char[] expected=text==null?null:text.toCharArray();
   if(!java.util.Arrays.equals(value.get(),expected)||value.get()!=value.get()||value.stamp()!=7||!NullArmEffects.trace.equals("PD"))throw new AssertionError("null arm selected value/effect ordering/capture snapshot");
  }catch(RuntimeException e){if(!"boom".equals(text)||e!=NullArmEffects.error||!NullArmEffects.trace.equals("P"))throw new AssertionError("null arm original exception identity/phase",e);}
  rows++;
 }System.out.println(rows+":initializer:null:arm");
}}`

func TestAdversarialAnonymousNullArmInitializerKeepsSelectedArm(t *testing.T) {
	for _, rename := range []string{"original", "renamed"} {
		t.Run(rename, func(t *testing.T) {
			for _, shape := range []string{"null-first", "null-second"} {
				t.Run(shape, func(t *testing.T) {
					source, owner := nativeAnonymousNullArmInitializerFixture, "NullArmOwner"
					if shape == "null-second" {
						source = strings.Replace(source, "NullArmEffects.inspect(text)==null?null:text.toCharArray()", "NullArmEffects.inspect(text)!=null?text.toCharArray():null", 1)
					}
					if rename == "renamed" {
						replace := strings.NewReplacer("NullArmOwner", "NullableEnvelope", "text", "payload")
						source, owner = replace.Replace(source), replace.Replace(owner)
					}
					testNativePrivateSetterFixture(t, source, owner, "NullArmDriver", "6:initializer:null:arm\n")
				})
			}
		})
	}
}
