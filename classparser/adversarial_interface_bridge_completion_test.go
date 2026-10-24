package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// An interface bridge is a default method in modern classfiles. In addition
// to reflection/dispatch, its accidental presence changes interface
// initialization when an implementing class is initialized (JVMS 5.5).
func TestAdversarialInterfaceBridgePreservesInitializationAndDispatch(t *testing.T) {
	const source = `interface GenericContract<T> {T read(long n,T value);}
class InterfaceEffects {static String trace="";static Object seen;static long number;static int mark(){trace+="I";return 7;}}
public interface BridgeContract extends GenericContract<String> {int TOKEN=InterfaceEffects.mark();String read(long n,String value);class Impl implements BridgeContract {public Impl(){InterfaceEffects.trace+="C";}public String read(long n,String value){InterfaceEffects.trace+="R";InterfaceEffects.seen=value;InterfaceEffects.number=n;return value;}}}
class InterfaceDriver {public static void main(String[]args)throws Exception {if(!InterfaceEffects.trace.equals(""))throw new AssertionError("eager setup");BridgeContract instance=new BridgeContract.Impl();if(!InterfaceEffects.trace.equals("EXPECTED_INITIAL"))throw new AssertionError("default bridge changed initialization:"+InterfaceEffects.trace);int count=0;for(java.lang.reflect.Method m:BridgeContract.class.getDeclaredMethods())if(m.getName().equals("read"))count++;if(count!=EXPECTED_METHODS)throw new AssertionError("declared method universe:"+count);int rows=0;for(String value:new String[]{null,new String("same identity")})for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){InterfaceEffects.trace="";if(instance.read(n,value)!=value||InterfaceEffects.seen!=value||InterfaceEffects.number!=n||!InterfaceEffects.trace.equals("R"))throw new AssertionError("typed call");GenericContract raw=instance;InterfaceEffects.trace="";if(raw.read(n,value)!=value||!InterfaceEffects.trace.equals("R"))throw new AssertionError("erased dispatch");InterfaceEffects.trace="";try{raw.read(n,new Object());throw new AssertionError("missing cast");}catch(ClassCastException expected){}if(!InterfaceEffects.trace.equals(""))throw new AssertionError("cast happened after side effect");rows++;}InterfaceEffects.trace="";if(BridgeContract.TOKEN!=7||!InterfaceEffects.trace.equals("EXPECTED_LATE"))throw new AssertionError("explicit interface initialization");System.out.println(rows+":interface:bridge:initialization:dispatch:cast:identity");}}`
	for _, owner := range []string{"BridgeContract", "RenamedBridgeContract"} {
		for _, original := range []int{5, 7, 8} {
			for _, target := range []int{8, 16} {
				t.Run(fmt.Sprintf("%s/original%d/target%d", owner, original, target), func(t *testing.T) {
					initial, late, methods := "C", "I", "1"
					if original == 8 {
						initial, late, methods = "IC", "", "2"
					}
					fixture := strings.NewReplacer("BridgeContract", owner, "EXPECTED_INITIAL", initial, "EXPECTED_LATE", late, "EXPECTED_METHODS", methods).Replace(source)
					roundTripConstructorBridgeArtifact(t, fixture, owner, "InterfaceDriver", "6:interface:bridge:initialization:dispatch:cast:identity\n", "none", target, original)
				})
			}
		}
	}
}
