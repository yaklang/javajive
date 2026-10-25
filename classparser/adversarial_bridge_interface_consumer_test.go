package javaclassparser

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestAdversarialInheritedBridgeConsumerScanRetainsUnrelatedInterfaceCalls(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	for _, kind := range []string{"reference", "void", "wide", "arrays"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, root := range []string{"BridgeOuter", "DifferentOwner"} {
				t.Run(fmt.Sprintf("%s/%s/%s", kind, debug, root), func(t *testing.T) {
					declaration := `public static Object side(java.util.concurrent.Callable<Object> p)throws Exception{return p.call();}`
					observer := `final Object token=new Object();if(BridgeOuter.Leaf.side(new java.util.concurrent.Callable<Object>(){public Object call(){return token;}})!=token)throw new AssertionError("interface reference identity");`
					switch kind {
					case "void":
						declaration = `public static void side(Runnable p){p.run();}`
						observer = `final int[] count={0};BridgeOuter.Leaf.side(new Runnable(){public void run(){count[0]++;}});if(count[0]!=1)throw new AssertionError("interface effects");`
					case "arrays":
						declaration = `public static long side(BridgeProbe p,long[] n,double[][] d,Object token)throws java.io.IOException{return p.apply(n,d,token);}`
						observer = `final Object token=new Object();final long[] numbers={Long.MIN_VALUE};final double[][] reals={{-0.0}};final int[] count={0};
 BridgeProbe probe=new BridgeProbe(){public long apply(long[] n,double[][] d,Object value){count[0]++;if(n!=numbers||d!=reals||value!=token)throw new AssertionError("array argument identity");return n[0]^Double.doubleToRawLongBits(d[0][0]);}};
 if(BridgeOuter.Leaf.side(probe,numbers,reals,token)!=(Long.MIN_VALUE^Double.doubleToRawLongBits(-0.0))||count[0]!=1)throw new AssertionError("array slots and effects");`
					case "wide":
						declaration = `public static long side(BridgeProbe p,long n,double d,Object token)throws java.io.IOException{return p.apply(n,d,token);}`
						observer = `final Object token=new Object();final int[] count={0};final java.io.IOException failure=new java.io.IOException("identity");
       BridgeProbe probe=new BridgeProbe(){public long apply(long n,double d,Object value)throws java.io.IOException{count[0]++;if(value!=token)throw failure;return n^Double.doubleToRawLongBits(d);}};
       for(long n:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(double d:new double[]{-0.0,Double.NaN,Double.POSITIVE_INFINITY}){
        long expected=java.math.BigInteger.valueOf(n).xor(java.math.BigInteger.valueOf(Double.doubleToRawLongBits(d))).longValue();
        if(BridgeOuter.Leaf.side(probe,n,d,token)!=expected)throw new AssertionError("wide interface argument slots");
       }if(count[0]!=9)throw new AssertionError("interface call count");
       try{BridgeOuter.Leaf.side(probe,1,2,null);throw new AssertionError("missing interface failure");}catch(java.io.IOException e){if(e!=failure)throw new AssertionError("interface exception identity");}
       try{BridgeOuter.Leaf.side(null,1,2,token);throw new AssertionError("missing null receiver");}catch(NullPointerException expected){}if(count[0]!=10)throw new AssertionError("null invocation effects");`
					}
					fixture := strings.Replace(inheritedCovariantBridgeFixture, "public Leaf(){}", "public Leaf(){}"+declaration, 1)
					probe := `interface BridgeProbe {long apply(long n,double d,Object token)throws java.io.IOException;}`
					if kind == "arrays" {
						probe = `interface BridgeProbe {long apply(long[] n,double[][] d,Object token)throws java.io.IOException;}`
					}
					fixture = probe + fixture
					fixture = strings.Replace(fixture, `{if(bridge!=null||!m.isSynthetic()`, `{if(!m.getName().equals("read"))continue;if(bridge!=null||!m.isSynthetic()`, 1)
					fixture = strings.Replace(fixture, `Object leaf=BridgeOuter.leaf();`, observer+`if(Class.forName("BridgeOuter$Leaf").getDeclaredMethods().length!=2)throw new AssertionError("exact original method inventory");Object leaf=BridgeOuter.leaf();`, 1)
					fixture = strings.ReplaceAll(fixture, "BridgeOuter", root)
					testIndependentInheritedBridge(t, modern, java, native, fixture, root, debug, false, "36:inherited:covariant:bridge:dispatch:wide:exception\n")
				})
			}
		}
	}
}
