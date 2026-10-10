package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

func TestAdversarialBareIfScopeIdentityRoundTrip(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("testdata/regression/BareIfAdv.java")
	if err != nil {
		t.Fatal(err)
	}
	source := strings.TrimSuffix(strings.TrimSpace(string(raw)), "}") + `public static void main(String[] args){BareIfOracle.main(args);}
}
class BareIfOracle {
 static String trace="";
 static boolean failComma;
 static class Probe extends BareIfAdv {
  void startArray(int n){trace+="A"+n;}
  void writeComma(){trace+="C";if(failComma)throw new IllegalStateException("comma");}
 }
 static class Input extends java.util.AbstractList<Object> {
  final int count,fail;
  Input(int count,int fail){this.count=count;this.fail=fail;}
  public int size(){trace+="S";return count;}
  public Object get(int index){trace+="G"+index;if(index==fail)throw new IllegalStateException("get");return index;}
 }
 public static void main(String[] args){
  for(boolean binary:new boolean[]{false,true})for(int count:new int[]{0,1,3})for(int fail:new int[]{-1,0,2})for(boolean comma:new boolean[]{false,true}){
   trace="";failComma=comma;String outcome="ok";
   try{new Probe().write(binary,new Input(count,fail));}catch(IllegalStateException error){outcome=error.getMessage();}
   System.out.println(binary+":"+count+":"+fail+":"+comma+":"+outcome+":"+trace);
  }
 }
}
`
	roundTripGenericFlow(t, "BareIfAdv", source, "precision", "compatibility", "legacy")
}
