package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialLegacyShapesPreserveCatchAndLaterLocalScopes(t *testing.T) {
	in := `class C {
	Handle install(Handle result) {
		try {
			try { call(); }
			catch (Throwable var6) {
				var6 = reconcile(var6);
				if (var6 != null) throw new IllegalStateException(var6);
			}
			Handle var6 = result;
			return var6;
		} catch (Throwable var4) { throw var4; }
	}
}`
	out := fixHardjarShapes(in)
	if strings.Contains(out, "Handle var6 = null;") || strings.Contains(out, "\n\t\tvar6 = null;") || !strings.Contains(out, "Handle var6 = result;") {
		t.Fatalf("a bound catch parameter must not hoist a different local with the same name:\n%s", out)
	}
}

func TestAdversarialCatchThenLocalIdentityRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "InstallLifecycle", `public class InstallLifecycle {
  static Handle install(boolean acquire,int fault,boolean recover,Handle handle) {
    if(!acquire)throw new IllegalStateException("lock");
    try {
      InstallOracle.mark("begin");
      try {InstallOracle.mark("install");InstallOracle.fail(fault);}
      catch(Throwable error) {
        error=InstallOracle.reconcile(error,recover);
        if(error!=null){InstallOracle.mark("remove");throw new IllegalStateException("rejected",error);}
      }
      InstallOracle.mark("success");
      Handle result=handle;
      InstallOracle.mark("release");
      return result;
    }catch(Throwable error){InstallOracle.mark("release-error");throw error;}
  }
  public static void main(String[] args){InstallOracle.run();}
}
class Handle {final int id;Handle(int id){this.id=id;}}
class InstallOracle {
  static StringBuilder trace;static int failAt,step;
  static void mark(String text){trace.append(text).append(';');if(++step==failAt)throw new IllegalArgumentException(text);}
  static void fail(int fault){if(fault==1)throw new IllegalArgumentException("input");if(fault==2)throw new LinkageError("linkage");}
  static Throwable reconcile(Throwable error,boolean recover){mark("reconcile:"+error.getClass().getSimpleName());return recover?null:error;}
  static String describe(Throwable error){String text=error.getClass().getSimpleName()+":"+error.getMessage();return error.getCause()==null?text:text+"/"+describe(error.getCause());}
  static void run(){
    for(int acquire=0;acquire<2;acquire++)for(int fault=0;fault<3;fault++)for(int recover=0;recover<2;recover++)for(int absent=0;absent<2;absent++)for(failAt=0;failAt<8;failAt++){
      trace=new StringBuilder();step=0;Handle handle=absent==0?new Handle(19):null;String outcome;
      try{Handle result=InstallLifecycle.install(acquire!=0,fault,recover!=0,handle);if(result!=handle)throw new AssertionError("identity");outcome=result==null?"null":"handle:"+result.id;}
      catch(Throwable error){outcome=describe(error);}
      System.out.println(acquire+":"+fault+":"+recover+":"+absent+":"+failAt+":"+outcome+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
