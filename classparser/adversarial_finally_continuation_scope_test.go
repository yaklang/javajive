package javaclassparser

import (
	"strings"
	"testing"
)

const tryValueScopeFixture = `
interface ScopeView {}
class ScopeValue implements ScopeView {}
class ScopeSoft extends RuntimeException { final boolean severe; ScopeSoft(boolean s){severe=s;} }
class ScopeFatal extends ScopeSoft { ScopeFatal(){super(true);} }
class ScopeBox { final Object value; ScopeBox(Object v){value=v;} Object get(){if(ScopedFactory.MONITORED&&!Thread.holdsLock(ScopedFactory.active))throw new AssertionError("box escaped monitor");ScopedFactory.trace+="H";return value;} }
public class ScopedFactory {
 static final boolean MONITORED=true;
 static Object active;static String trace="";
 static final ScopeSoft soft=new ScopeSoft(false),severe=new ScopeSoft(true);
 static final ScopeFatal fatal=new ScopeFatal();static final RuntimeException terminal=new IllegalStateException("finally");
 private final Object mutex=new Object();private ScopeBox cachedBox;private boolean busy;
 private void step(String s){if(MONITORED&&!Thread.holdsLock(mutex))throw new AssertionError("effect escaped monitor");trace+=s;}
 private void before(){step("B");busy=true;}
 private Object resolve(int mode,Object seed){step("R");if(mode==1)throw soft;if(mode==2)throw severe;if(mode==3)throw fatal;return mode==4?null:seed;}
 private ScopeBox create(Object seed){step("C");return new ScopeBox(seed);}
 private void after(boolean fail){step("F");busy=false;if(fail)throw terminal;}
 private ScopeView view(Object value,int mode){step("U");if(mode==5)throw soft;if(mode==6)throw severe;if(mode==7)throw fatal;return (ScopeView)value;}
 ScopeView read(boolean cached,boolean existing,boolean cleanupFail,int mode,Object seed){
  active=mutex;synchronized(mutex){
   ScopeBox box=cached?new ScopeBox(seed):null;
   if(box!=null){ScopeView prior=(ScopeView)box.get();return prior;}
   else {Object candidate=existing?seed:null;
    if(candidate instanceof ScopeView){ScopeView prior=(ScopeView)candidate;return prior;}
    else {if(!busy){Object value;
     try{before();value=resolve(mode,seed);if(value==null){box=create(seed);value=box.get();}}
     catch(ScopeFatal error){throw error;}
     catch(ScopeSoft error){if(error.severe)throw error;else{step("S");ScopeView empty=null;return empty;}}
     finally{after(cleanupFail);}
     ScopeView result=view(value,mode);if(box!=null){cachedBox=box;step("W");}return result;
    }else{return null;}}
   }
  }
 }
}
class ScopeDriver {
 public static void main(String[]args){int rows=0;
  for(boolean cached:new boolean[]{false,true})for(boolean existing:new boolean[]{false,true})for(Object seed:new Object[]{null,new ScopeValue()})for(boolean cleanupFail:new boolean[]{false,true})for(int mode=0;mode<8;mode++){
   ScopedFactory target=new ScopedFactory();ScopedFactory.trace="";boolean early=cached||(existing&&seed!=null),created=seed==null||mode==4;
   RuntimeException expected=early?null:cleanupFail?ScopedFactory.terminal:mode==2||mode==6?ScopedFactory.severe:mode==3||mode==7?ScopedFactory.fatal:mode==5?ScopedFactory.soft:null;
   Object want=early?seed:mode==1?null:seed;String trace=early?(cached?"H":""):mode==1?"BRSF":mode==2||mode==3?"BRF":"BR"+(created?"CH":"")+"F"+(cleanupFail?"":"U"+(mode<5&&created?"W":""));
   try{Object value=target.read(cached,existing,cleanupFail,mode,seed);if(expected!=null||value!=want)throw new AssertionError("value or missing original exception");}
   catch(RuntimeException error){if(error!=expected)throw new AssertionError("failure identity",error);}
   if(Thread.holdsLock(ScopedFactory.active))throw new AssertionError("monitor retained after exit");if(!ScopedFactory.trace.equals(trace))throw new AssertionError("order/cleanup:"+ScopedFactory.trace+" expected:"+trace);rows++;
  }
  System.out.println(rows+":scope:alias:catch:finally:monitor:identity");
 }
}`

func TestAdversarialFinallyContinuationKeepsTryValueScope(t *testing.T) {
	for _, monitor := range []bool{false, true} {
		for _, owner := range []string{"ScopedFactory", "ChangedFactory"} {
			fixture := strings.ReplaceAll(tryValueScopeFixture, "ScopedFactory", owner)
			label := owner + "/monitor"
			if !monitor {
				fixture = strings.Replace(fixture, "MONITORED=true", "MONITORED=false", 1)
				fixture = strings.Replace(fixture, "synchronized(mutex)", "", 1)
				label = owner + "/plain"
			}
			t.Run(label, func(t *testing.T) {
				testNativePrivateSetterCompiledFixtureWithPolicies(t, owner, "ScopeDriver", "128:scope:alias:catch:finally:monitor:identity\n", func(t *testing.T, debug string) map[string][]byte {
					return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
				}, nil, []string{"normal"})
			})
		}
	}
}
