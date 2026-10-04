package javaclassparser

import "testing"

func TestAdversarialConstructorErasedFieldOriginalOverload(t *testing.T) {
	roundTripGenericFlowUnitsClasspath(t, "GenericFieldReviewDriver", `
class GenericFieldReviewBox<T> {final T value;GenericFieldReviewBox(T v){value=v;}}
class GenericFieldReviewParent {final Object value;final int tag;GenericFieldReviewParent(Object v){value=v;tag=1;}GenericFieldReviewParent(String v){value=v;tag=2;}Object capture(){return null;}}
class GenericFieldReviewInheritedBox extends GenericFieldReviewBox<String> {GenericFieldReviewInheritedBox(String v){super(v);}}
class GenericFieldReviewInheritedChild extends GenericFieldReviewParent {GenericFieldReviewInheritedChild(GenericFieldReviewInheritedBox b){super((Object)b.value);}}
class GenericFieldReviewProducer {static int calls;static <T> T identity(T value){calls++;return value;}}
class GenericFieldReviewReturnChild extends GenericFieldReviewParent {GenericFieldReviewReturnChild(String value){super((Object)GenericFieldReviewProducer.<String>identity(value));}}
class GenericFieldReviewOwner {final Object token;GenericFieldReviewOwner(Object t){token=t;}final class Member extends GenericFieldReviewParent {Member(GenericFieldReviewBox<String> box){super((Object)box.value);}Object capture(){return GenericFieldReviewOwner.this.token;}}Member make(GenericFieldReviewBox<String> box){return new Member(box);}}
class GenericFieldReviewOracle {static void run(){Object token=new Object();GenericFieldReviewOwner owner=new GenericFieldReviewOwner(token);for(String value:new String[]{null,"text"}){GenericFieldReviewParent out=owner.make(new GenericFieldReviewBox<String>(value));if(out.value!=value||out.tag!=1||out.capture()!=token)throw new AssertionError("original erased field overload/capture");GenericFieldReviewParent inherited=new GenericFieldReviewInheritedChild(new GenericFieldReviewInheritedBox(value));int before=GenericFieldReviewProducer.calls;GenericFieldReviewParent returned=new GenericFieldReviewReturnChild(value);if(inherited.value!=value||returned.value!=value||inherited.tag!=1||returned.tag!=1||GenericFieldReviewProducer.calls!=before+1)throw new AssertionError("inherited field/producer binding and count");System.out.println((value==null)+":"+out.tag+":"+inherited.tag+":"+returned.tag);}try{owner.make(null);throw new AssertionError("missing null failure");}catch(NullPointerException expected){System.out.println("null");}}}
public class GenericFieldReviewDriver {public static void main(String[]args){GenericFieldReviewOracle.run();}}
`, nil, []string{"GenericFieldReviewOwner", "GenericFieldReviewOwner$Member", "GenericFieldReviewInheritedChild", "GenericFieldReviewReturnChild"}, true, Precision, Compatibility, "legacy")
}
