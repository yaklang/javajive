package javaclassparser

import (
	"strings"
	"testing"
)

// Captured type variables belong to distinct original enclosing declarations.
// Reading only Inner's own Signature loses Root.T and Level.U, even though
// erased field/parameter types and physical enclosing instances remain valid.
func TestAdversarialMethodLocalInheritedBinderIdentityRoundTrip(t *testing.T) {
	for _, root := range []string{"InheritedLocalOwner", "OtherInheritedOwner"} {
		t.Run(root, func(t *testing.T) {
			source := `abstract class InheritedLocalBase{static Object published,observedNumber,observedText;InheritedLocalBase(){published=this;observedNumber=number();observedText=text();}abstract Number number();abstract CharSequence text();}
class InheritedLocalOwner<T extends Number>{class Level<U extends CharSequence>{class Inner{InheritedLocalBase make(final T seed,final U token){class Entry extends InheritedLocalBase{T number(){return seed;}U text(){return token;}}return new Entry();}}}InheritedLocalBase nested(T seed,CharSequence token){return new Level<CharSequence>().new Inner().make(seed,token);}}
class InheritedLocalDriver{public static void main(String[] args)throws Exception{int count=0;Number[] numbers={null,Integer.valueOf(-1),Long.valueOf(Long.MIN_VALUE),new java.math.BigInteger("9223372036854775808")};CharSequence[] texts={null,"identity",new StringBuilder("builder")};for(Number seed:numbers)for(CharSequence token:texts){InheritedLocalOwner<Number> owner=new InheritedLocalOwner<Number>();InheritedLocalBase value=owner.nested(seed,token);if(value.number()!=seed||value.text()!=token||InheritedLocalBase.published!=value||InheritedLocalBase.observedNumber!=seed||InheritedLocalBase.observedText!=token)throw new AssertionError("capture identity before superclass callbacks");Class<?> kind=value.getClass();if(!kind.isLocalClass()||kind.getEnclosingClass()!=InheritedLocalOwner.Level.Inner.class||kind.getEnclosingMethod().getDeclaringClass()!=InheritedLocalOwner.Level.Inner.class||kind.getDeclaredConstructors()[0].getParameterCount()!=3||!kind.getName().equals("InheritedLocalOwner$Level$Inner$1Entry"))throw new AssertionError("actual method owner/capture ABI");java.lang.reflect.Type first=kind.getDeclaredMethod("number").getGenericReturnType(),second=kind.getDeclaredMethod("text").getGenericReturnType();if(!first.equals(InheritedLocalOwner.class.getTypeParameters()[0])||!second.equals(InheritedLocalOwner.Level.class.getTypeParameters()[0])||first.equals(second))throw new AssertionError("distinct physical class binders");count++;}System.out.println(count+":inherited:two-binders:identity:pre-super");}}`
			source = strings.ReplaceAll(source, "InheritedLocalOwner", root)
			testNativeIndependentFamilyFixture(t, source, []string{root}, "InheritedLocalDriver", "12:inherited:two-binders:identity:pre-super\n")
		})
	}
}
