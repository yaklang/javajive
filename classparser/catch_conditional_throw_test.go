package javaclassparser

import "testing"

// Catch-local definitions belong to the handler even when constructor arms
// merge at one ATHROW. The independent JVM oracle checks causes, payloads,
// handler selection, constructor failures and failures before the try.
func TestAdversarialCatchConditionalThrowRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "CatchConditionalThrow", `public class CatchConditionalThrow {
  String wrap(int input,boolean first) {
    CatchThrowOracle.mark("before");
    try {return CatchThrowOracle.read(input);}
    catch (CatchThrowSpecial ex) {throw new IllegalArgumentException("special",ex);}
    catch (RuntimeException ex) {
      String label;
      switch (input) {case 1:label="one";break;case 2:label="two";break;default:label="other";break;}
      throw first ? new CatchThrowFirst(ex,new Object[]{label,ex.getMessage()}) : new CatchThrowSecond(ex,new Object[]{label,ex.getMessage()});
    }
  }
  String nested(int input,boolean first) {
    try {return wrap(input,first);}
    catch (CatchThrowFirst ex) {CatchThrowOracle.mark("outer");throw ex;}
  }
  public static void main(String[] args) {CatchThrowOracle.run();}
}
class CatchThrowSpecial extends RuntimeException {CatchThrowSpecial() {super("special");}}
class CatchThrowReadFailure extends IllegalArgumentException {
  CatchThrowReadFailure(int input) {super("input"+input);}
  public String getMessage() {CatchThrowOracle.trace.append("message;");return super.getMessage();}
}
class CatchThrowFirst extends RuntimeException {
  CatchThrowFirst(Throwable cause,Object[] data) {super(java.util.Arrays.toString(data),cause);CatchThrowOracle.mark("first");}
}
class CatchThrowSecond extends RuntimeException {
  CatchThrowSecond(Throwable cause,Object[] data) {super(java.util.Arrays.toString(data),cause);CatchThrowOracle.mark("second");}
}
class CatchThrowOracle {
  static StringBuilder trace;static int step,failAt;
  static void mark(String s) {trace.append(s).append(';');if (++step==failAt) throw new IllegalStateException(s);}
  static String read(int input) {mark("read");if (input==0) return "ok";if (input==3) throw new CatchThrowSpecial();throw new CatchThrowReadFailure(input);}
  static void run() {
    CatchConditionalThrow choice=new CatchConditionalThrow();
    for (int method=0;method<2;method++) for (int input=0;input<4;input++) for (int first=0;first<2;first++) for (failAt=0;failAt<6;failAt++) {
      trace=new StringBuilder();step=0;String outcome;
      try {outcome="value:"+(method==0 ? choice.wrap(input,first==0) : choice.nested(input,first==0));}
      catch (Throwable ex) {Throwable cause=ex.getCause();outcome=ex.getClass().getSimpleName()+":"+ex.getMessage()+":"+(cause==null ? "no-cause" : cause.getClass().getSimpleName()+":"+cause.getMessage());}
      System.out.println(method+":"+input+":"+first+":"+failAt+":"+outcome+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
