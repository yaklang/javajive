package javaclassparser

import (
	"strings"
	"testing"
)

// A class variable may describe throws rather than an input. This authored
// runtime fixture keeps payloads, evaluation order and exception identities
// independent of the interface spelling. The values-level test separately
// rejects name-based receiver erasure when declaration evidence is absent.
func TestAdversarialWildcardReceiverPreservesIndependentThrowsContract(t *testing.T) {
	const fixture = `
interface Transformer<E extends RuntimeException> { Object transform(Object value) throws E; }
class ContractEffects {
 static String trace="";static int mode;static Object payload;
 static final IllegalStateException checked=new IllegalStateException("same");
 static final RuntimeException unchecked=new IllegalArgumentException("same");
 static final Transformer<IllegalStateException> target=new Transformer<IllegalStateException>(){
  public Object transform(Object value)throws IllegalStateException {
   trace+="C;";if(value!=payload)throw new AssertionError("argument identity");
   if(mode==1)throw checked;if(mode==2)throw unchecked;return value;
  }
 };
 static Transformer<? extends IllegalStateException> receiver(){trace+="R;";return mode==3?null:target;}
 static Object argument(){trace+="A;";return payload;}
}
public class WildcardContractOwner {
 static Object run(Transformer<? extends IllegalStateException> receiver,Object value)throws IllegalStateException{return receiver.transform(value);}
}
class ContractDriver {public static void main(String[]args){int rows=0;
 for(Object payload:new Object[]{null,"text",Integer.valueOf(17),new Object()})for(int mode=0;mode<4;mode++){
  ContractEffects.trace="";ContractEffects.mode=mode;ContractEffects.payload=payload;
  Object result=null;Throwable failure=null;try{result=WildcardContractOwner.run(ContractEffects.receiver(),ContractEffects.argument());}catch(Throwable caught){failure=caught;}
  String expected=mode==3?"R;A;":"R;A;C;";
  if(!expected.equals(ContractEffects.trace))throw new AssertionError("evaluation order "+ContractEffects.trace);
  if(mode==0?failure!=null||result!=payload:mode==1?failure!=ContractEffects.checked:mode==2?failure!=ContractEffects.unchecked:failure==null||failure.getClass()!=NullPointerException.class)throw new AssertionError("result or exception identity",failure);
  rows++;
 }System.out.println(rows+":wildcard:throws:effects:identity");}}
`
	for _, spelling := range []string{"Transformer", "Callback"} {
		t.Run(spelling, func(t *testing.T) {
			source := strings.ReplaceAll(fixture, "Transformer", spelling)
			testNativePrivateSetterCompiledFixtureWithPolicies(t, "WildcardContractOwner", "ContractDriver", "16:wildcard:throws:effects:identity\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{"WildcardContractOwner.java": source}, debug, "8")
			}, nil, []string{"normal", "no-source-rewrites"})
		})
	}
}
