package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// The original uses a typed null primary to distinguish an invokestatic owner
// from a same-spelled field. This null is discarded before arguments are
// evaluated; it must neither load that field nor introduce a receiver NPE.
func TestAdversarialStaticClassOwnersKeepValueNamespaceAndEffects(t *testing.T) {
	var source strings.Builder
	source.WriteString("class StaticOwnerDecoy{static int compute(int x){return 123;}static long compute(long x){return 456;}}\n")
	for i := 0; i < 16; i++ {
		fmt.Fprintf(&source, "class Producer%d{static int calls;static final RuntimeException failure=new RuntimeException(\"same\");static int compute(int x){calls++;if(x<0)throw failure;return x*17+%d;}static long compute(long x){throw new AssertionError(\"wrong overload\");}}\n", i, i)
	}
	source.WriteString("class StaticOwnerParent{")
	for i := 8; i < 16; i++ {
		fmt.Fprintf(&source, "static StaticOwnerDecoy Producer%d=new StaticOwnerDecoy();", i)
	}
	source.WriteString("}class StaticOwnerProbe extends StaticOwnerParent{")
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&source, "static StaticOwnerDecoy Producer%d=new StaticOwnerDecoy();", i)
	}
	for i := 0; i < 16; i++ {
		fmt.Fprintf(&source, "static int m%d(int x){return((Producer%d)null).compute(x);}", i, i)
	}
	source.WriteString("}\npublic class StaticOwnerDriver{public static void main(String[]args){int rows=0;")
	for i := 0; i < 16; i++ {
		fmt.Fprintf(&source, "for(int x:new int[]{Integer.MIN_VALUE,-31,-1,0,1,17,Integer.MAX_VALUE}){Producer%d.calls=0;try{int got=StaticOwnerProbe.m%d(x);if(x<0||got!=java.math.BigInteger.valueOf(x).multiply(java.math.BigInteger.valueOf(17)).add(java.math.BigInteger.valueOf(%d)).intValue())throw new AssertionError(\"word/overload\");}catch(RuntimeException e){if(x>=0||e!=Producer%d.failure)throw new AssertionError(\"exception identity\");}if(Producer%d.calls!=1)throw new AssertionError(\"owner/once\");rows++;}", i, i, i, i, i)
	}
	source.WriteString(`System.out.println(rows+":static:owner:word:once:exception:overload");}}`)
	roundTripGenericFlowUnitsClasspath(t, "StaticOwnerDriver", source.String(), nil, []string{"StaticOwnerProbe"}, true, Precision, Compatibility, "legacy")
}

func TestAdversarialInterfaceStaticOwnerAvoidsValueShadowWithoutClassPrimary(t *testing.T) {
	const api = `package owners;public interface ApiProducer{static int compute(int x){OwnerState.calls++;if(x<0)throw OwnerState.failure;return x*17+5;}static long compute(long x){throw new AssertionError("wrong overload");}}`
	const source = `class InterfaceOwnerProbe{static int ApiProducer=19;static int m(int x){return owners.ApiProducer.compute(x);}}
public class InterfaceOwnerDriver{public static void main(String[]args){int rows=0;for(int x:new int[]{Integer.MIN_VALUE,-1,0,1,17,Integer.MAX_VALUE}){owners.OwnerState.calls=0;try{int got=InterfaceOwnerProbe.m(x);if(x<0||got!=java.math.BigInteger.valueOf(x).multiply(java.math.BigInteger.valueOf(17)).add(java.math.BigInteger.valueOf(5)).intValue())throw new AssertionError("value/overload");}catch(RuntimeException e){if(x>=0||e!=owners.OwnerState.failure)throw new AssertionError("failure identity");}if(owners.OwnerState.calls!=1)throw new AssertionError("effects/once");rows++;}System.out.println(rows+":interface:static:value:effects:failure");}}`
	roundTripGenericFlowSources(t, "InterfaceOwnerDriver", source, map[string]string{"owners/ApiProducer.java": api, "owners/OwnerState.java": `package owners;public class OwnerState{public static int calls;public static final RuntimeException failure=new RuntimeException("same");}`}, nil, []string{"InterfaceOwnerProbe"}, true, Precision, Compatibility, "legacy")
}
