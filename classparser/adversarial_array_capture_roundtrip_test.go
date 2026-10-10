package javaclassparser

import (
	"strings"
	"testing"
)

// Array stores modify elements, not the identity of a source variable. Both
// producer loops precede an anonymous member-SUPER allocation. The unchanged
// driver observes each copied element, deferred override, owner, and event.
func TestAdversarialArrayElementStoresKeepAnonymousCaptureOwnership(t *testing.T) {
	testArrayStoreCaptureFamily(t, false)
}

func TestAdversarialArrayStoresKeepCapturedArrayIdentity(t *testing.T) {
	testArrayStoreCaptureFamily(t, true)
}

// The same source traversal proves precise catch rethrow. Storing the caught
// object in a heap array does not overwrite the catch parameter. Missing parent
// declarations must not be replaced with an invented broad-throws helper.
func TestAdversarialArrayStoresKeepPreciseCatchRethrow(t *testing.T) {
	roundTripGenericFlowUnits(t, "ArrayRethrowDriver", `
class ArrayRethrowParent {}
class ArrayRethrowOps {
 static final java.io.IOException checked=new java.io.IOException("checked");
 static final RuntimeException runtime=new RuntimeException("runtime");
 static final Error fatal=new AssertionError("fatal");
 static int calls,records;static Throwable recorded;
 static int effect(int mode)throws java.io.IOException{calls++;if(mode==1)throw checked;if(mode==2)throw runtime;if(mode==3)throw fatal;return 31;}
 static void record(Throwable failure){records++;recorded=failure;}
}
class ArrayRethrowConsumer extends ArrayRethrowParent {
 static int run(int mode,int slot)throws java.io.IOException{
  try{return ArrayRethrowOps.effect(mode);}catch(Throwable caught){
   Throwable[] seen=new Throwable[2];seen[slot]=caught;ArrayRethrowOps.record(seen[slot]);throw caught;
  }
 }
}
public class ArrayRethrowDriver {public static void main(String[]args){for(int mode=0;mode<4;mode++)for(int slot=0;slot<2;slot++){
 ArrayRethrowOps.calls=ArrayRethrowOps.records=0;ArrayRethrowOps.recorded=null;Object result;
 try{result=Integer.valueOf(ArrayRethrowConsumer.run(mode,slot));}catch(Throwable failure){result=failure;}
 Object expected=mode==1?ArrayRethrowOps.checked:mode==2?ArrayRethrowOps.runtime:mode==3?ArrayRethrowOps.fatal:Integer.valueOf(31);
 if((expected instanceof Throwable?result!=expected:!expected.equals(result))||ArrayRethrowOps.calls!=1||ArrayRethrowOps.records!=(mode==0?0:1)||(mode!=0&&ArrayRethrowOps.recorded!=expected))throw new AssertionError("array-store rethrow identity/effects");
 System.out.println(mode+":"+slot+":"+(result instanceof Throwable?((Throwable)result).getMessage():result));
 }}}
`, func(name string) bool { return name != "ArrayRethrowParent" }, []string{"ArrayRethrowConsumer"}, Precision, Compatibility, "legacy")
}

func testArrayStoreCaptureFamily(t *testing.T, captureArray bool) {
	t.Helper()
	const source = `class ArrayCaptureEffects{static String trace="";static String convert(Object x){trace+="C";return x==null?"nil":x.toString();}}
class ArrayCaptureOwner{
 private abstract class Parent{final Object[] data;Parent(Object[] data){ArrayCaptureEffects.trace+="P";this.data=data;}abstract String element(Object x);String render(){StringBuilder b=new StringBuilder();for(int i=0;i<data.length;i++){if(i!=0)b.append(";");b.append(element(data[i]));}return b.toString();}Object owner(){return ArrayCaptureOwner.this;}}
 Parent fromArray(Object[] input){String[] copy=new String[input.length];for(int i=0;i<input.length;i++){copy[i]=ArrayCaptureEffects.convert(input[i]);}return new Parent(copy){String element(Object x){ArrayCaptureEffects.trace+="E";return "V="+x;}};}
 Parent fromList(java.util.List input){String[] copy=new String[input.size()];for(int i=0;i<input.size();i++){copy[i]=ArrayCaptureEffects.convert(input.get(i));}return new Parent(copy){String element(Object x){ArrayCaptureEffects.trace+="E";return "V="+x;}};}
 String array(Object[] x){Parent p=fromArray(x);if(p.owner()!=this||p.getClass().getEnclosingClass()!=ArrayCaptureOwner.class||!p.getClass().getName().equals("ArrayCaptureOwner$1"))throw new AssertionError("array owner");return p.render();}
 String list(java.util.List x){Parent p=fromList(x);if(p.owner()!=this||p.getClass().getEnclosingClass()!=ArrayCaptureOwner.class||!p.getClass().getName().equals("ArrayCaptureOwner$2"))throw new AssertionError("list owner");return p.render();}}
class ArrayCaptureDriver{public static void main(String[]args){ArrayCaptureOwner owner=new ArrayCaptureOwner();int rows=0;for(Object[]input:new Object[][]{new Object[0],new Object[]{null},new Object[]{"a",null,"b"},new Object[]{1,-1,0,Long.MIN_VALUE,Long.MAX_VALUE}}){String want="";String trace="";for(int i=0;i<input.length;i++){if(i>0)want+=";";want+="V="+(input[i]==null?"nil":input[i].toString());trace+="C";}trace+="P";for(int i=0;i<input.length;i++)trace+="E";ArrayCaptureEffects.trace="";if(!owner.array(input).equals(want)||!ArrayCaptureEffects.trace.equals(trace))throw new AssertionError("array value/effects");ArrayCaptureEffects.trace="";if(!owner.list(java.util.Arrays.asList(input)).equals(want)||!ArrayCaptureEffects.trace.equals(trace))throw new AssertionError("list value/effects");rows+=2;}System.out.println(rows+":array-store:original-owner:values:effects");}}`
	fixture := source
	if captureArray {
		fixture = strings.ReplaceAll(fixture, `ArrayCaptureEffects.trace+="E";return "V="+x;`, `if(data!=copy)throw new AssertionError("captured array identity");ArrayCaptureEffects.trace+="E";return "V="+x;`)
	}
	testNativePrivateSetterSourceFixture(t, map[string]string{"ArrayCaptureOwner.java": fixture}, "ArrayCaptureOwner", "ArrayCaptureDriver", "8:array-store:original-owner:values:effects\n")
}
