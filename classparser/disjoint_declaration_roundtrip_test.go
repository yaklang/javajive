package javaclassparser

import "testing"

func TestAdversarialDisjointDeclarationRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "DisjointDeclaration", `import java.nio.*;
import java.nio.file.*;
import java.util.function.*;
public class DisjointDeclaration {
  static long decode(boolean wide,long input,int tail,String name) {
    long result;
    if(wide) {
      DeclarationOracle.mark("wide");
      ByteBuffer buffer=ByteBuffer.allocate(4);
      buffer.order(ByteOrder.LITTLE_ENDIAN);buffer.putInt((int)input);buffer.flip();
      result=buffer.getInt()&0xffffffffL;
    } else {
      DeclarationOracle.mark("short");
      ByteBuffer buffer=ByteBuffer.allocate(2);
      buffer.order(ByteOrder.LITTLE_ENDIAN);buffer.putShort((short)input);buffer.flip();
      result=(buffer.getShort()&0xffffL)+1L;
    }
    if(tail==0)return result;
    DeclarationOracle.mark("path");
    Path parent=Paths.get(name).normalize().getParent();
    Supplier<String> path=()->{DeclarationOracle.mark("lambda");return parent==null?"":parent.toString();};
    String text=path.get();
    if(tail==2)text=text+path.get();
    DeclarationOracle.mark("finish");
    return result+text.length();
  }
  public static void main(String[] args){DeclarationOracle.run();}
}
class DeclarationOracle {
  static StringBuilder trace;static int step,failAt;
  static void mark(String text){trace.append(text).append(';');if(++step==failAt)throw new IllegalStateException(text);}
  static void run(){
    long[] inputs={Long.MIN_VALUE,-65537,-1,0,32767,65535,65536,Integer.MAX_VALUE,Long.MAX_VALUE};
    String[] names={null,"","a/b","a/../b/c"};
    for(int wide=0;wide<2;wide++)for(int i=0;i<inputs.length;i++)for(int tail=0;tail<3;tail++)for(int n=0;n<names.length;n++)for(failAt=0;failAt<5;failAt++) {
      trace=new StringBuilder();step=0;String outcome;
      try {
        long result=DisjointDeclaration.decode(wide!=0,inputs[i],tail,names[n]);
        if(failAt==0) {
          long expected=wide!=0?(inputs[i]&0xffffffffL):(inputs[i]&0xffffL)+1;
          if(tail!=0) {
            Path parent=Paths.get(names[n]).normalize().getParent();
            int length=parent==null?0:parent.toString().length();expected+=length*(tail==2?2:1);
          }
          if(result!=expected)throw new AssertionError("numeric decoding");
        }
        outcome="ok:"+result;
      }catch(Throwable error){outcome=error.getClass().getSimpleName();}
      System.out.println(wide+":"+i+":"+tail+":"+n+":"+failAt+":"+outcome+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
