package javaclassparser

import "testing"

func TestAdversarialTernaryGenericRegistryKeepsCompatibleArmAndLazyFactoryRoundTrip(t *testing.T) {
	fixture := `interface RegistryLookup<E>{E lookup(String key);}
class RegistryEntry{final int value;RegistryEntry(int value){this.value=value;}}
class RegistryProduct<E> implements RegistryLookup<E>{private final E value;RegistryProduct(E value){this.value=value;}public E lookup(String key){return value;}}
class RegistryProducer<E>{private E value;static int creations;static <E>RegistryProducer<E> create(){creations++;return new RegistryProducer<E>();}RegistryProducer<E> register(String key,E value){this.value=value;return this;}RegistryProduct<E> build(){return new RegistryProduct<E>(value);}}
public class TernaryRegistryOwner{
 private final RegistryLookup<RegistryEntry> registry;
 public TernaryRegistryOwner(RegistryLookup<RegistryEntry> provided){registry=provided!=null?provided:RegistryProducer.<RegistryEntry>create().register("value",new RegistryEntry(31)).build();}
 public RegistryLookup<RegistryEntry> registry(){return registry;}
 public RegistryEntry get(){return registry.lookup("value");}
}
class TernaryRegistryCheck{public static void main(String[] args){int rows=0;
 RegistryEntry token=new RegistryEntry(17);RegistryLookup<RegistryEntry> provided=new RegistryProduct<RegistryEntry>(token);
 RegistryProducer.creations=0;TernaryRegistryOwner original=new TernaryRegistryOwner(provided);if(original.registry()!=provided||original.get()!=token||RegistryProducer.creations!=0)throw new AssertionError("compatible arm / identity / lazy factory");rows++;
 TernaryRegistryOwner generated=new TernaryRegistryOwner(null);if(generated.get().value!=31||RegistryProducer.creations!=1)throw new AssertionError("generic factory arm / count / value");rows++;
 RegistryLookup polluted=new RegistryProduct<String>("wrong payload");TernaryRegistryOwner raw=new TernaryRegistryOwner(polluted);if(raw.registry()!=polluted||RegistryProducer.creations!=1)throw new AssertionError("raw identity / lazy factory");try{raw.get();throw new AssertionError("lost original payload check");}catch(ClassCastException expected){rows++;}
 System.out.println(rows+":ternary:generic:registry:lazy:identity:payload-check");
}}`
	testNativePrivateSetterSourceFixture(t, map[string]string{"TernaryRegistryOwner.java": fixture}, "TernaryRegistryOwner", "TernaryRegistryCheck", "3:ternary:generic:registry:lazy:identity:payload-check\n")
}
