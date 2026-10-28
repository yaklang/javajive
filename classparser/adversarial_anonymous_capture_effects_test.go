package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialAnonymousCaptureBeforeSuperAndInitialization(t *testing.T) {
	const fixture = `
class CaptureEffects{static String trace;static int phase;static final RuntimeException FAILURE=new IllegalStateException("identity");static void hit(String s,int p){trace+=s;if(phase==p)throw FAILURE;}}
abstract class SnapshotBase{SnapshotBase(){CaptureEffects.hit("B",1);if(snapshot()==null)throw new AssertionError("capture unavailable in super");}abstract Object snapshot();abstract long number();abstract double fraction();}
public class CaptureFactory{SnapshotBase create(final Object token,final long wide,final double decimal){CaptureEffects.hit("A",2);return new SnapshotBase(){final Object observed=initialize();Object initialize(){CaptureEffects.hit("I",3);return token;}Object snapshot(){return token;}long number(){return wide;}double fraction(){return decimal;}};}}
class CaptureEffectsDriver{public static void main(String[]args){int rows=0;CaptureFactory factory=new CaptureFactory();for(int p=0;p<4;p++)for(long n:new long[]{0L,Long.MIN_VALUE,Long.MAX_VALUE})for(double d:new double[]{-0.0,Double.NaN,Double.POSITIVE_INFINITY}){Object token=new Object();CaptureEffects.trace="";CaptureEffects.phase=p;SnapshotBase value=null;Throwable actual=null;try{value=factory.create(token,n,d);}catch(Throwable e){actual=e;}
 String expected=p==2?"A":p==1?"AB":"ABI";if(!expected.equals(CaptureEffects.trace))throw new AssertionError("effects="+CaptureEffects.trace);
 if(p==0){if(actual!=null||value==null||value.snapshot()!=token||value.number()!=n||Double.doubleToLongBits(value.fraction())!=Double.doubleToLongBits(d))throw new AssertionError("capture mismatch",actual);SnapshotBase other=factory.create(token,n,d);if(other==value)throw new AssertionError("allocation identity");}else if(actual!=CaptureEffects.FAILURE)throw new AssertionError("failure identity",actual);rows++;}System.out.println(rows+":anonymous:numeric:identity:super:effects");}}
`
	for _, spelling := range []string{"original", "renamed"} {
		t.Run(spelling, func(t *testing.T) {
			text, owner := fixture, "CaptureFactory"
			if spelling == "renamed" {
				text = strings.NewReplacer("CaptureFactory", "StoredCallback", "token", "reference", "wide", "counter", "decimal", "ratio").Replace(text)
				owner = "StoredCallback"
			}
			testAuthoredOwnerAcrossSourceModes(t, text, owner, "CaptureEffectsDriver", "36:anonymous:numeric:identity:super:effects\n", owner+"$1")
		})
	}
}
