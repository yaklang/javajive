package javaclassparser

import "testing"

// A class formal's erasure belongs to the original declaring Signature, not
// an inferred receiver argument. The original caller checks both arms and the
// original payload check without directing generic receiver inference.
func TestAdversarialClassFormalConditionalResultPreservesErasedFieldStoreRoundTrip(t *testing.T) {
	fixture := `interface FormalLookup<E>{E lookup();}
class FormalEntry{final int value;FormalEntry(int n){value=n;}}
class FormalProduct<E> implements FormalLookup<E>{E value;FormalProduct(E v){value=v;}public E lookup(){return value;}}
class FormalProducer<E,R extends FormalLookup<E>>{
 final R result;static int creations,steps;FormalProducer(R r){result=r;}
 static <E>FormalProducer<E,FormalProduct<E>> create(){creations++;return new FormalProducer<E,FormalProduct<E>>(new FormalProduct<E>(null));}
 FormalProducer<E,R> register(E value){((FormalProduct<E>)result).value=value;return this;}
 FormalProducer<E,R> step(){steps++;return this;}
 R build(){return result;}
}
public class FormalResultOwner{
 private final FormalLookup<FormalEntry> registry;
 public FormalResultOwner(FormalLookup<FormalEntry> provided){registry=provided!=null?provided:FormalProducer.<FormalEntry>create().register(new FormalEntry(31)).step().build();}
 public FormalLookup<FormalEntry> registry(){return registry;}public FormalEntry get(){return registry.lookup();}
}
class FormalResultDriver{public static void main(String[] args){int rows=0;FormalEntry token=new FormalEntry(17);FormalLookup<FormalEntry> provided=new FormalProduct<FormalEntry>(token);FormalProducer.creations=0;FormalProducer.steps=0;
 FormalResultOwner first=new FormalResultOwner(provided);if(first.registry()!=provided||first.get()!=token||FormalProducer.creations!=0||FormalProducer.steps!=0)throw new AssertionError("provided identity/lazy receiver");rows++;
 FormalResultOwner made=new FormalResultOwner(null);if(made.get().value!=31||FormalProducer.creations!=1||FormalProducer.steps!=1)throw new AssertionError("class formal / generic receiver / factory");rows++;
 FormalLookup polluted=new FormalProduct<String>("polluted");FormalResultOwner raw=new FormalResultOwner(polluted);if(raw.registry()!=polluted||FormalProducer.creations!=1||FormalProducer.steps!=1)throw new AssertionError("raw identity / lazy receiver");try{raw.get();throw new AssertionError("lost original payload check");}catch(ClassCastException expected){rows++;}
 System.out.println(rows+":class-formal:result:identity:lazy:payload-check");}}
`
	testNativePrivateSetterSourceFixture(t, map[string]string{"FormalResultOwner.java": fixture}, "FormalResultOwner", "FormalResultDriver", "3:class-formal:result:identity:lazy:payload-check\n")
}
