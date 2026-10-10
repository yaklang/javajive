package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

func wideSourceDependencyFixture(nodes int, cycle bool) (string, []string) {
	var source strings.Builder
	owners := make([]string, nodes)
	for i := 0; i < nodes; i++ {
		owners[i] = fmt.Sprintf("GraphScope%d", i)
		fmt.Fprintf(&source, "class %s {", owners[i])
		if i == 0 {
			source.WriteString(`final Object token;GraphScope0(Object token){this.token=token;}class Capture extends GraphParent {Object observe(){return GraphScope0.this.token;}}Capture make(){return new Capture();}`)
		}
		fmt.Fprintf(&source, `static class Marker {static final int initialized=GraphDriver.mark(%d);final Object token;Marker(Object token){this.token=token;}`, i)
		if i+1 < nodes {
			fmt.Fprintf(&source, "GraphScope%d.Marker next;", i+1)
		}
		if cycle && i == 1 {
			source.WriteString("GraphScope0.Marker back;")
		}
		source.WriteString("}}\n")
	}
	source.WriteString(`abstract class GraphParent {final Object observed;GraphParent(){GraphDriver.callback+="P";observed=observe();if(GraphDriver.fail)throw GraphDriver.error;}abstract Object observe();}
class GraphDriver {static String trace="",callback="";static boolean fail;static final RuntimeException error=new RuntimeException("identity");static int mark(int n){trace+=n+",";return n;}
public static void main(String[]args)throws Exception{int nodes=NODES;Object token=new Object();Object[] values=new Object[nodes];String expected="";for(int i=0;i<nodes;i++){String name="GraphScope"+i;Class<?> owner=Class.forName(name,false,GraphDriver.class.getClassLoader());Class<?> type=Class.forName(name+"$Marker",false,GraphDriver.class.getClassLoader());if(type.getDeclaringClass()!=owner)throw new AssertionError("declaring identity");java.lang.reflect.Constructor<?> ctor=type.getDeclaredConstructor(Object.class);ctor.setAccessible(true);values[i]=ctor.newInstance(token);if(type.getDeclaredField("token").get(values[i])!=token||type.getDeclaredField("initialized").getInt(null)!=i)throw new AssertionError("capture/integer field");expected+=i+",";if(!trace.equals(expected))throw new AssertionError("lazy initialization:"+trace);if(i>0){java.lang.reflect.Field next=values[i-1].getClass().getDeclaredField("next");if(next.getType()!=type)throw new AssertionError("bound declaration type");next.set(values[i-1],values[i]);if(next.get(values[i-1])!=values[i])throw new AssertionError("link identity");}}
CYCLE
int rows=0;for(Object value:new Object[]{null,token})for(boolean abrupt:new boolean[]{false,true}){GraphScope0 owner=new GraphScope0(value);callback="";fail=abrupt;try{GraphScope0.Capture captured=owner.make();if(abrupt||captured.observed!=value||captured.getClass().getDeclaringClass()!=GraphScope0.class)throw new AssertionError("preSUPER capture");}catch(RuntimeException e){if(!abrupt||e!=error)throw new AssertionError("failure identity",e);}if(!callback.equals("P"))throw new AssertionError("callback order");rows++;}System.out.println(nodes+":"+rows+":wide-graph:owners:identity:init:callback:failure");}}
`)
	s := strings.ReplaceAll(source.String(), "NODES", fmt.Sprint(nodes))
	check := ""
	if cycle {
		check = `java.lang.reflect.Field back=values[1].getClass().getDeclaredField("back");if(back.getType()!=values[0].getClass())throw new AssertionError("cycle type");back.set(values[1],values[0]);if(back.get(values[1])!=values[0])throw new AssertionError("cycle identity");`
	}
	s = strings.ReplaceAll(s, "CYCLE", check)
	return s, owners
}
func TestAdversarialWideIndependentSourceDependencyRoundTrip(t *testing.T) {
	fixture, owners := wideSourceDependencyFixture(96, false)
	for _, rename := range []bool{false, true} {
		t.Run(fmt.Sprint(rename), func(t *testing.T) {
			f := fixture
			names := append([]string(nil), owners...)
			if rename {
				f = strings.ReplaceAll(f, "GraphScope", "IndependentDeclaration")
				for i := range names {
					names[i] = strings.ReplaceAll(names[i], "GraphScope", "IndependentDeclaration")
				}
			}
			testNativeIndependentFamilyFixture(t, f, names, "GraphDriver", "96:4:wide-graph:owners:identity:init:callback:failure\n", nativeLexicalExactSignatures)
		})
	}
}
func TestAdversarialWideCondensationSourceDependencyRoundTrip(t *testing.T) {
	fixture, owners := wideSourceDependencyFixture(96, true)
	testNativeIndependentFamilyFixture(t, fixture, owners, "GraphDriver", "96:4:wide-graph:owners:identity:init:callback:failure\n", nativeLexicalExactSignatures)
}
