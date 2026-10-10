package javaclassparser

import (
	"strings"
	"testing"
)

const nativeAnonymousBranchOrderFixture = `class BranchChoiceEffects {static String trace="";static int fail;static final RuntimeException error=new RuntimeException("original");}
class BranchChoiceCost {final long value;final String event;BranchChoiceCost(long v,String e){value=v;event=e;}long cost(){BranchChoiceEffects.trace+=event;if(BranchChoiceEffects.fail==1)throw BranchChoiceEffects.error;return value;}}
abstract class BranchChoiceParent {final Object seed;BranchChoiceParent(Object seed){BranchChoiceEffects.trace+="P";this.seed=java.util.Objects.requireNonNull(seed);}abstract int selected();abstract double captured();}
class BranchChoiceOwner {
 final BranchChoiceCost left,right;BranchChoiceOwner(BranchChoiceCost l,BranchChoiceCost r){left=l;right=r;}
 BranchChoiceParent make(Object seed,double captured){if(left==null||(right!=null&&left.cost()<=right.cost())){return new BranchChoiceParent(seed){int selected(){return 1;}double captured(){return captured;}};}else{return new BranchChoiceParent(seed){int selected(){return 2;}double captured(){return captured;}};}}
}
class BranchChoiceDriver {public static void main(String[]args){Object token=new Object();int rows=0;for(long l:new long[]{-1,0,1})for(long r:new long[]{-1,0,1})for(int mask=0;mask<4;mask++)for(double v:new double[]{-0.0,Double.NaN,Double.POSITIVE_INFINITY}){BranchChoiceOwner owner=new BranchChoiceOwner((mask&1)==0?new BranchChoiceCost(l,"L"):null,(mask&2)==0?new BranchChoiceCost(r,"R"):null);int selected=owner.left==null||(owner.right!=null&&l<=r)?1:2;String before=owner.left!=null&&owner.right!=null?"LR":"";BranchChoiceEffects.trace="";BranchChoiceParent p=owner.make(token,v);if(p.seed!=token||p.selected()!=selected||Double.doubleToRawLongBits(p.captured())!=Double.doubleToRawLongBits(v)||!p.getClass().getName().equals("BranchChoiceOwner$"+selected)||p.getClass().getEnclosingClass()!=BranchChoiceOwner.class||!p.getClass().getEnclosingMethod().getName().equals("make")||!BranchChoiceEffects.trace.equals(before+"P"))throw new AssertionError("ordinal/condition/capture/order/owner");rows++;BranchChoiceEffects.trace="";try{owner.make(null,v);throw new AssertionError("missing null failure");}catch(NullPointerException e){if(!BranchChoiceEffects.trace.equals(before+"P"))throw new AssertionError("null failure order");}}BranchChoiceOwner owner=new BranchChoiceOwner(new BranchChoiceCost(0,"L"),new BranchChoiceCost(1,"R"));BranchChoiceEffects.fail=1;BranchChoiceEffects.trace="";try{owner.make(token,0);throw new AssertionError("missing condition failure");}catch(RuntimeException e){if(e!=BranchChoiceEffects.error||!BranchChoiceEffects.trace.equals("L"))throw new AssertionError("condition failure identity/order");}System.out.println(rows+":branch:ordinal:identity:order:owner");}}
`

func TestNativeAnonymousConditionalOrderKeepsOriginalClassIdentity(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeAnonymousBranchOrderFixture, "BranchChoiceOwner", "BranchChoiceDriver", "108:branch:ordinal:identity:order:owner\n")
}

func TestNativeAnonymousConditionalOrderIgnoresSourceSpelling(t *testing.T) {
	fixture := strings.ReplaceAll(nativeAnonymousBranchOrderFixture, "BranchChoiceOwner", "IndependentChoiceScope")
	fixture = strings.ReplaceAll(fixture, "captured", "savedValue")
	testNativePrivateSetterFixture(t, fixture, "IndependentChoiceScope", "BranchChoiceDriver", "108:branch:ordinal:identity:order:owner\n")
}

func TestNativeAnonymousNestedConditionalOrderKeepsEveryIdentity(t *testing.T) {
	fixture := strings.Replace(nativeAnonymousBranchOrderFixture, `if(left==null||(right!=null&&left.cost()<=right.cost())){return new BranchChoiceParent(seed){int selected(){return 1;}double captured(){return captured;}};}else{return new BranchChoiceParent(seed){int selected(){return 2;}double captured(){return captured;}};}`, `if(left==null){return new BranchChoiceParent(seed){int selected(){return 1;}double captured(){return captured;}};}else if(right==null||left.cost()<=right.cost()){return new BranchChoiceParent(seed){int selected(){return 2;}double captured(){return captured;}};}else{return new BranchChoiceParent(seed){int selected(){return 3;}double captured(){return captured;}};}`, 1)
	fixture = strings.Replace(fixture, `int selected=owner.left==null||(owner.right!=null&&l<=r)?1:2;`, `int selected=owner.left==null?1:owner.right==null||l<=r?2:3;`, 1)
	testNativePrivateSetterFixture(t, fixture, "BranchChoiceOwner", "BranchChoiceDriver", "108:branch:ordinal:identity:order:owner\n")
}

func TestNativeAnonymousTernaryOrderKeepsOriginalClassIdentity(t *testing.T) {
	fixture := strings.Replace(nativeAnonymousBranchOrderFixture, `if(left==null||(right!=null&&left.cost()<=right.cost())){return new BranchChoiceParent(seed){int selected(){return 1;}double captured(){return captured;}};}else{return new BranchChoiceParent(seed){int selected(){return 2;}double captured(){return captured;}};}`, `return (left==null||(right!=null&&left.cost()<=right.cost()))?new BranchChoiceParent(seed){int selected(){return 1;}double captured(){return captured;}}:new BranchChoiceParent(seed){int selected(){return 2;}double captured(){return captured;}};`, 1)
	testNativePrivateSetterFixture(t, fixture, "BranchChoiceOwner", "BranchChoiceDriver", "108:branch:ordinal:identity:order:owner\n")
}
