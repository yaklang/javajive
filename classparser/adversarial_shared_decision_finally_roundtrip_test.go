package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// A shared decision tail must not exhaust a domain proof merely because its
// unfolded formula has many paths. The original handler protects the typed
// catch's rethrow too, so cleanup must run on every abrupt decision evaluation.
func TestAdversarialSharedDecisionFinallyDomainRoundTrip(t *testing.T) {
	var source strings.Builder
	source.WriteString(`class SharedFinallyEffects{static String trace="";static int fail;static long mask;static final RuntimeException SAME=new RuntimeException("same");static boolean gate(int i){trace+="G"+i+";";if(i==fail)throw SAME;return (mask&(1L<<i))!=0;}}
class SharedFinallyConsumer{`)
	for _, n := range []int{3, 9, 13} {
		fmt.Fprintf(&source, "static int depth%d(){try{return (", n)
		for i := 0; i < n; i++ {
			if i > 0 {
				source.WriteString("&&")
			}
			fmt.Fprintf(&source, "(!SharedFinallyEffects.gate(%d)||SharedFinallyEffects.gate(%d))", 2*i, 2*i+1)
		}
		source.WriteString(`)?2:-3;}catch(RuntimeException ex){SharedFinallyEffects.trace+="E;";throw ex;}finally{SharedFinallyEffects.trace+="F;";}}`)
	}
	source.WriteString(`}class SharedFinallyObserver{static void run(){for(long mask:new long[]{0L,-1L,0x555555L,0x333333L})for(int fail:new int[]{-1,0,1,5,17,25}){SharedFinallyEffects.mask=mask;SharedFinallyEffects.fail=fail;`)
	for _, n := range []int{3, 9, 13} {
		fmt.Fprintf(&source, `SharedFinallyEffects.trace="";try{System.out.println("%d:"+SharedFinallyConsumer.depth%d()+":"+SharedFinallyEffects.trace);}catch(RuntimeException ex){System.out.println("%d:"+(ex==SharedFinallyEffects.SAME)+":"+SharedFinallyEffects.trace);}`, n, n, n)
	}
	source.WriteString(`}}}public class SharedFinallyDriver{public static void main(String[]args){SharedFinallyObserver.run();}}`)
	roundTripGenericFlowUnits(t, "SharedFinallyDriver", source.String(), nil, []string{"SharedFinallyConsumer"}, Precision, Compatibility, "legacy")
}
