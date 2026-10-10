package javaclassparser

import "testing"

func TestAdversarialSharedLiteralReturnAfterAssignedGuardRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "SharedLiteralReturn", `public class SharedLiteralReturn {
 static String run(String text){int n=0;if(text==null || (n=text.length())==0)return "";return n==1?text.toUpperCase():"long:"+n;}
 public static void main(String[] args){for(String text:new String[]{null,"","a","bc","xyz"})System.out.println(String.valueOf(text)+":"+String.valueOf(run(text)));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialCompoundBooleanCallArgumentRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "CompoundBooleanCall", `import java.util.*;
public class CompoundBooleanCall {
 static String trace;
 static void emit(String token,boolean force){trace+=token+":"+force+";";}
 static String run(int previous,int current,boolean branching){trace="";
  for(String rule:Arrays.asList("left","right")){
   if(rule.length()>0){for(String branch:Arrays.asList("x","y"))for(String token:new String[]{"p","q"}){
    emit(branch+token,(previous==109 && current==110)||(previous==110 && current==109));
    if(!branching)break;
   }break;}
  }return trace;
 }
 public static void main(String[] args){for(int a:new int[]{0,109,110,111})for(int b:new int[]{0,109,110,111})for(boolean branching:new boolean[]{false,true})System.out.println(a+":"+b+":"+branching+":"+run(a,b,branching));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialLoopCompoundBooleanEffectsAndThrowsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "LoopBooleanEffects", `public class LoopBooleanEffects {
 static String trace;
 static int mask;
 static boolean test(int bit){trace+="T"+bit+";";if((mask&(bit*16))!=0)throw new IllegalStateException("test"+bit);return (mask&bit)!=0;}
 static void emit(boolean value){trace+="E:"+value+";";}
 static String run(int flags){trace="";mask=flags;
  try{for(int i=0;i<2;i++){boolean value=(test(1)&&test(2))||(test(4)&&test(8));emit(value);}}
  catch(IllegalStateException e){trace+="X:"+e.getMessage();}return trace;
 }
 public static void main(String[] args){for(int flags=0;flags<256;flags++)System.out.println(flags+":"+run(flags));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialSharedPrimitiveAndNullReturnsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "SharedPrimitiveReturns", `public class SharedPrimitiveReturns {
 static int trace;
 static boolean check(int n){trace=trace*10+n;return n==0;}
 static int run(String text){int n=0;if(text==null || (n=text.length())==0)return -7;return n;}
 static String nullable(boolean outer,int n){if(outer || check(n))return null;return "value";}
 public static void main(String[] args){for(String s:new String[]{null,"","a","abcd"})System.out.println(run(s));for(boolean b:new boolean[]{false,true})for(int n=0;n<3;n++){trace=0;System.out.println(b+":"+n+":"+nullable(b,n)+":"+trace);}}
}`, Precision, Compatibility, "legacy")
}
