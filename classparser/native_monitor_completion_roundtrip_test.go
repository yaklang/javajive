package javaclassparser

import (
	"strings"
	"testing"
)

const nativeMonitorCompletionFixture = `class MonitorCompletionEffects {
 static String trace="";static Object outer,inner;
 static final java.io.IOException checked=new java.io.IOException("checked");
 static final RuntimeException unchecked=new RuntimeException("unchecked"),cleanup=new RuntimeException("cleanup");
 static void mark(String tag,boolean needInner){if(!Thread.holdsLock(outer)||Thread.holdsLock(inner)!=needInner)throw new AssertionError("monitor identity "+tag);trace+=tag;}
 static void action(int mode)throws java.io.IOException{mark("A",false);if(mode==2)throw checked;if(mode==3)throw unchecked;}
 static void clean(int mode){mark("F",false);if(mode==4)throw cleanup;}
}
class MonitorCompletionOwner {
 private Object token;MonitorCompletionOwner(Object token){this.token=token;}
 static class Reader{Object read(MonitorCompletionOwner x){return x.token;}}
 int run(Object external,int mode){synchronized(external){MonitorCompletionEffects.mark("E",false);if(mode==0)return 17;try{MonitorCompletionEffects.action(mode);}catch(java.io.IOException e){if(e!=MonitorCompletionEffects.checked)throw new AssertionError("checked identity");MonitorCompletionEffects.mark("C",false);}finally{MonitorCompletionEffects.clean(mode);synchronized(this){MonitorCompletionEffects.mark("I",true);token=null;}}MonitorCompletionEffects.mark("R",false);return 23;}}
}
class MonitorCompletionDriver{public static void main(String[]args){int rows=0;for(int mode=0;mode<5;mode++){Object external=new Object(),payload=new Object();MonitorCompletionOwner owner=new MonitorCompletionOwner(payload);MonitorCompletionEffects.outer=external;MonitorCompletionEffects.inner=owner;MonitorCompletionEffects.trace="";RuntimeException seen=null;int result=-1;try{result=owner.run(external,mode);}catch(RuntimeException e){seen=e;}String want=mode==0?"E":mode==2?"EACFIR":mode==3?"EAFI":mode==4?"EAF":"EAFIR";if(!MonitorCompletionEffects.trace.equals(want))throw new AssertionError("trace "+mode+":"+MonitorCompletionEffects.trace);RuntimeException expected=mode==3?MonitorCompletionEffects.unchecked:mode==4?MonitorCompletionEffects.cleanup:null;if(seen!=expected||(seen==null&&result!=(mode==0?17:23)))throw new AssertionError("completion identity");Object field=new MonitorCompletionOwner.Reader().read(owner);if(field!=(mode==0||mode==4?payload:null))throw new AssertionError("cleanup partial state");if(Thread.holdsLock(external)||Thread.holdsLock(owner))throw new AssertionError("monitor leaked");rows++;}Object payload=new Object();MonitorCompletionOwner owner=new MonitorCompletionOwner(payload);MonitorCompletionEffects.trace="";boolean failed=false;try{owner.run(null,0);}catch(NullPointerException e){failed=true;}if(!failed||!MonitorCompletionEffects.trace.equals("")||new MonitorCompletionOwner.Reader().read(owner)!=payload)throw new AssertionError("failed acquisition ran cleanup");System.out.println(rows+":monitor:abrupt:finally:identity:oracle");}}`

func TestNativeMonitorAbruptFinallySourceBindingRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeMonitorCompletionFixture, "MonitorCompletionOwner", "MonitorCompletionDriver", "5:monitor:abrupt:finally:identity:oracle\n")
}
func TestNativeMonitorAbruptFinallySourceBindingRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeMonitorCompletionFixture, "MonitorCompletionOwner", "OtherMonitorLexicalScope")
	testNativePrivateSetterFixture(t, f, "OtherMonitorLexicalScope", "MonitorCompletionDriver", "5:monitor:abrupt:finally:identity:oracle\n")
}
