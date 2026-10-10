package javaclassparser

import "testing"

func TestAdversarialSequentialLoopsBeforeNestedDrainRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "DemandDrain", `public class DemandDrain {
  static String run(int[][] batches,int[] consumers,boolean replace) {
    int batch=0,sum=0;String trace="";
    OUTER: while(batch<batches.length) {
      int requested=4;
      for(int n:consumers)requested=Math.min(requested,n);
      int index=0;
      while(requested>0) {
        if(replace && (batch&1)==0) {batch++;trace+="R";continue OUTER;}
        int value;
        try {value=100/batches[batch][index++];}
        catch(ArithmeticException e) {trace+="A";break;}
        catch(ArrayIndexOutOfBoundsException e) {trace+="E";break;}
        if(value==0)break;
        for(int n:consumers)sum+=value*n;
        requested--;
      }
      if(requested==0)trace+="Z";
      trace+="T";batch++;
    }
    return sum+":"+batch+":"+trace;
  }
  public static void main(String[] args) {
    for(int[] consumers:new int[][]{{},{0},{1},{3,2}})
      for(boolean replace:new boolean[]{false,true})
        System.out.print(run(new int[][]{{1,2},{0,3},{200,4},{5,10}},consumers,replace)+";");
  }
}`, Precision, Compatibility, "legacy")
}
