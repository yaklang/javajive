package javaclassparser

import "testing"

// A handler argument is a definition at handler entry, not a fresh local at
// its first ASTORE. Reassignments must preserve the caught value on the other
// branch, and must not create a null-initialized declaration that shadows it.
func TestAdversarialCatchParameterReassignRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "CatchParameterReassign", `import java.security.*;
public class CatchParameterReassign {
  static String execute(int kind,int form,boolean unwrap,boolean log) {
    Object previous=null;
    try {
      CatchParameterOracle.mark("outer");
      if(form!=0) {
        try {previous=CatchParameterOracle.read(kind);}
        catch(Exception error) {
          if(unwrap && error instanceof PrivilegedActionException)
            error=((PrivilegedActionException)error).getException();
          if(log)CatchParameterOracle.record(error);
          if(form==2) {
            try {CatchParameterOracle.second(kind);}
            catch(Exception nested) {
              if(nested instanceof PrivilegedActionException)
                nested=((PrivilegedActionException)nested).getException();
              CatchParameterOracle.record(nested);
            }
          }
        }
      }
      return CatchParameterOracle.finish(previous);
    }catch(RuntimeException error){return "runtime:"+error.getMessage();}
  }
  public static void main(String[] args){CatchParameterOracle.run();}
}
class CatchParameterOracle {
  static StringBuilder trace;static int failAt,step;
  static void mark(String text){trace.append(text).append(';');if(++step==failAt)throw new IllegalStateException(text);}
  static Object read(int kind) throws Exception {
    mark("read");
    if(kind==0)return "value";
    if(kind==1)throw new Exception("plain");
    if(kind==2)throw new PrivilegedActionException(new Exception("wrapped"));
    if(kind==3)throw new PrivilegedActionException(new IllegalArgumentException("inner"));
    if(kind==4)throw new IllegalStateException("direct");
    if(kind==5)throw new AssertionError("error");
    throw new NullPointerException("null");
  }
  static void second(int kind) throws Exception {mark("second");read(kind);}
  static void record(Throwable error){mark("record");trace.append(error.getClass().getSimpleName()).append(':').append(error.getMessage()).append(';');}
  static String finish(Object value){mark("finish");return String.valueOf(value);}
  static void run(){
    for(int kind=0;kind<7;kind++)for(int form=0;form<3;form++)for(int unwrap=0;unwrap<2;unwrap++)for(int log=0;log<2;log++)for(failAt=0;failAt<6;failAt++) {
      trace=new StringBuilder();step=0;String result;
      try{result=CatchParameterReassign.execute(kind,form,unwrap!=0,log!=0);}
      catch(Throwable error){result="escaped:"+error.getClass().getSimpleName()+":"+error.getMessage();}
      System.out.println(kind+":"+form+":"+unwrap+":"+log+":"+failAt+":"+result+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
