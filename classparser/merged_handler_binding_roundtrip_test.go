package javaclassparser

import "testing"

// javac's resource cleanup emits distinct catch-entry definitions for the
// primary failure and for cleanup/rethrow. Folding them must retain the thrown
// object's identity and the reverse close/addSuppressed order, not just compile.
func TestAdversarialMergedHandlerBindingRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ResourceBinding", `public class ResourceBinding {
  static BindingResult run(int seed,int body,int firstClose,int secondClose,boolean failSecondInit) throws Exception {
    try (BindingResource first=new BindingResource("first",firstClose,false);
         BindingResource second=new BindingResource("second",secondClose,failSecondInit)) {
      BindingOracle.mark("body");
      BindingOracle.fail("body",body);
      return new BindingResult(seed*31+7);
    }
  }
  public static void main(String[] args){BindingOracle.run();}
}
class BindingResult {final int value;BindingResult(int value){this.value=value;}}
class BindingResource implements AutoCloseable {
  final String name;final int fault;
  BindingResource(String name,int fault,boolean fail) {
    this.name=name;this.fault=fault;BindingOracle.mark("new:"+name);
    if(fail)throw new IllegalArgumentException("init:"+name);
  }
  public void close() throws Exception {BindingOracle.mark("close:"+name);BindingOracle.fail(name,fault);}
}
class BindingOracle {
  static StringBuilder trace;
  static void mark(String value){trace.append(value).append(';');}
  static void fail(String name,int kind) throws Exception {
    if(kind==1)throw new Exception(name);
    if(kind==2)throw new LinkageError(name);
  }
  static String describe(Throwable error) {
    String text=error.getClass().getSimpleName()+":"+error.getMessage();
    for(Throwable suppressed:error.getSuppressed())text+="["+describe(suppressed)+"]";
    return text;
  }
  static void run() {
    for(int seed:new int[]{Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE})for(int body=0;body<3;body++)for(int first=0;first<3;first++)for(int second=0;second<3;second++)for(int init=0;init<2;init++) {
      trace=new StringBuilder();String result;
      try{result="value:"+ResourceBinding.run(seed,body,first,second,init!=0).value;}
      catch(Throwable error){result=describe(error);}
      System.out.println(seed+":"+body+":"+first+":"+second+":"+init+":"+result+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
