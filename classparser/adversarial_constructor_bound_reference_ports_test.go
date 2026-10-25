package javaclassparser

import (
	"os"
	"strings"
	"testing"
)

const constructorBoundReferencePortsFixture = `
import java.util.function.Supplier;
class PairEffects {static String trace="";static int mode;static final IllegalStateException failure=new IllegalStateException("identity");}
class PairProducer {final String name;final Object token;PairProducer(String name,Object token){this.name=name;this.token=token;}public Object get(){PairEffects.trace+=name;if(PairEffects.mode==2)throw PairEffects.failure;return token;}}
class PairParent<V,P> {final V token;final P owner;final Object a,b;final long bits;
 PairParent(V token,P owner,Supplier<Object> a,Supplier<Object> b,long bits){PairEffects.trace+="P";if(PairEffects.mode==1)throw PairEffects.failure;this.token=token;this.owner=owner;this.a=a.get();this.b=b.get();this.bits=bits;}}
public class BoundPairOwner<V> extends PairParent<V,PairProducer> {
 public BoundPairOwner(V token,PairProducer left,PairProducer right,long bits){super(token,left,left::get,right::get,bits);}}
class BridgeDriver {public static void main(String[]args)throws Exception {
 Object token=new Object(),leftValue=new Object(),rightValue=new Object();PairProducer left=new PairProducer("L",leftValue),right=new PairProducer("R",rightValue);int rows=0;
 if(BoundPairOwner.class.getDeclaredMethods().length!=0||BoundPairOwner.class.getDeclaredConstructors().length!=1)throw new AssertionError("no temporary helper declarations");
 for(PairProducer a:new PairProducer[]{null,left})for(PairProducer b:new PairProducer[]{null,left,right})for(int mode=0;mode<3;mode++)for(long bits:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE}){
 PairEffects.trace="";PairEffects.mode=mode;Throwable caught=null;BoundPairOwner<Object> value=null;
 try{value=new BoundPairOwner<Object>(token,a,b,bits);}catch(Throwable error){caught=error;}
 String trace=a==null||b==null?"":mode==1?"P":mode==2?"PL":"PL"+b.name;
 if(!PairEffects.trace.equals(trace))throw new AssertionError("reference creation before constructor body: "+PairEffects.trace,caught);
 if(a==null||b==null){if(!(caught instanceof NullPointerException))throw new AssertionError("eager null input",caught);}
 else if(mode!=0){if(caught!=PairEffects.failure)throw new AssertionError("exception identity",caught);}
 else if(caught!=null||value.token!=token||value.owner!=a||value.a!=a.token||value.b!=b.token||value.bits!=bits)throw new AssertionError("distinct argument/capture identities",caught);rows++;
 }System.out.println(rows+":constructor:reference:ports:order:identity");}}
`

func TestAdversarialConstructorMultipleBoundReferencesKeepArgumentPorts(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("original native compiler required")
	}
	for _, root := range []string{"BoundPairOwner", "RenamedPairScope"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(root+"/"+debug, func(t *testing.T) {
				fixture := strings.ReplaceAll(constructorBoundReferencePortsFixture, "BoundPairOwner", root)
				testIndependentInheritedBridgeOwner(t, modern, java, native, fixture, root, root, debug, true, "54:constructor:reference:ports:order:identity\n", nil)
			})
		}
	}
}
