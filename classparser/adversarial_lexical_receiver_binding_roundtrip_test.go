package javaclassparser

import (
	"strings"
	"testing"
)

const lexicalReceiverBindingFixture = `class BindingEffects {static String trace="";static boolean fail;static final RuntimeException error=new RuntimeException("identity");}
class BindingOwner<K> {
 final K token;BindingOwner(K token){this.token=token;}
 class Gate {boolean accepts(K value){BindingEffects.trace+="G";if(BindingEffects.fail)throw BindingEffects.error;return value==token;}}
 class Reader {final BindingOwner<K>.Gate delegate;Reader(Gate delegate){this.delegate=delegate;}boolean contains(Object value){BindingEffects.trace+="R";return delegate.accepts((K)value);}}
 Gate gate(){return new Gate();}Reader reader(Gate gate){return new Reader(gate);}
}
class BindingDriver {public static void main(String[]args)throws Exception{Object identity=new Object();int rows=0;for(Object token:new Object[]{null,identity,"text"}){BindingOwner<Object> owner=new BindingOwner<Object>(token);BindingOwner<Object>.Reader reader=owner.reader(owner.gate());for(Object value:new Object[]{null,identity,"text",new Object()})for(boolean fail:new boolean[]{false,true}){BindingEffects.trace="";BindingEffects.fail=fail;try{boolean got=reader.contains(value);if(fail||got!=(value==token))throw new AssertionError("identity");}catch(RuntimeException e){if(!fail||e!=BindingEffects.error)throw new AssertionError("failure",e);}if(!BindingEffects.trace.equals("RG"))throw new AssertionError("evaluation order");rows++;}}BindingEffects.trace="";BindingEffects.fail=false;BindingOwner<Object> owner=new BindingOwner<Object>(identity);try{owner.reader(null).contains(identity);throw new AssertionError("null receiver");}catch(NullPointerException e){if(!BindingEffects.trace.equals("R"))throw new AssertionError("null timing");}System.out.println(rows+":lexical:receiver:identity:failure:order");}}`

func TestAdversarialLexicalReceiverOuterBindingRoundTrip(t *testing.T) {
	testSourceTargetReleaseFamilyFixture(t, lexicalReceiverBindingFixture, "BindingOwner", "BindingDriver", "24:lexical:receiver:identity:failure:order\n", "8", []int{8, 11})
}

// Leaf and outer formals occupy separate segments; the erased descriptor is
// Object for both, so a positional merge can silently choose the wrong binder.
func TestAdversarialLexicalReceiverOwnAndOuterBindingRoundTrip(t *testing.T) {
	fixture := strings.ReplaceAll(lexicalReceiverBindingFixture, "class Gate {", "class Gate<V> {")
	fixture = strings.ReplaceAll(fixture, "class Reader {", "class Reader<V> {")
	fixture = strings.ReplaceAll(fixture, "BindingOwner<K>.Gate delegate", "BindingOwner<K>.Gate<V> delegate")
	fixture = strings.ReplaceAll(fixture, "Reader(Gate delegate)", "Reader(Gate<V> delegate)")
	fixture = strings.ReplaceAll(fixture, "Gate gate(){return new Gate();}Reader reader(Gate gate){return new Reader(gate);}", "Gate<String> gate(){return new Gate<String>();}Reader<String> reader(Gate<String> gate){return new Reader<String>(gate);}")
	fixture = strings.ReplaceAll(fixture, "BindingOwner<Object>.Reader reader", "BindingOwner<Object>.Reader<String> reader")
	testSourceTargetReleaseFamilyFixture(t, fixture, "BindingOwner", "BindingDriver", "24:lexical:receiver:identity:failure:order\n", "8", []int{8, 11})
}
func TestAdversarialLexicalRawReceiverPreservesErasedBindingRoundTrip(t *testing.T) {
	fixture := strings.ReplaceAll(lexicalReceiverBindingFixture, "BindingOwner<K>.Gate delegate", "BindingOwner.Gate delegate")
	fixture = strings.ReplaceAll(fixture, "delegate.accepts((K)value)", "delegate.accepts(value)")
	testSourceTargetReleaseFamilyFixture(t, fixture, "BindingOwner", "BindingDriver", "24:lexical:receiver:identity:failure:order\n", "8", []int{8, 11})
}

