package javaclassparser

import "testing"

// Array length belongs to the selected arm's operand prefix. Losing it as an
// opaque value must not discard the copy/cast producer or move a null check
// across a guard, an observable getter, or an exception handler.
func TestAdversarialGuardedArrayCopyRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "GuardedArrayCopy", `import java.util.Arrays;
public class GuardedArrayCopy {
  static final CopyToken[] EMPTY=new CopyToken[0];
  static final CopyToken[][] EMPTY_MATRIX=new CopyToken[0][];
  static CopyToken[] copy(CopyToken[] values){return values==null||values.length==0 ? EMPTY : Arrays.copyOf(values,values.length);}
  static CopyToken[][] matrix(CopyToken[][] values){return values==null||values.length==0 ? EMPTY_MATRIX : Arrays.copyOf(values,values.length);}
  static CopyToken[] evaluated(CopyToken[] values){return CopyOracle.get(values)==null||CopyOracle.get(values).length==0 ? EMPTY : Arrays.copyOf(CopyOracle.get(values),CopyOracle.get(values).length);}
  static CopyToken[] protectedCopy(CopyToken[] values){try{return values==null||values.length==0 ? EMPTY : Arrays.copyOf(CopyOracle.get(values),CopyOracle.get(values).length);}catch(IllegalStateException e){CopyOracle.mark("handler");return EMPTY;}}
  public static void main(String[] args){CopyOracle.run();}
}
class CopyToken { final int value;CopyToken(int v){value=v;} }
class CopyOracle {
  static StringBuilder trace;static int step,failAt;
  static void mark(String s){trace.append(s).append(';');if(++step==failAt)throw new IllegalStateException(s);}
  static CopyToken[] get(CopyToken[] values){mark("get");return values;}
  static void run(){
    for(int length:new int[]{-1,0,1,4})for(int kind=0;kind<4;kind++)for(failAt=0;failAt<8;failAt++){
      trace=new StringBuilder();step=0;CopyToken[] input=length<0 ? null : new CopyToken[length];
      if(input!=null)for(int n=0;n<length;n++)input[n]=(n&1)==0 ? null : new CopyToken(n);
      String result;
      try{
        if(kind==3){
          CopyToken[][] a=input==null ? null : new CopyToken[input.length][];if(a!=null)for(int n=0;n<a.length;n++)a[n]=input;
          CopyToken[][] b=GuardedArrayCopy.matrix(a);
          result=b.length+":"+(b==a)+":"+(b==GuardedArrayCopy.EMPTY_MATRIX)+":"+(b.length==0||b[0]==input);
          if(b.length>0){b[0]=null;result+=":"+(a[0]==input);}
        }else{
          CopyToken[] output=kind==0 ? GuardedArrayCopy.copy(input) : kind==1 ? GuardedArrayCopy.evaluated(input) : GuardedArrayCopy.protectedCopy(input);
          result=output.length+":"+(output==input)+":"+(output==GuardedArrayCopy.EMPTY)+":"+Arrays.toString(output.length==0 ? new int[0] : new int[]{output[output.length-1]==null ? -1 : output[output.length-1].value});
          if(output.length>0){output[0]=new CopyToken(99);result+=":"+(input[0]==null);}
        }
      }catch(Throwable e){result=e.getClass().getSimpleName()+":"+e.getMessage();}
      System.out.println(length+":"+kind+":"+failAt+":"+result+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
