package javaclassparser

import "testing"

// Field loads and argument-producing calls on one conditional arm are already
// evaluated bytecode operands, not uninitialized locals. Compare the original
// JVM's values, failures and trace, including casts before the branch and a
// receiver/argument evaluated after it, across all modes and debug variants.
func TestAdversarialTernaryFieldCallCastsRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "FieldCallChoice", `import java.util.*;
public class FieldCallChoice {
  volatile Object member;
  Map table;
  FieldCallChoice(Object value,Map table) {this.member=value;this.table=table;}
  Object field(boolean on) {
    List value=on ? (List)member : null;
    FieldCallOracle.mark("after");return value;
  }
  Object guarded(boolean on) {
    List value=member instanceof List ? (List)member : FieldCallOracle.fallback();
    FieldCallOracle.mark("after");return value;
  }
  Object call(boolean on) {
    String value=on ? (String)table.get(FieldCallOracle.key()) : null;
    FieldCallOracle.mark("after");return value;
  }
  Object nullable(boolean on) {
    String value=table!=null ? (String)table.get(FieldCallOracle.key()) : null;
    FieldCallOracle.mark("after");return value;
  }
  Object earlier(boolean on) {
    String before=(String)member;
    String value=on ? before : null;
    FieldCallOracle.mark("after");return value;
  }
  Object caught(boolean on) {
    try {
      String value=on ? (String)table.get(FieldCallOracle.key()) : null;
      FieldCallOracle.mark("after");return value;
    } catch (ClassCastException ex) {FieldCallOracle.mark("caught");return "caught";}
  }
  Object argument(boolean on) {
    return FieldCallOracle.finish(on ? (String)table.get(FieldCallOracle.key()) : null,FieldCallOracle.later());
  }
  Object initialized(boolean on,int which) {
    String value=on ? (which==0 ? (String)FieldCallStaticOk.VALUE : which==1 ? (String)FieldCallStaticFail.VALUE : (String)FieldCallStaticWrong.VALUE) : null;
    FieldCallOracle.mark("after");return value;
  }
  Object named(boolean on,Class key) {
    String value=on ? (String)table.get(key.getName()) : null;
    FieldCallOracle.mark("after");return value;
  }
  public static void main(String[] args) {FieldCallOracle.run();}
}
class FieldCallOracle {
  static StringBuilder trace;
  static int step,failAt;
  static void mark(String event) {trace.append(event).append(';');if (++step==failAt) throw new IllegalStateException(event);}
  static String key() {mark("key");return "k";}
  static String later() {mark("later");return "late";}
  static String finish(String value,String later) {mark("finish");return value+":"+later;}
  static List fallback() {mark("fallback");return Collections.singletonList("fallback");}
  static void run() {
    for (int variant=0;variant<9;variant++) for (int on=0;on<2;on++)
      for (int payload=0;payload<4;payload++) for (failAt=0;failAt<6;failAt++) {
        final Object mapped=payload==0 ? "ok" : payload==1 ? null : Integer.valueOf(9);
        Map map=payload==3 ? null : new HashMap() {
          public Object get(Object key) {mark("get");return mapped;}
        };
        Object member=variant<2 ? (payload==0 ? Collections.singletonList("item") : payload==1 ? null : "wrong") : mapped;
        FieldCallChoice choice=new FieldCallChoice(member,map);
        trace=new StringBuilder();step=0;
        String outcome;
        try {
          Object value=variant==0 ? choice.field(on==0) : variant==1 ? choice.guarded(on==0) : variant==2 ? choice.call(on==0) : variant==3 ? choice.nullable(on==0) : variant==4 ? choice.earlier(on==0) : variant==5 ? choice.caught(on==0) : variant==6 ? choice.argument(on==0) : variant==7 ? choice.initialized(on==0,payload%3) : choice.named(on==0,payload==1 ? null : String.class);
          outcome="value:"+value;
        } catch (Throwable ex) {outcome=ex.getClass().getSimpleName();}
        System.out.println(variant+":"+on+":"+payload+":"+failAt+":"+outcome+":"+trace);
      }
  }
}
class FieldCallStaticOk {
  static final Object VALUE=init();
  static Object init() {FieldCallOracle.mark("init-ok");return "static";}
}
class FieldCallStaticFail {
  static final Object VALUE=init();
  static Object init() {FieldCallOracle.mark("init-fail");throw new IllegalArgumentException("init");}
}
class FieldCallStaticWrong {
  static final Object VALUE=init();
  static Object init() {FieldCallOracle.mark("init-wrong");return Integer.valueOf(5);}
}`, Precision, Compatibility, DecompileMode("legacy"))
}
