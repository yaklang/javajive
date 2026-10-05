package javaclassparser

import (
	"slices"
	"strings"
	"testing"
)

// The outer and inner formals are distinct binders even when they share a
// spelling. The original captured field has a raw JVM descriptor. Rebuilding
// its qualified-this receiver must not invent an outer-to-inner substitution.
const nativeLexicalRawReceiverFixture = `class ScopedList<E> extends java.util.LinkedList<E>{
 private class Cursor<E> implements java.util.Iterator<E>{
  final java.util.ListIterator<E> iter;
  Cursor(int start){iter=((ScopedList)ScopedList.this).listIterator(start);}
  public boolean hasNext(){return iter.hasNext();}
  public E next(){return iter.next();}
  public void remove(){iter.remove();}
 }
 <Item> java.util.Iterator<Item> cursor(int start){return new Cursor<Item>(start);}
}
class ScopedListDriver{public static void main(String[]args){ScopedList<Object> list=new ScopedList<Object>();Object token=new Object();list.add(token);list.add(Integer.valueOf(7));list.add(null);int rows=0;for(int n:new int[]{-1,0,1,2,3,4}){try{java.util.Iterator<Object> it=list.<Object>cursor(n);if(n<0||n>list.size())throw new AssertionError("missing bounds");int at=n;while(it.hasNext()){Object value=it.next();if(value!=list.get(at++))throw new AssertionError("erased identity");}if(at!=list.size())throw new AssertionError("iterator length");try{it.next();throw new AssertionError("missing exhaustion");}catch(java.util.NoSuchElementException expected){}rows++;}catch(IndexOutOfBoundsException expected){if(n>=0&&n<=list.size())throw new AssertionError("wrong bounds");rows++;}}System.out.println(rows+":lexical:erased:receiver:distinct:binders");}}`

func TestNativeLexicalRawReceiverDistinctBindersRoundTrip(t *testing.T) {
	testNativePrivateSetterCompiledFixture(t, "ScopedList", "ScopedListDriver", "6:lexical:erased:receiver:distinct:binders\n", func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileDebugClasses(t, nativeLexicalRawReceiverFixture, debug)
	}, nativeLexicalExactSignatures)
}
func TestNativeLexicalRawReceiverRenamedBindersRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeLexicalRawReceiverFixture, "private class Cursor<E> implements java.util.Iterator<E>", "private class Cursor<Payload> implements java.util.Iterator<Payload>", 1)
	fixture = strings.Replace(fixture, "final java.util.ListIterator<E>", "final java.util.ListIterator<Payload>", 1)
	fixture = strings.Replace(fixture, "public E next()", "public Payload next()", 1)
	fixture = strings.ReplaceAll(fixture, "ScopedList", "BinderList")
	testNativePrivateSetterCompiledFixture(t, "BinderList", "BinderListDriver", "6:lexical:erased:receiver:distinct:binders\n", func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileDebugClasses(t, fixture, debug)
	}, nativeLexicalExactSignatures)
}

func nativeLexicalExactSignatures(t *testing.T, name string, original, rebuilt []byte) {
	t.Helper()
	signatures := func(raw []byte) []string {
		obj, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		result := []string{}
		collect := func(key string, attrs []AttributeInfo) {
			for _, a := range attrs {
				if s, ok := a.(*SignatureAttribute); ok {
					if s == nil {
						t.Fatal("nil original signature")
					}
					text, known := sourceBridgeUTF8(obj, s.SignatureIndex)
					if !known {
						t.Fatal("bad signature")
					}
					result = append(result, key+"="+text)
				}
			}
		}
		collect("class", obj.Attributes)
		for _, field := range obj.Fields {
			n, _ := sourceBridgeUTF8(obj, field.NameIndex)
			d, _ := sourceBridgeUTF8(obj, field.DescriptorIndex)
			collect("field:"+n+d, field.Attributes)
		}
		for _, method := range obj.Methods {
			n, _ := sourceBridgeUTF8(obj, method.NameIndex)
			d, _ := sourceBridgeUTF8(obj, method.DescriptorIndex)
			collect("method:"+n+d, method.Attributes)
		}
		slices.Sort(result)
		return result
	}
	old, next := signatures(original), signatures(rebuilt)
	if !slices.Equal(old, next) {
		t.Fatalf("generic binding metadata changed %s\noriginal %v\nrebuilt %v", name, old, next)
	}
}

func TestNativeLexicalRawReceiverFieldReadRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeLexicalRawReceiverFixture, "Cursor(int start){iter=((ScopedList)ScopedList.this).listIterator(start);}", "Cursor(int start){iter=build(start);}java.util.ListIterator<E> build(int start){return ((ScopedList)ScopedList.this).listIterator(start);}", 1)
	testNativePrivateSetterCompiledFixture(t, "ScopedList", "ScopedListDriver", "6:lexical:erased:receiver:distinct:binders\n", func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileDebugClasses(t, fixture, debug)
	}, nativeLexicalExactSignatures)
}
func TestNativeLexicalRawReceiverTypedOuterControlRoundTrip(t *testing.T) {
	fixture := strings.Replace(nativeLexicalRawReceiverFixture, "private class Cursor<E> implements java.util.Iterator<E>", "private class Cursor implements java.util.Iterator<E>", 1)
	fixture = strings.Replace(fixture, "<Item> java.util.Iterator<Item> cursor(int start){return new Cursor<Item>(start);}", "java.util.Iterator<E> cursor(int start){return new Cursor(start);}", 1)
	fixture = strings.Replace(fixture, "((ScopedList)ScopedList.this)", "ScopedList.this", 1)
	testNativePrivateSetterCompiledFixture(t, "ScopedList", "ScopedListDriver", "6:lexical:erased:receiver:distinct:binders\n", func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileDebugClasses(t, fixture, debug)
	}, nativeLexicalExactSignatures)
}
