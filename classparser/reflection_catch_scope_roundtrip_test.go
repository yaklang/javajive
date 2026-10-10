package javaclassparser

import "testing"

// Checked exceptions come from invocation declarations and the exception table.
// A method-name scan cannot prove their absence, including when a Class[] local
// is declared outside the protected interval or the throwing callee has another
// name. Helpers retain their original declarations as an independent oracle.
func TestAdversarialReflectionCatchScopeRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ScopedReflectionCatch", `import java.lang.reflect.*;
public class ScopedReflectionCatch {
  static String method(Class<?> owner,String name,int form) {
    Class<?>[] params=CatchReflectionOracle.types(form);
    try {
      CatchReflectionOracle.mark("method");
      return owner.getMethod(name,params).getName();
    } catch(NoSuchMethodException | SecurityException e) {
      return CatchReflectionOracle.caught(e);
    }
  }
  static String constructor(Class<?> owner,int form) {
    Class<?>[] params=CatchReflectionOracle.types(form);
    try {
      CatchReflectionOracle.mark("ctor");
      return owner.getConstructor(params).getName();
    } catch(SecurityException | NoSuchMethodException e) {
      return CatchReflectionOracle.caught(e);
    }
  }
  static String named(CatchReflectionInspector inspector,boolean missing) {
    try {
      CatchReflectionOracle.mark("named");
      return inspector.getMethod(missing ? "missing" : "present","descriptor");
    } catch(NoSuchMethodException | SecurityException e) {
      return CatchReflectionOracle.caught(e);
    }
  }
  static String other(CatchReflectionInspector inspector,boolean missing) {
    try {
      CatchReflectionOracle.mark("other");
      return inspector.resolve(missing);
    } catch(NoSuchMethodException | SecurityException e) {
      return CatchReflectionOracle.caught(e);
    }
  }
  public static void main(String[] args){CatchReflectionOracle.run();}
}
class CatchReflectionInspector {
  String getMethod(String name,String desc) throws NoSuchMethodException {
    CatchReflectionOracle.mark("getMethod");
    if(name.equals("missing"))throw new NoSuchMethodException(desc);
    return name;
  }
  String resolve(boolean missing) throws NoSuchMethodException {
    CatchReflectionOracle.mark("resolve");
    if(missing)throw new NoSuchMethodException("resolve");
    return "resolved";
  }
}
class CatchReflectionOracle {
  static StringBuilder trace;static int step,failAt;
  static void mark(String label){trace.append(label).append(';');if(++step==failAt)throw new SecurityException(label);}
  static Class<?>[] types(int form){mark("types");return form==0 ? new Class<?>[0] : form==1 ? new Class<?>[]{String.class} : new Class<?>[]{Object.class};}
  static String caught(Exception e){mark("caught");return e.getClass().getSimpleName()+":"+e.getMessage();}
  static void run(){
    for(int method=0;method<4;method++)for(int present=0;present<2;present++)for(int form=0;form<3;form++)for(failAt=0;failAt<5;failAt++){
      trace=new StringBuilder();step=0;String result;
      try {
        Class<?> owner=present==0 ? String.class : Object.class;
        if(method==0)result=ScopedReflectionCatch.method(owner,present==0 ? "toString" : "absent",form);
        else if(method==1)result=ScopedReflectionCatch.constructor(owner,form);
        else if(method==2)result=ScopedReflectionCatch.named(new CatchReflectionInspector(),present!=0);
        else result=ScopedReflectionCatch.other(new CatchReflectionInspector(),present!=0);
      }catch(Throwable e){result="escaped:"+e.getClass().getSimpleName()+":"+e.getMessage();}
      System.out.println(method+":"+present+":"+form+":"+failAt+":"+result+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
