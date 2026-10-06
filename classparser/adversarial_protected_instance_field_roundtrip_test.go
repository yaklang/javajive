package javaclassparser

import (
	"strings"
	"testing"
)

func nativeProtectedInstanceFieldSources(root string) map[string]string {
	parent := `package instance.base;public class InstanceParent{protected Object value;protected volatile long number;protected double bits;public InstanceParent(Object x,long n,double d){value=x;number=n;bits=d;}public void set(Object x,long n,double d){value=x;number=n;bits=d;}}`
	middle := `package instance.base;public class InstanceMiddle extends InstanceParent{public InstanceMiddle(Object x,long n,double d){super(x,n,d);}}`
	owner := `package instance.use;public class InstanceFieldOwner{public class Child extends instance.base.InstanceMiddle{public Child(Object x,long n,double d){super(x,n,d);}public class Reader{private Object value=new Object();public Object read(Child c){return c.value;}public Object readAgain(Child c){return c.value;}public long number(Child c){return c.number;}public double bits(Child c){return c.bits;}}}}
 class InstanceFieldDriver{static String trace="";static InstanceFieldOwner.Child choose(InstanceFieldOwner.Child c){trace+="R";return c;}public static void main(String[]a){InstanceFieldOwner one=new InstanceFieldOwner(),two=new InstanceFieldOwner();Object token=new Object(),other=new Object();InstanceFieldOwner.Child first=one.new Child(token,0,0),second=two.new Child(other,0,0);InstanceFieldOwner.Child.Reader r=first.new Reader();if(r.getClass().getDeclaringClass()!=InstanceFieldOwner.Child.class||first.getClass().getDeclaringClass()!=InstanceFieldOwner.class)throw new AssertionError("owners");int rows=0;for(InstanceFieldOwner.Child c:new InstanceFieldOwner.Child[]{first,second})for(Object x:new Object[]{null,token,other})for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){double d=Double.longBitsToDouble(n);c.set(x,n,d);trace="";if(r.read(choose(c))!=x||r.readAgain(c)!=x||r.number(c)!=n||Double.doubleToRawLongBits(r.bits(c))!=n||!trace.equals("R"))throw new AssertionError("receiver/volatile/category/payload/order");rows++;}trace="";try{r.read(choose(null));throw new AssertionError("missing null receiver");}catch(NullPointerException e){if(!trace.equals("R"))throw new AssertionError("receiver effects lost");}System.out.println(rows+":protected:instance:field:binding:receiver:null:width:identity");}}`
	owner = strings.ReplaceAll(owner, "InstanceFieldOwner", root)
	return map[string]string{"instance/base/InstanceParent.java": parent, "instance/base/InstanceMiddle.java": middle, "instance/use/" + root + ".java": owner}
}
func TestAdversarialProtectedInstanceFieldNestedOwnerRoundTrip(t *testing.T) {
	testNativePrivateSetterSourceFixture(t, nativeProtectedInstanceFieldSources("InstanceFieldOwner"), "instance/use/InstanceFieldOwner", "instance.use.InstanceFieldDriver", "30:protected:instance:field:binding:receiver:null:width:identity\n")
}
func TestAdversarialProtectedInstanceFieldRenamedRoundTrip(t *testing.T) {
	testNativePrivateSetterSourceFixture(t, nativeProtectedInstanceFieldSources("ChangedReceiverOwner"), "instance/use/ChangedReceiverOwner", "instance.use.InstanceFieldDriver", "30:protected:instance:field:binding:receiver:null:width:identity\n")
}

// The resolver-aware eligibility path also admits the existing protected call
// packet inside a nonstatic member. The override in the intermediate original
// declaration must remain the virtual target; receiver and wide arguments run once.
func nativeProtectedNestedCallSources(root string) map[string]string {
	parent := `package nested.base;public class CallParent{protected Object call(Object x,long n){Trace.value+="P";return x;}}`
	middle := `package nested.base;public class CallMiddle extends CallParent{@Override protected Object call(Object x,long n){Trace.value+="M";return x;}}`
	trace := `package nested.base;public class Trace{public static String value="";}`
	owner := `package nested.use;public class CallOwner{public class Child extends nested.base.CallMiddle{public class Reader{private Object call(Object x,long n){throw new AssertionError("lexical overload");}public Object read(Child c,Object x,long n){return c.call(x,n);}}}}
class NestedCallDriver{static CallOwner.Child choose(CallOwner.Child c){nested.base.Trace.value+="R";return c;}static long chooseLong(long n){nested.base.Trace.value+="L";return n;}public static void main(String[]a){CallOwner one=new CallOwner(),two=new CallOwner();CallOwner.Child first=one.new Child(),second=two.new Child();CallOwner.Child.Reader r=first.new Reader();Object token=new Object(),other=new Object();int rows=0;for(CallOwner.Child c:new CallOwner.Child[]{first,second})for(Object x:new Object[]{null,token,other})for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){nested.base.Trace.value="";if(r.read(choose(c),x,chooseLong(n))!=x||!nested.base.Trace.value.equals("RLM"))throw new AssertionError("dispatch/receiver/arguments/identity");rows++;}nested.base.Trace.value="";try{r.read(choose(null),token,chooseLong(0));throw new AssertionError("null receiver");}catch(NullPointerException e){if(!nested.base.Trace.value.equals("RL"))throw new AssertionError("null argument order");}System.out.println(rows+":protected:nested:virtual:receiver:wide:identity");}}`
	owner = strings.ReplaceAll(owner, "CallOwner", root)
	return map[string]string{"nested/base/CallParent.java": parent, "nested/base/CallMiddle.java": middle, "nested/base/Trace.java": trace, "nested/use/" + root + ".java": owner}
}
func TestAdversarialProtectedNestedVirtualCallRoundTrip(t *testing.T) {
	testNativePrivateSetterSourceFixture(t, nativeProtectedNestedCallSources("CallOwner"), "nested/use/CallOwner", "nested.use.NestedCallDriver", "30:protected:nested:virtual:receiver:wide:identity\n")
}
func TestAdversarialProtectedNestedVirtualCallRenamedRoundTrip(t *testing.T) {
	testNativePrivateSetterSourceFixture(t, nativeProtectedNestedCallSources("AlteredCallOwner"), "nested/use/AlteredCallOwner", "nested.use.NestedCallDriver", "30:protected:nested:virtual:receiver:wide:identity\n")
}

func TestAdversarialProtectedInstanceClosedGenericFieldRoundTrip(t *testing.T) {
	sources := nativeProtectedInstanceFieldSources("ClosedFieldOwner")
	for _, file := range []string{"instance/base/InstanceParent.java", "instance/base/InstanceMiddle.java"} {
		sources[file] = strings.ReplaceAll(sources[file], "Object", "java.util.List<?>")
	}
	file := "instance/use/ClosedFieldOwner.java"
	sources[file] = strings.ReplaceAll(sources[file], "public Child(Object x,long n,double d)", "public Child(java.util.List<?> x,long n,double d)")
	sources[file] = strings.ReplaceAll(sources[file], "Object token=new Object(),other=new Object();", `java.util.List<?> token=java.util.Collections.singletonList("first"),other=java.util.Collections.singletonList("second");`)
	sources[file] = strings.ReplaceAll(sources[file], "for(Object x:new Object[]{null,token,other})", "for(java.util.List<?> x:new java.util.List<?>[]{null,token,other})")
	testNativePrivateSetterSourceFixture(t, sources, "instance/use/ClosedFieldOwner", "instance.use.InstanceFieldDriver", "30:protected:instance:field:binding:receiver:null:width:identity\n")
}
