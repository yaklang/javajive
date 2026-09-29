package javaclassparser

import "testing"

func TestAdversarialNestedCatchNumericFallbackRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "NumericFallback", `public class NumericFallback {
  static int effects;
  static long read(String text,int mode) {
    if(mode>0) {
      if(mode==1 || text.length()<4) {
        try { long n=Long.parseLong(text); effects++; return n; }
        catch(NumberFormatException ignored) {}
      }
    }
    effects+=10;
    double decimal=Double.parseDouble(text);
    long result=(long)decimal;
    if(result!=decimal) throw new NumberFormatException("not integral");
    return result;
  }
  static int wrapped(String text) {
    try { return Integer.parseInt(text); }
    catch(NumberFormatException e) { throw new IllegalArgumentException("wrapped",e); }
  }
  public static void main(String[] args) {
    for(String text:new String[]{"0","-3","1.0","1e3","1.25","9223372036854775808","NaN","bad"}) {
      for(int mode=0;mode<3;mode++) {
        try { System.out.print(read(text,mode)+":"); }
        catch(NumberFormatException e) { System.out.print(e.getClass().getSimpleName()+":"); }
        System.out.print(effects+";");
      }
    }
    try { wrapped("bad"); }
    catch(IllegalArgumentException e) { System.out.print(e.getMessage()+":"+e.getCause().getClass().getSimpleName()); }
  }
}`, Precision, Compatibility, "legacy")
}
