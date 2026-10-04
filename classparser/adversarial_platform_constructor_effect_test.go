package javaclassparser

import "testing"

const platformConstructorEffectFixture = `
class PlatformEffectsOwner {
 java.util.AbstractList<Object> list(final Object token){return new java.util.AbstractList<Object>(){public Object get(int i){if(i==0)return token;throw new IndexOutOfBoundsException();}public int size(){return 1;}};}
 java.io.InputStream stream(final int value){return new java.io.InputStream(){public int read(){return value;}};}
}
class PlatformEffectsOracle {
 static void run()throws Exception{PlatformEffectsOwner owner=new PlatformEffectsOwner();Object token=new Object();
  for(Object value:new Object[]{null,token}){
   java.util.AbstractList<Object> result=owner.list(value);
   if(result.get(0)!=value||result.iterator().next()!=value||result.size()!=1)throw new AssertionError("platform capture identity");
   System.out.println("list:"+(value==token));
  }
  for(int value:new int[]{-1,0,1,127,255}){java.io.InputStream result=owner.stream(value);if(result.read()!=value)throw new AssertionError("primitive capture");System.out.println("stream:"+value);}
 }
}
public class PlatformEffectsDriver {public static void main(String[]args)throws Exception{PlatformEffectsOracle.run();}}
`

func TestAdversarialPlatformConstructorEffectsNeedOriginalCode(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "PlatformEffectsDriver", platformConstructorEffectFixture, nil,
		[]string{"PlatformEffectsOwner", "PlatformEffectsOwner$1", "PlatformEffectsOwner$2"}, true, Precision, Compatibility, "legacy")
}
