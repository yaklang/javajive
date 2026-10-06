package javaclassparser

import (
	"strings"
	"testing"
)

// Both inner implementations are replaced. The independent original JVM
// checks accepted/rejected keys and identity; equal-spelled caller variables
// alone are never declaration evidence.
func TestAdversarialFlattenedLexicalReceiverBindingRoundTrip(t *testing.T) {
	for _, root := range []string{"LexicalRangeOwner", "RenamedRangeOwner"} {
		t.Run(root, func(t *testing.T) {
			source := `class LexicalRangeOwner<K,V>{class Range{K accepted;V value;Range(K k,V v){accepted=k;value=v;}boolean inRange(K k){return k==accepted;}V get(){return value;}}class Consumer{final Range range;Consumer(Range r){range=r;}boolean contains(Object key){return range.inRange((K)key);}V value(){return range.get();}}}
class LexicalRangeDriver{public static void main(String[]args){int count=0;Object[] keys={null,"key",new Object()};Object[] values={null,"value",new Object()};for(Object k:keys)for(Object v:values){LexicalRangeOwner<Object,Object> owner=new LexicalRangeOwner<Object,Object>();LexicalRangeOwner<Object,Object>.Range range=owner.new Range(k,v);LexicalRangeOwner<Object,Object>.Consumer consumer=owner.new Consumer(range);if(!consumer.contains(k)||consumer.contains(new Object())||consumer.value()!=v)throw new AssertionError("declaration receiver identity/binding");count++;}System.out.println(count+":lexical:flat:binding");}}`
			source = strings.ReplaceAll(source, "LexicalRangeOwner", root)
			roundTripGenericFlowUnitsClasspath(t, "LexicalRangeDriver", source, nil, []string{root, root + "$Range", root + "$Consumer"}, true, Precision, Compatibility, "legacy")
		})
	}
}
