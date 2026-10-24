package javaclassparser

import "testing"

const sourceLayoutGenericFixture = `import java.util.function.IntUnaryOperator;
abstract class LayoutParent<T>{final T seen;LayoutParent(){seen=read();}abstract T read();}
class LayoutGood<T>{private final T value;LayoutGood(T value){this.value=value;}static class Token{}final class Leaf extends LayoutParent<T>{final LayoutOpaque.Token<T> peer;Leaf(LayoutOpaque.Token<T> peer){this.peer=peer;}T read(){return LayoutGood.this.value;}T saved(){return peer.value;}}Leaf make(LayoutOpaque.Token<T> peer){return new Leaf(peer);}}
class LayoutOpaque{LayoutGood.Token peer;static class Token<U>{final U value;Token(U value){this.value=value;}}static IntUnaryOperator unrelated(final String text){return n->n+text.length();}}
class LayoutDriver{public static void main(String[]args){int rows=0;String[] same=new String[]{"same"};String[][] values={null,same,new String[0],new String[]{"same"}};for(String[] left:values)for(String[] right:values){LayoutGood<String[]> good=new LayoutGood<String[]>(left);LayoutOpaque.Token<String[]> peer=new LayoutOpaque.Token<String[]>(right);LayoutGood<String[]>.Leaf leaf=good.make(peer);if(leaf.seen!=left||leaf.read()!=left||leaf.saved()!=right||leaf.peer!=peer||leaf.getClass().getDeclaringClass()!=LayoutGood.class)throw new AssertionError("generic binder or original identity");LayoutGood<String[]>.Leaf nil=good.make(null);if(nil.seen!=left||nil.read()!=left||nil.peer!=null)throw new AssertionError("generic null peer construction");try{nil.saved();throw new AssertionError("missing null peer check");}catch(NullPointerException wanted){}if(LayoutOpaque.unrelated("abc").applyAsInt(7)!=10)throw new AssertionError("unchanged original generic peer body");rows++;}System.out.println(rows+":source:layout:generic");}}
`

func TestAdversarialSourceLayoutSubsetPreservesIndependentGenericBinders(t *testing.T) {
	testSourceLayoutSubset(t, sourceLayoutGenericFixture, "LayoutGood", "LayoutOpaque", "LayoutDriver", "16:source:layout:generic\n")
}
