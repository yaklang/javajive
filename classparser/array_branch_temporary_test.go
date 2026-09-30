package javaclassparser

import "testing"

// Distinct array initializer temporaries cannot borrow a later cursor declaration.
func TestAdversarialArrayBranchTemporaryRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ArrayBranchTemporary", `public class ArrayBranchTemporary {
  static String trace="";
  static void count(long[] array,int add){trace+="A";array[0]+=add;}
  static void count(BranchMarker marker,long[] array,int add){trace+="B";array[0]+=marker.size+add;}
  static BranchCursor empty(){trace+="E";return new BranchCursor(0);}
  static long select(boolean dense,int size,int add) {
    long[] values;
    if(dense) {
      values=new long[]{size};
      count(values,add);
    } else {
      values=new long[]{0};
      BranchMarker marker=new BranchMarker(size);
      count(marker,values,add);
      trace+="S"+marker.size;
    }
    if(values[0]<0 && size>0)throw new IllegalStateException("negative");
    else {
      BranchCursor cursor=values[0]==0 ? empty() : new BranchCursor(values[0]);
      return cursor.value+cursor.value;
    }
  }
  public static void main(String[] args){ArrayBranchOracle.main(args);}
}
class ArrayBranchOracle {
  public static void main(String[] args) {
    for(boolean dense:new boolean[]{false,true})for(int size:new int[]{-1,0,1,4})for(int add:new int[]{-3,-1,0,2}) {
      ArrayBranchTemporary.trace="";
      String outcome;
      try {outcome="value="+ArrayBranchTemporary.select(dense,size,add);}
      catch(IllegalStateException error){outcome="error="+error.getMessage();}
      System.out.println(dense+":"+size+":"+add+":"+outcome+":"+ArrayBranchTemporary.trace);
    }
  }
}

class BranchMarker { final int size; BranchMarker(int value){size=value;ArrayBranchTemporary.trace+="M";} }
class BranchCursor { final long value; BranchCursor(long value){this.value=value;ArrayBranchTemporary.trace+="C";} }
`, Precision, Compatibility, "legacy")
}