const segmentedReceiverBindingFixture = `class SegmentedEffects {static String trace="";static boolean fail;static final RuntimeException error=new RuntimeException("identity");}
class SegmentedOwner<K> {final K key;SegmentedOwner(K key){this.key=key;}
 class Layer<V> {final V value;Layer(V value){this.value=value;}
  class Gate<W> {final W word;Gate(W word){this.word=word;}boolean accepts(K a,V b,W c){SegmentedEffects.trace+="G";if(SegmentedEffects.fail)throw SegmentedEffects.error;return a==key&&b==value&&c==word;}}
  class Reader<W> {final SegmentedOwner<K>.Layer<V>.Gate<W> gate;Reader(Gate<W> gate){this.gate=gate;}boolean contains(Object a,Object b,Object c){SegmentedEffects.trace+="R";return gate.accepts((K)a,(V)b,(W)c);}}
  Gate<Object> gate(Object word){return new Gate<Object>(word);}Reader<Object> reader(Gate<Object> gate){return new Reader<Object>(gate);}}
 Layer<Object> layer(Object value){return new Layer<Object>(value);}}
class SegmentedDriver {public static void main(String[]args)throws Exception{Object a=new Object(),b=new Object(),c=new Object();SegmentedOwner<Object> owner=new SegmentedOwner<Object>(a);SegmentedOwner<Object>.Layer<Object> layer=owner.layer(b);SegmentedOwner<Object>.Layer<Object>.Reader<Object> reader=layer.reader(layer.gate(c));int rows=0;for(Object x:new Object[]{null,a,b})for(Object y:new Object[]{null,b,c})for(Object z:new Object[]{null,c,a})for(boolean fail:new boolean[]{false,true}){SegmentedEffects.trace="";SegmentedEffects.fail=fail;try{boolean got=reader.contains(x,y,z);if(fail||got!=(x==a&&y==b&&z==c))throw new AssertionError("segment/object identity");}catch(RuntimeException e){if(!fail||e!=SegmentedEffects.error)throw new AssertionError("failure identity",e);}if(!SegmentedEffects.trace.equals("RG"))throw new AssertionError("order");rows++;}System.out.println(rows+":three-segment:binding:identity:failure");}}`

func TestAdversarialLexicalReceiverThreeSegmentBindingRoundTrip(t *testing.T) {
	testSourceTargetReleaseFamilyFixture(t, segmentedReceiverBindingFixture, "SegmentedOwner", "SegmentedDriver", "54:three-segment:binding:identity:failure\n", "8", []int{8, 11})
}

func TestAdversarialModernLexicalReceiverOuterBindingRoundTrip(t *testing.T) {
	testSourceTargetReleaseFamilyFixture(t, lexicalReceiverBindingFixture, "BindingOwner", "BindingDriver", "24:lexical:receiver:identity:failure:order\n", "11", []int{11, 16})
}

func TestAdversarialLexicalReceiverInheritedBindingRoundTrip(t *testing.T) {
	fixture := strings.Replace(lexicalReceiverBindingFixture, "class Gate {boolean accepts(K value){BindingEffects.trace+=\"G\";if(BindingEffects.fail)throw BindingEffects.error;return value==token;}}", "class Gate extends BindingParent<K> {Gate(){super(BindingOwner.this.token);}}", 1)
	fixture += `class BindingParent<T>{final T token;BindingParent(T token){this.token=token;}boolean accepts(T value){BindingEffects.trace+="G";if(BindingEffects.fail)throw BindingEffects.error;return value==token;}}`
	testSourceTargetReleaseFamilyFixture(t, fixture, "BindingOwner", "BindingDriver", "24:lexical:receiver:identity:failure:order\n", "8", []int{8, 11})
}
