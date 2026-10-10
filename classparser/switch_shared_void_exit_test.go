package javaclassparser

import "testing"

// Each constructor arm assigns one blank final field and then jumps to the
// shared RETURN, also reached by the null arm before the switch. Lost case
// exits must not turn that into multiple assignments or real fall-through.
func TestAdversarialSwitchSharedVoidExitRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "SwitchSharedVoidExit", `import java.util.function.Predicate;
public class SwitchSharedVoidExit {
  final Predicate<String> validator;
  SwitchSharedVoidExit(String format) {
    if (format==null) {validator=null;}
    else {
      switch (format) {
        case "Aa": validator=SwitchVoidOracle.first();break;
        case "BB": validator=SwitchVoidOracle.second();break;
        case "FB": validator=SwitchVoidOracle.first();break;
        case "Ea": validator=SwitchVoidOracle.second();break;
        case "large": validator=SwitchVoidOracle.second();break;
        default: validator=null;break;
      }
    }
  }
  void sparse(int key) {
    if (key==0) {SwitchVoidOracle.mark("early");return;}
    switch (key) {
      case Integer.MIN_VALUE: SwitchVoidOracle.mark("min");break;
      case 1: SwitchVoidOracle.mark("one");
      case 2: SwitchVoidOracle.mark("two");break;
      case Integer.MAX_VALUE: SwitchVoidOracle.mark("max");break;
      default: SwitchVoidOracle.mark("other");break;
    }
  }
  void table(int key) {
    if (key==0) {SwitchVoidOracle.mark("early");return;}
    switch (key) {
      case 1: SwitchVoidOracle.mark("one");break;
      case 2: case 3: SwitchVoidOracle.mark("group");break;
      case 4: SwitchVoidOracle.mark("four");return;
      default: SwitchVoidOracle.mark("other");break;
    }
  }
  void cleanup(int key) {
    try {
      if (key==0) {SwitchVoidOracle.mark("early");return;}
      switch (key) {
        case 1: SwitchVoidOracle.mark("one");return;
        case 2: case 3: SwitchVoidOracle.mark("group");break;
        default: SwitchVoidOracle.mark("other");break;
      }
    } finally {SwitchVoidOracle.trace.append("cleanup;");}
  }
  public static void main(String[] args) {SwitchVoidOracle.run();}
}
class SwitchVoidOracle {
  static StringBuilder trace;static int failAt,step;
  static void mark(String value) {trace.append(value).append(';');if (++step==failAt) throw new IllegalStateException(value);}
  static Predicate<String> first() {mark("first");return value -> value!=null && value.startsWith("a");}
  static Predicate<String> second() {mark("second");return value -> value!=null && value.endsWith("b");}
  static void run() {
    String[] formats={null,"Aa","BB","FB","Ea","large","unknown","aa"};
    String[] values={null,"a","b","ab","z"};
    int[] keys={Integer.MIN_VALUE,-1,0,1,2,3,4,5,Integer.MAX_VALUE};
    for (int format=0;format<formats.length;format++) for (int input=0;input<values.length;input++) for (failAt=0;failAt<3;failAt++) {
      trace=new StringBuilder();step=0;String out;
      try {SwitchSharedVoidExit value=new SwitchSharedVoidExit(formats[format]);out=value.validator==null ? "none" : "test:"+value.validator.test(values[input]);}
      catch (Throwable ex) {out=ex.getClass().getSimpleName()+":"+ex.getMessage();}
      System.out.println("ctor:"+format+":"+input+":"+failAt+":"+out+":"+trace);
    }
    SwitchSharedVoidExit value=new SwitchSharedVoidExit(null);
    for (int method=0;method<3;method++) for (int key:keys) for (failAt=0;failAt<4;failAt++) {
      trace=new StringBuilder();step=0;String out="ok";
      try {if (method==0) value.sparse(key);else if (method==1) value.table(key);else value.cleanup(key);}
      catch (Throwable ex) {out=ex.getClass().getSimpleName()+":"+ex.getMessage();}
      System.out.println("method:"+method+":"+key+":"+failAt+":"+out+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
