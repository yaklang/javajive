package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

func TestAdversarialMethodLocalInheritedBinderShadowAndStaticRoundTrip(t *testing.T) {
	rows := []struct{ name, declarations, factory, firstType, secondType string }{
		{"static cut", `class InheritedVariantOwner<T>{static class Level<U extends Number>{class Inner<V extends CharSequence>{InheritedVariantBase make(final U seed,final V token){class Entry extends InheritedVariantBase{U number(){return seed;}V text(){return token;}}return new Entry();}}}static InheritedVariantBase build(Number seed,CharSequence token){return new Level<Number>().new Inner<CharSequence>().make(seed,token);}}`, "InheritedVariantOwner.build(seed,token)", "InheritedVariantOwner.Level.class.getTypeParameters()[0]", "InheritedVariantOwner.Level.Inner.class.getTypeParameters()[0]"},
		{"outer dependent bound survives inner shadow", `class InheritedVariantOwner<T extends Number,U extends T>{class Level<T extends CharSequence>{class Inner{InheritedVariantBase make(final U seed,final T token){class Entry extends InheritedVariantBase{U number(){return seed;}T text(){return token;}}return new Entry();}}}InheritedVariantBase build(U seed,CharSequence token){return new Level<CharSequence>().new Inner().make(seed,token);}}`, "new InheritedVariantOwner<Number,Number>().build(seed,token)", "InheritedVariantOwner.class.getTypeParameters()[1]", "InheritedVariantOwner.Level.class.getTypeParameters()[0]"},
		{"method shadow with inherited sibling", `class InheritedVariantOwner<T extends Number>{class Level<U extends Number>{class Inner{<T extends CharSequence> InheritedVariantBase make(final U seed,final T token){class Entry extends InheritedVariantBase{U number(){return seed;}T text(){return token;}}return new Entry();}}}InheritedVariantBase build(Number seed,CharSequence token){return new Level<Number>().new Inner().make(seed,token);}}`, "new InheritedVariantOwner<Number>().build(seed,token)", "InheritedVariantOwner.Level.class.getTypeParameters()[0]", "kind.getEnclosingMethod().getTypeParameters()[0]"},
	}
	for _, row := range rows {
		for _, root := range []string{"InheritedVariantOwner", "RenamedVariantOwner"} {
			t.Run(row.name+root, func(t *testing.T) {
				source := `abstract class InheritedVariantBase{static Object published,observedNumber,observedText;InheritedVariantBase(){published=this;observedNumber=number();observedText=text();}abstract Number number();abstract CharSequence text();}` + row.declarations + fmt.Sprintf(`
 class InheritedVariantDriver{public static void main(String[] args)throws Exception{int count=0;Number[] numbers={null,Integer.valueOf(-1),Long.valueOf(Long.MIN_VALUE),new java.math.BigInteger("9223372036854775808")};CharSequence[] texts={null,"identity",new StringBuilder("builder")};for(Number seed:numbers)for(CharSequence token:texts){InheritedVariantBase value=%s;if(value.number()!=seed||value.text()!=token||InheritedVariantBase.published!=value||InheritedVariantBase.observedNumber!=seed||InheritedVariantBase.observedText!=token)throw new AssertionError("capture identity before superclass callbacks");Class<?> kind=value.getClass();if(!kind.isLocalClass()||kind.getEnclosingClass()!=InheritedVariantOwner.Level.Inner.class||kind.getEnclosingMethod().getDeclaringClass()!=InheritedVariantOwner.Level.Inner.class||kind.getDeclaredConstructors()[0].getParameterCount()!=3||!kind.getName().equals("InheritedVariantOwner$Level$Inner$1Entry"))throw new AssertionError("actual method owner/capture ABI");java.lang.reflect.Type first=kind.getDeclaredMethod("number").getGenericReturnType(),second=kind.getDeclaredMethod("text").getGenericReturnType();if(!first.equals(%s)||!second.equals(%s)||first.equals(second))throw new AssertionError("distinct physical binders");count++;}System.out.println(count+":inherited:variants:identity:pre-super");}}`, row.factory, row.firstType, row.secondType)
				source = strings.ReplaceAll(source, "InheritedVariantOwner", root)
				testNativeIndependentFamilyFixture(t, source, []string{root}, "InheritedVariantDriver", "12:inherited:variants:identity:pre-super\n")
			})
		}
	}
}
