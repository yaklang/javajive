package javaclassparser

import (
	"strings"
	"testing"
)

const constructorPostInitHandlerFixture = `
class CaptureHandlerTrace {
 static String trace="";static final RuntimeException failure=new RuntimeException("body");
 static void tick(int n){trace+="B";if(n<0)throw failure;}
 static void caught(RuntimeException e){if(e!=failure)throw new AssertionError("handler identity");trace+="C";}
}
class CaptureHandlerBase {final int number;CaptureHandlerBase(int value){number=value;}Object capture(){return null;}Object owner(){return null;}}
class CaptureHandlerOwner {
 CaptureHandlerBase make(final Object token,int n){return new CaptureHandlerBase(n){
  {try{CaptureHandlerTrace.tick(n);}catch(RuntimeException failure){CaptureHandlerTrace.caught(failure);}}
  Object capture(){return token;}Object owner(){return CaptureHandlerOwner.this;}
 };}
}
public class CaptureHandlerDriver {public static void main(String[]args){
 CaptureHandlerOwner owner=new CaptureHandlerOwner();Object token=new Object();
 for(Object value:new Object[]{null,token})for(int n:new int[]{-2,0,2}){
  CaptureHandlerTrace.trace="";CaptureHandlerBase result=owner.make(value,n);
  if(result.capture()!=value||result.owner()!=owner||result.number!=n||!CaptureHandlerTrace.trace.equals(n<0?"BC":"B"))throw new AssertionError("capture/handler behavior");
  System.out.println(n+":"+(value==token)+":"+CaptureHandlerTrace.trace);
 }
}}
`

// An original handler beginning after initialization owns only the later body;
// it must not masquerade as an exception domain covering earlier capture stores
// or receiver initialization. The original superclass/owner/driver are retained.
func TestAdversarialConstructorCaptureWithPostInitHandlerRoundTrip(t *testing.T) {
	for _, variant := range []string{"original", "renamed"} {
		t.Run(variant, func(t *testing.T) {
			f, unit := constructorPostInitHandlerFixture, "CaptureHandlerOwner$1"
			if variant == "renamed" {
				f = strings.ReplaceAll(f, "CaptureHandlerOwner", "PostBoundaryOwner")
				f = strings.ReplaceAll(f, "CaptureHandlerBase", "PostBoundaryBase")
				unit = "PostBoundaryOwner$1"
			}
			roundTripGenericFlowUnits(t, "CaptureHandlerDriver", f, nil, []string{unit}, Precision, Compatibility, "legacy")
		})
	}
}
