package javaclassparser

import "testing"

func TestAdversarialGenericFieldBindingRetainsMethodTypeVariables(t *testing.T) {
	const fixture = `class BindingBase<T>{public T value;public T[] array;}class BindingChild<T> extends BindingBase<T>{public T value;public T[] array;}
class BindingAccess{static<T>T read(BindingChild<T> c){return ((BindingBase<T>)c).value;}static<T extends Number>T number(BindingChild<T> c){return ((BindingBase<T>)c).value;}static<T>T[] array(BindingChild<T> c){return ((BindingBase<T>)c).array;}static<T>T assigned(BindingChild<T> c,T value){return ((BindingBase<T>)c).value=value;}}
class BindingDriver{public static void main(String[]args){int rows=0;for(Object value:new Object[]{new Object(),"word",null}){BindingChild<Object> c=new BindingChild<Object>();((BindingBase)c).value=value;c.value="child";Object[] array=new Object[]{value};((BindingBase)c).array=array;c.array=new Object[]{"child"};if(BindingAccess.read(c)!=value||BindingAccess.array(c)!=array||BindingAccess.assigned(c,array)!=array||((BindingBase)c).value!=array||c.value!="child")throw new AssertionError("method formal field owner/erasure/array");rows++;}for(Number value:new Number[]{Integer.valueOf(7),Double.valueOf(2.5),null}){BindingChild<Number> c=new BindingChild<Number>();((BindingBase)c).value=value;c.value=Integer.valueOf(19);if(BindingAccess.number(c)!=value)throw new AssertionError("number method bound");((BindingBase)c).value="wrong";try{BindingAccess.number(c);throw new AssertionError("original number check");}catch(ClassCastException expected){}rows++;}System.out.println(rows+":field-method-formal:array:erasure:identity");}}
`
	fieldOwnerOriginalDriverRoundTrip(t, fixture, "6:field-method-formal:array:erasure:identity\n")
}
