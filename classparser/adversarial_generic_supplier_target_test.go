package javaclassparser

import (
	"strings"
	"testing"
)

const genericOptionalSupplierFixture = `class SupplierBox<T>{final T value;SupplierBox(T value){this.value=value;}}class SupplierMaker{<T>java.util.Optional<T> pick(SupplierBox<T> box,T present){return java.util.Optional.ofNullable(present);} <T>T build(SupplierBox<T> box){return box.value;}}class SupplierEffects{static String trace="";static void copy(Object a,Object b){trace+="C";if(a!=b)throw new AssertionError("factory identity");}}
class SupplierOwner{static <T>T make(SupplierMaker maker,SupplierBox<T> box,T present,Object source){T result;if(source!=null){SupplierBox<T> capture=box;SupplierMaker producer=maker;Object copied=source;result=maker.pick(box,present).orElseGet(()->{T value=producer.build(capture);SupplierEffects.copy(copied,value);return value;});}else{result=maker.build(box);}return result;}}
class SupplierDriver{public static void main(String[]args){Object token=new Object();SupplierMaker maker=new SupplierMaker();SupplierBox<Object> box=new SupplierBox<Object>(token);for(boolean source:new boolean[]{false,true})for(boolean present:new boolean[]{false,true}){SupplierEffects.trace="";Object got=SupplierOwner.make(maker,box,present?token:null,source?token:null);if(got!=token||!SupplierEffects.trace.equals(source&&!present?"C":""))throw new AssertionError("supplier result/inference/evaluation order");}System.out.println("4:generic-method:witness:supplier:identity:order");}}`

func TestAdversarialGenericOptionalSupplierUsesOriginalMethodWitnesses(t *testing.T) {
	testNativeIndependentFamilyFixture(t, genericOptionalSupplierFixture, []string{"SupplierOwner"}, "SupplierDriver", "4:generic-method:witness:supplier:identity:order\n", nativeLexicalExactSignatures)
}

// The caller cast erases to Object and therefore leaves no CHECKCAST. Box<T>
// remains an independent invariant witness for the caller-scoped T.
func TestAdversarialGenericOptionalSupplierRestoresErasedCallerArgument(t *testing.T) {
	f := strings.Replace(genericOptionalSupplierFixture, "SupplierBox<T> box,T present,Object source", "SupplierBox<T> box,Object present,Object source", 1)
	f = strings.Replace(f, "maker.pick(box,present).orElseGet", "maker.pick(box,(T)present).orElseGet", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"SupplierOwner"}, "SupplierDriver", "4:generic-method:witness:supplier:identity:order\n", nativeLexicalExactSignatures)
}

func TestAdversarialGenericOptionalSupplierStaticErasedCallerArgument(t *testing.T) {
	f := strings.Replace(genericOptionalSupplierFixture, "SupplierBox<T> box,T present,Object source", "SupplierBox<T> box,Object present,Object source", 1)
	f = strings.Replace(f, "maker.pick(box,present).orElseGet", "maker.pick(box,(T)present).orElseGet", 1)
	f = strings.Replace(f, "<T>java.util.Optional<T> pick", "static <T>java.util.Optional<T> pick", 1)
	f = strings.Replace(f, "<T>T build", "static <T>T build", 1)
	testNativeIndependentFamilyFixture(t, f, []string{"SupplierOwner"}, "SupplierDriver", "4:generic-method:witness:supplier:identity:order\n", nativeLexicalExactSignatures)
}
