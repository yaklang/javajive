package javaclassparser

import "testing"

func TestAdversarialConditionalConstructorArrayPrefixRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ConditionalArrayPrefix", `import java.util.*;
class PrefixEvents {
  static String trace="";
  static String mark(String s,int fail,int stage) {trace+=s;if(fail==stage)throw new IllegalArgumentException(s);return s;}
  static boolean choose(boolean b) {trace+="C";return b;}
}
class PrefixParent {
  final String result;
  PrefixParent(String text,Object[] items,String tail) {
    PrefixEvents.trace+="B";result=text+":"+Arrays.toString(items)+":"+tail;
  }
}
public class ConditionalArrayPrefix extends PrefixParent {
  ConditionalArrayPrefix(boolean b,int fail) {
    this(new StringBuilder().append(PrefixEvents.mark("P",fail,0))
      .append(PrefixEvents.choose(b) ? PrefixEvents.mark("L",fail,1) : PrefixEvents.mark("R",fail,1))
      .append(PrefixEvents.mark("S",fail,2)).toString(),
      new Object[]{PrefixEvents.mark("A",fail,3),PrefixEvents.mark("D",fail,4)},
      PrefixEvents.mark("Z",fail,5));
  }
  ConditionalArrayPrefix(int fail,boolean b) {
    super(new StringBuilder().append(PrefixEvents.mark("P",fail,0))
      .append(PrefixEvents.choose(b) ? PrefixEvents.mark("L",fail,1) : PrefixEvents.mark("R",fail,1))
      .append(PrefixEvents.mark("S",fail,2)).toString(),
      new Object[]{PrefixEvents.mark("A",fail,3),PrefixEvents.mark("D",fail,4)},
      PrefixEvents.mark("Z",fail,5));
  }
  private ConditionalArrayPrefix(String s,Object[] a,String z) {super(s,a,z);}
  public static void main(String[] args) {
    for(boolean b:new boolean[]{false,true})for(int fail=-1;fail<6;fail++)for(boolean direct:new boolean[]{false,true}) {
      PrefixEvents.trace="";String result;
      try {result=(direct ? new ConditionalArrayPrefix(fail,b) : new ConditionalArrayPrefix(b,fail)).result;}
      catch(IllegalArgumentException e) {result="error:"+e.getMessage();}
      System.out.print(PrefixEvents.trace+":"+result+";");
    }
  }
}`, Precision, Compatibility, "legacy")
}
