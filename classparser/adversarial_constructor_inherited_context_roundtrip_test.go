package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialConstructorInheritedContextCaptureRoundTrip(t *testing.T) {
	const fixture = `
abstract class InheritedContextBase{final Object parent;final boolean top;final int ordinal;InheritedContextBase(Object p,int n){parent=p;top=p==null;ordinal=n;}}
class InheritedContextLeaf extends InheritedContextBase{final InheritedContextParent reader;InheritedContextLeaf(InheritedContextParent r,int n){super(null,n);reader=r;}}
class InheritedContextParent{final InheritedContextLeaf context;InheritedContextParent(int n,RuntimeException failure){context=new InheritedContextLeaf(this,n);if(n<0)throw failure;}Object token(){return null;}Object owner(){return null;}boolean retained(){return context.reader==this&&context.top&&context.parent==null;}}
class InheritedContextOwner{final class Child extends InheritedContextParent{final Object value;Child(Object v,int n,RuntimeException failure){super(n,failure);value=v;}Object token(){return value;}Object owner(){return InheritedContextOwner.this;}}InheritedContextParent make(Object v,int n,RuntimeException failure){return new Child(v,n,failure);}}
class InheritedContextDriver{public static void main(String[]args){int rows=0;InheritedContextOwner owner=new InheritedContextOwner();Object token=new Object();for(Object v:new Object[]{null,token})for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){RuntimeException failure=new RuntimeException("identity");try{InheritedContextParent p=owner.make(v,n,failure);if(n<0||p.token()!=v||p.owner()!=owner||!p.retained()||p.context.ordinal!=n)throw new AssertionError("inherited private context/capture");}catch(RuntimeException e){if(n>=0||e!=failure)throw new AssertionError("failure identity",e);}rows++;}System.out.println(rows+":inherited-context:retention:identity:abrupt");}}
`
	for _, namespace := range []string{"InheritedContext", "PrivateCarrier"} {
		t.Run(namespace, func(t *testing.T) {
			source := strings.ReplaceAll(fixture, "InheritedContext", namespace)
			testNativeIndependentFamilyFixture(t, source, []string{namespace + "Owner"}, namespace+"Driver", "10:inherited-context:retention:identity:abrupt\n", nativeLexicalExactSignatures)
		})
	}
}
