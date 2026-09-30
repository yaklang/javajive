package javaclassparser

import "testing"

// The erased array cast removes a temporary before variable naming. Outer
// boolean reads in both nested catch arms must follow their declaration's
// identity, irrespective of numbering or a reused reference-array slot.
func TestAdversarialCatchLocalRebindRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "CatchLocalRebind", `public class CatchLocalRebind {
  String run(boolean sort,boolean enabled,int kind,boolean permit) {
    try {Object[] startup=new Object[]{"start",kind};CatchRebindOracle.start(startup);}
    catch (RuntimeException ex) {CatchRebindOracle.mark("startup-catch");}
    boolean fallback=CatchRebindOracle.flag(enabled);
    String label=CatchRebindOracle.label();
    try {
      String[] words=sort ? (String[])CatchRebindOracle.sorted(CatchRebindOracle.words()) : CatchRebindOracle.words();
      for (String word:words) {
        try {CatchRebindOracle.fail(kind);return label+word;}
        catch (IllegalArgumentException ex) {return fallback || CatchRebindOracle.allow(permit) ? "argument-default" : ex.getMessage();}
        catch (UnsupportedOperationException ex) {return fallback || CatchRebindOracle.allow(permit) ? "unsupported-default" : ex.getMessage();}
      }
      return "empty";
    } catch (IllegalStateException ex) {return fallback || CatchRebindOracle.allow(permit) ? "state-default" : ex.getMessage();}
  }
  public static void main(String[] args) {CatchRebindOracle.run();}
}
class CatchRebindOracle {
  static StringBuilder trace;
  static void mark(String value) {trace.append(value).append(';');}
  static void start(Object[] values) {mark("start:"+values.length);}
  static boolean flag(boolean enabled) {mark("flag");return enabled;}
  static String label() {mark("label");return "value:";}
  static String[] words() {mark("words");return new String[]{"a","b"};}
  static <T> T[] sorted(T[] values) {mark("sort");return values.clone();}
  static void fail(int kind) {mark("fail");if (kind==1) throw new IllegalArgumentException("argument");if (kind==2) throw new UnsupportedOperationException("unsupported");if (kind==3) throw new IllegalStateException("state");}
  static boolean allow(boolean permit) {mark("allow");return permit;}
  static void run() {
    CatchLocalRebind choice=new CatchLocalRebind();
    for (int sort=0;sort<2;sort++) for (int enabled=0;enabled<2;enabled++) for (int kind=0;kind<4;kind++) for (int permit=0;permit<2;permit++) {
      trace=new StringBuilder();
      System.out.println(sort+":"+enabled+":"+kind+":"+permit+":"+choice.run(sort==0,enabled==0,kind,permit==0)+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
