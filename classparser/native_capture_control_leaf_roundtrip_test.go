package javaclassparser

import "testing"

// A monitor exit/CFG anchor has no Java local operand. It cannot invalidate a
// captured local's declaration identity, or authorize moving the original
// monitor or superclass callback. Original JVMs are the independent oracle.
func TestNativeAnonymousCaptureAfterMonitorBranchRoundTrip(t *testing.T) {
	const fixture = `abstract class MonitorParent{static int seen;MonitorParent(Object[]args){seen=read();}abstract int read();}class MonitorCaptureOwner{final Object lock=new Object();MonitorParent make(final int input){synchronized(lock){if(input<0)return null;return new MonitorParent(new Object[]{input}){int read(){return input;}};}}}class MonitorCaptureDriver{public static void main(String[]args){MonitorCaptureOwner owner=new MonitorCaptureOwner();if(owner.make(-1)!=null||owner.make(17).read()!=17||MonitorParent.seen!=17||Thread.holdsLock(owner.lock))throw new AssertionError("parameter capture, superclass observation, lock release");System.out.println("capture:monitor:branch:super-observation");}}`
	testNativeIndependentFamilyFixture(t, fixture, []string{"MonitorCaptureOwner"}, "MonitorCaptureDriver", "capture:monitor:branch:super-observation\n", nativeLexicalExactSignatures)
}
func TestNativeAnonymousCaptureLocalAcrossNestedMonitorRoundTrip(t *testing.T) {
	const fixture = `abstract class GuardParent{static int seen;GuardParent(String label){seen=read();}abstract int read();}class GuardCaptureOwner{final Object lock=new Object();GuardParent make(int input){final int result=input+3;synchronized(lock){if(input==0)return null;synchronized(this){return new GuardParent("guard"){int read(){return result;}};}}}}class GuardCaptureDriver{public static void main(String[]args){GuardCaptureOwner owner=new GuardCaptureOwner();for(int i:new int[]{1,2,9})if(owner.make(i).read()!=i+3||GuardParent.seen!=i+3||Thread.holdsLock(owner.lock)||Thread.holdsLock(owner))throw new AssertionError("local/monitor capture and callbacks");if(owner.make(0)!=null)throw new AssertionError("branch");System.out.println("capture:local:nested-monitors:callback");}}`
	testNativeIndependentFamilyFixture(t, fixture, []string{"GuardCaptureOwner"}, "GuardCaptureDriver", "capture:local:nested-monitors:callback\n", nativeLexicalExactSignatures)
}

func TestNativeAnonymousCaptureWithinMonitorRejectedExecutionRoundTrip(t *testing.T) {
	const fixture = `abstract class RequestParent{static int seen;RequestParent(String name,Object[]args){seen=read();}abstract int read();}class RequestEffects{static RequestParent saved;static boolean reject;static void accept(RequestParent p){if(reject)throw new java.util.concurrent.RejectedExecutionException();saved=p;}}class RequestCaptureOwner{final Object lock=new Object();java.util.Set<Integer> active=new java.util.HashSet<Integer>();void make(final int input,final java.util.List<String> items){synchronized(lock){if(active.contains(input)){return;}else{active.add(input);try{RequestEffects.accept(new RequestParent("request",new Object[]{input}){int read(){return input+items.size();}});}catch(java.util.concurrent.RejectedExecutionException expected){}return;}}}}class RequestCaptureDriver{public static void main(String[]args){RequestCaptureOwner owner=new RequestCaptureOwner();java.util.List<String> items=new java.util.ArrayList<String>();items.add("a");owner.make(4,items);if(RequestEffects.saved.read()!=5||RequestParent.seen!=5||Thread.holdsLock(owner.lock))throw new AssertionError("request capture/parent observation");RequestEffects.reject=true;owner.make(8,items);if(RequestParent.seen!=9||!owner.active.contains(8)||Thread.holdsLock(owner.lock))throw new AssertionError("rejected effect/monitor");System.out.println("capture:request:rejected:monitor");}}`
	testNativeIndependentFamilyFixture(t, fixture, []string{"RequestCaptureOwner"}, "RequestCaptureDriver", "capture:request:rejected:monitor\n", nativeLexicalExactSignatures)
}
