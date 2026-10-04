package javaclassparser

import "testing"

// Both sides of the constructor merge are non-null values. The clone belongs
// exclusively to the copy branch, including its null failure and fresh identity.
func TestAdversarialConditionalCopyCloneRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ConditionalCopyClone", `public class ConditionalCopyClone {
  final double[] data;
  final String[] texts;
  final int[][] nested;
  ConditionalCopyClone(double[] values){texts=null;nested=null;data=values;}
  ConditionalCopyClone(ConditionalCopyClone other,boolean copy){texts=null;nested=null;data=copy?other.data.clone():other.data;}
  ConditionalCopyClone(String[] values,boolean copy){data=null;nested=null;texts=copy?values.clone():values;}
  ConditionalCopyClone(int[][] values,boolean copy){data=null;texts=null;nested=copy?values.clone():values;}
  static double[] select(double[] values,boolean copy){return copy?values.clone():values;}
  public static void main(String[] args){CopyCloneOracle.main(args);}
}
class CopyCloneOracle {
 static String view(double[] values){
  if(values==null)return "null";
  String out="";for(double value:values)out+=Long.toHexString(Double.doubleToRawLongBits(value))+",";return out;
 }
 public static void main(String[] args){
  for(boolean copy:new boolean[]{false,true})for(int kind=0;kind<4;kind++){
   double[] values=kind<2?null:kind==2?new double[0]:new double[]{-0.0,Double.NaN,Double.POSITIVE_INFINITY,1};
   ConditionalCopyClone input=kind==0?null:new ConditionalCopyClone(values);
   try{
    ConditionalCopyClone result=new ConditionalCopyClone(input,copy);
    String before=view(result.data);
    if(result.data!=null&&result.data.length>0)result.data[0]=2;
    System.out.println(copy+":"+kind+":ctor:"+(result.data==input.data)+":"+before+":"+view(input.data));
   }catch(NullPointerException expected){System.out.println(copy+":"+kind+":ctor:NPE");}
   try{
    double[] result=ConditionalCopyClone.select(values,copy);
    System.out.println(copy+":"+kind+":select:"+(result==values)+":"+view(result));
   }catch(NullPointerException expected){System.out.println(copy+":"+kind+":select:NPE");}
  }
  for(boolean copy:new boolean[]{false,true})for(int kind=0;kind<3;kind++){
   String[] texts=kind==0?null:kind==1?new String[0]:new String[]{"value",null};
   try {
    ConditionalCopyClone result=new ConditionalCopyClone(texts,copy);
    boolean alias=result.texts==texts;
    if(result.texts!=null&&result.texts.length>0)result.texts[0]="changed";
    System.out.println(copy+":"+kind+":texts:"+alias+":"+java.util.Arrays.toString(texts));
   }catch(NullPointerException expected){System.out.println(copy+":"+kind+":texts:NPE");}
   int[][] nested=kind==0?null:kind==1?new int[0][]:new int[][]{new int[]{1},null};
   try {
    ConditionalCopyClone result=new ConditionalCopyClone(nested,copy);
    boolean alias=result.nested==nested;
    boolean shallow=result.nested==null||result.nested.length==0||result.nested[0]==nested[0];
    if(result.nested!=null&&result.nested.length>0)result.nested[0]=new int[]{9};
    System.out.println(copy+":"+kind+":nested:"+alias+":"+shallow+":"+java.util.Arrays.deepToString(nested));
   }catch(NullPointerException expected){System.out.println(copy+":"+kind+":nested:NPE");}
  }
 }
}
`, Precision, Compatibility, "legacy")
}
