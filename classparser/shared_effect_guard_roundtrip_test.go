package javaclassparser

import "testing"

// The effect is reached both directly and after two short-circuit checks.
// Array construction can prevent condition folding, but cannot give the shared
// effect exclusively to the direct arm or omit it from the checked arm.
func TestAdversarialSharedEffectAfterArrayGuardsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "SharedEffectAfterGuards", `public class SharedEffectAfterGuards {
 static String trace;
 static int mask;
 static boolean check(int bit,String... labels){trace+="C"+bit+":"+labels.length+";";return (mask&bit)!=0;}
 static void emit(){trace+="E;";}
 static String run(boolean edge,int flags){trace="";mask=flags;
  if (!(edge && (check(1,"red","blue") || check(2,"green","amber")))) emit();
  trace+="Z;";return trace;
 }
 public static void main(String[] args){for(boolean edge:new boolean[]{false,true})for(int flags=0;flags<8;flags++)System.out.println(edge+":"+flags+":"+run(edge,flags));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialProtectedSharedEffectAfterGuardsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ProtectedSharedEffect", `public class ProtectedSharedEffect {
 static String trace;
 static int mask;
 static boolean check(int bit,String... labels){trace+="C"+bit+":"+labels.length+";";if((mask&(bit*4))!=0)throw new IllegalArgumentException("check"+bit);return (mask&bit)!=0;}
 static void emit(){trace+="E;";if((mask&16)!=0)throw new IllegalArgumentException("emit");}
 static String run(boolean edge,int flags){trace="";mask=flags;
  try{if (!(edge && (check(1,"one","two") || check(2,"three","four")))) emit();trace+="Z;";}
  catch(IllegalArgumentException e){trace+="T:"+e.getMessage()+";";}
  return trace;
 }
 public static void main(String[] args){for(boolean edge:new boolean[]{false,true})for(int flags=0;flags<32;flags++)System.out.println(edge+":"+flags+":"+run(edge,flags));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialSharedConstantReturnAfterArrayGuardsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "SharedConstantAfterGuards", `public class SharedConstantAfterGuards {
 static String trace;
 static int mask;
 static boolean check(int bit,String... labels){trace+="C"+bit+":"+labels.length+";";return (mask&bit)!=0;}
 static boolean result(boolean edge){
  if(edge && check(1,"alpha","beta","gamma"))return true;
  if((check(2,"delta","epsilon") || check(4,"zeta","eta")) && check(8,"theta"))return true;
  return false;
 }
 public static void main(String[] args){for(boolean edge:new boolean[]{false,true})for(int flags=0;flags<16;flags++){trace="";mask=flags;System.out.println(edge+":"+flags+":"+result(edge)+":"+trace);}}
}`, Precision, Compatibility, "legacy")
}

// These initializer calls cannot be moved by the literal-array normalization.
// The shared invocation must still survive in the original conditional path.
func TestAdversarialSharedEffectAfterEffectfulArrayGuardsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "SharedEffectAfterEffectfulGuards", `public class SharedEffectAfterEffectfulGuards {
 static String trace;
 static int mask;
 static Object label(int n){trace+="L"+n+";";if((mask&(n*4))!=0)throw new IllegalArgumentException("label"+n);return null;}
 static boolean check(int bit,Object... labels){trace+="C"+bit+":"+labels.length+";";return (mask&bit)!=0;}
 static void emit(){trace+="E;";if((mask&16)!=0)throw new IllegalArgumentException("emit");}
 static String run(boolean edge,int flags){trace="";mask=flags;
  try{if (!(edge && (check(1,"red",label(1)) || check(2,"green",label(2))))) emit();trace+="Z;";}
  catch(IllegalArgumentException e){trace+="T:"+e.getMessage()+";";}
  return trace;
 }
 public static void main(String[] args){for(boolean edge:new boolean[]{false,true})for(int flags=0;flags<32;flags++)System.out.println(edge+":"+flags+":"+run(edge,flags));}
}`, Precision, Compatibility, "legacy")
}

func TestAdversarialEarlierOperandPrimitiveGuardArraysRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "EarlierOperandPrimitiveGuards", `public class EarlierOperandPrimitiveGuards {
 static String trace;
 static int mask;
 static int earlier(int bit){trace+="A"+bit+";";if((mask&(bit*16))!=0)throw new IllegalArgumentException("arg"+bit);return bit;}
 static boolean check(int bit,int witness,int... labels){trace+="C"+bit+":"+witness+":"+labels.length+";";return (mask&bit)!=0;}
 static boolean result(boolean edge){
  if(edge && check(8,earlier(8),10,20,30))return true;
  if((check(1,earlier(1),40,50) || check(2,earlier(2),60,70)) && check(4,earlier(4),80))return true;
  return false;
 }
 static String run(boolean edge,int flags){trace="";mask=flags;try{return result(edge)+":"+trace;}catch(Throwable e){return e.getClass().getName()+":"+e.getMessage()+":"+trace;}}
 public static void main(String[] args){for(boolean edge:new boolean[]{false,true})for(int flags=0;flags<160;flags++)System.out.println(edge+":"+flags+":"+run(edge,flags));}
}`, Precision, Compatibility, "legacy")
}
