package javaclassparser

import (
	"strings"
	"testing"
)

const nativeNestedCapturePhaseFixture = `class CapturePhaseBox{final int number;CapturePhaseBox(int n){number=n;}}
class CapturePhaseInput{CapturePhaseBox value;CapturePhaseBox load(){return value;}}
abstract class CapturePhaseBase{CapturePhaseBase(Object ignored){if(check()!=7)throw new AssertionError("capture before parent callback");}abstract int check();}
abstract class CapturePhaseProvider{abstract CapturePhaseResult result(CapturePhaseInput input);}
class CapturePhaseResult{final CapturePhaseProvider provider;final CapturePhaseBase base;CapturePhaseResult(CapturePhaseProvider p,CapturePhaseBase b){provider=p;base=b;}CapturePhaseResult(Object p,Object b){throw new AssertionError("wrong typed binding");}}
class CapturePhaseOwner{CapturePhaseProvider make(){return new CapturePhaseProvider(){CapturePhaseResult result(CapturePhaseInput input){final CapturePhaseBox value=input.load();if(value==null)return null;CapturePhaseBase base=new CapturePhaseBase(value){int check(){return value.number;}};return new CapturePhaseResult(this,base);}};}}
class CapturePhaseDriver{public static void main(String[]args){CapturePhaseProvider p=new CapturePhaseOwner().make();CapturePhaseInput input=new CapturePhaseInput();if(p.result(input)!=null)throw new AssertionError("null branch");input.value=new CapturePhaseBox(7);CapturePhaseResult r=p.result(input);if(r.provider!=p||r.base.check()!=7)throw new AssertionError("nested identity");System.out.println("nested:local:capture:callback:binding:identity");}}`

func TestNativeNestedCapturePhaseRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeNestedCapturePhaseFixture, "CapturePhaseOwner", "CapturePhaseDriver", "nested:local:capture:callback:binding:identity\n")
}
func TestNativeNestedCapturePhaseRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeNestedCapturePhaseFixture, "CapturePhaseOwner", "DifferentPreparationScope")
	testNativePrivateSetterFixture(t, f, "DifferentPreparationScope", "CapturePhaseDriver", "nested:local:capture:callback:binding:identity\n")
}

func TestNativeNestedCapturePhaseFailureOrderRoundTrip(t *testing.T) {
	f := strings.Replace(nativeNestedCapturePhaseFixture, "CapturePhaseBox value;CapturePhaseBox load(){return value;}", `CapturePhaseBox value;RuntimeException failure;CapturePhaseBox load(){CapturePhaseTrace.trace+="L";if(failure!=null)throw failure;return value;}`, 1)
	f = strings.Replace(f, `CapturePhaseBase(Object ignored){if(check()!=7)`, `CapturePhaseBase(Object ignored){CapturePhaseTrace.trace+="P";if(check()!=7)`, 1)
	f = strings.Replace(f, `int check(){return value.number;}`, `int check(){CapturePhaseTrace.trace+="C";return value.number;}`, 1)
	f = strings.Replace(f, `CapturePhaseResult(CapturePhaseProvider p,CapturePhaseBase b){provider=p;base=b;}`, `CapturePhaseResult(CapturePhaseProvider p,CapturePhaseBase b){CapturePhaseTrace.trace+="R";if(CapturePhaseTrace.failure!=null)throw CapturePhaseTrace.failure;provider=p;base=b;}`, 1)
	start := strings.Index(f, "class CapturePhaseDriver")
	f = f[:start] + `class CapturePhaseTrace{static String trace="";static RuntimeException failure;}
 class CapturePhaseDriver{public static void main(String[]args){CapturePhaseProvider p=new CapturePhaseOwner().make();CapturePhaseInput input=new CapturePhaseInput();if(p.result(input)!=null||!CapturePhaseTrace.trace.equals("L"))throw new AssertionError("null branch");input.value=new CapturePhaseBox(7);CapturePhaseTrace.trace="";CapturePhaseResult r=p.result(input);if(r.provider!=p||r.base.check()!=7||!CapturePhaseTrace.trace.equals("LPCRC"))throw new AssertionError("original once/callback/binding order:"+CapturePhaseTrace.trace);RuntimeException failure=new RuntimeException("identity");input.failure=failure;CapturePhaseTrace.trace="";try{p.result(input);throw new AssertionError("producer failure missing");}catch(RuntimeException got){if(got!=failure||!CapturePhaseTrace.trace.equals("L"))throw new AssertionError("producer order");}input.failure=null;input.value=new CapturePhaseBox(8);CapturePhaseTrace.trace="";try{p.result(input);throw new AssertionError("parent callback failure missing");}catch(AssertionError got){if(!CapturePhaseTrace.trace.equals("LPC"))throw new AssertionError("parent order:"+CapturePhaseTrace.trace);}input.value=new CapturePhaseBox(7);CapturePhaseTrace.failure=failure;CapturePhaseTrace.trace="";try{p.result(input);throw new AssertionError("result failure missing");}catch(RuntimeException got){if(got!=failure||!CapturePhaseTrace.trace.equals("LPCR"))throw new AssertionError("result order");}System.out.println("nested:failure:callback:producer:once:identity");}}`
	testNativePrivateSetterFixture(t, f, "CapturePhaseOwner", "CapturePhaseDriver", "nested:failure:callback:producer:once:identity\n")
}
func TestNativeNestedCapturePhaseWideRoundTrip(t *testing.T) {
	f := strings.Replace(nativeNestedCapturePhaseFixture, "CapturePhaseBox value;", "CapturePhaseBox value;long bits;", 1)
	f = strings.Replace(f, `CapturePhaseBase(Object ignored){if(check()!=7)`, `CapturePhaseBase(Object ignored,long bits){if(stamp()!=bits)throw new AssertionError("wide capture before callback");if(check()!=7)`, 1)
	f = strings.Replace(f, `abstract int check();`, `abstract int check();abstract long stamp();`, 1)
	f = strings.Replace(f, `final CapturePhaseBox value=input.load();`, `final long bits=input.bits;final CapturePhaseBox value=input.load();`, 1)
	f = strings.Replace(f, `new CapturePhaseBase(value){int check(){return value.number;}}`, `new CapturePhaseBase(value,bits){int check(){return value.number;}long stamp(){return bits;}}`, 1)
	start := strings.Index(f, "class CapturePhaseDriver")
	f = f[:start] + `class CapturePhaseDriver{public static void main(String[]args){CapturePhaseProvider p=new CapturePhaseOwner().make();CapturePhaseInput input=new CapturePhaseInput();if(p.result(input)!=null)throw new AssertionError("null branch");input.value=new CapturePhaseBox(7);for(long bits:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){input.bits=bits;CapturePhaseResult r=p.result(input);if(r.provider!=p||r.base.check()!=7||r.base.stamp()!=bits)throw new AssertionError("wide nested identity:"+bits);}System.out.println("nested:wide:capture:callback:binding:identity");}}`
	testNativePrivateSetterFixture(t, f, "CapturePhaseOwner", "CapturePhaseDriver", "nested:wide:capture:callback:binding:identity\n")
}
