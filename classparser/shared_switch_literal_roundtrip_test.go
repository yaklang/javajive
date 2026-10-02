package javaclassparser

import "testing"

func TestAdversarialSiblingSwitchSharedLiteralDefaultRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "SiblingSwitchLiteral", `public class SiblingSwitchLiteral {
 static String trace;
 static boolean equal(Object a,Object b){trace+="eq;";return a.equals(b);}
 static boolean contains(int size,Object needle,Object one,Object two,Object three){
  if(needle==null){
   switch(size){case 3:if(three==null)return true;case 2:if(two==null)return true;case 1:if(one==null)return true;}
  }else{
   switch(size){case 3:if(equal(needle,three))return true;case 2:if(equal(needle,two))return true;case 1:if(equal(needle,one))return true;}
  }
  return false;
 }
 public static void main(String[] args){
  for(int size=-1;size<=4;size++)for(Object needle:new Object[]{null,"a","z"})for(Object one:new Object[]{null,"a"})for(Object two:new Object[]{null,"a"})for(Object three:new Object[]{null,"a"}){
   trace="";System.out.println(size+":"+needle+":"+one+":"+two+":"+three+":"+contains(size,needle,one,two,three)+":"+trace);
  }
 }
}`, Precision, Compatibility, "legacy")
}
