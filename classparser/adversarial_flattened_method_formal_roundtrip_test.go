package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialFlattenedMethodFormalDescriptorRoundTrip(t *testing.T) {
	const fixture = `
interface ErasureCaptureBound {int number();}
class ErasureCaptureToken implements ErasureCaptureBound {final int value;ErasureCaptureToken(int n){value=n;}public int number(){return value;}}
interface ErasureCaptureConverter<A,B>{B forward(A input);A back(B input);}
class ErasureCaptureOwner<T>{static<T extends ErasureCaptureBound> ErasureCaptureConverter<Integer,T> make(final T token){return new ErasureCaptureConverter<Integer,T>(){public T forward(Integer n){return token;}public Integer back(T t){return Integer.valueOf(7);}};}}
class ErasureCaptureDriver {public static void main(String[]args)throws Exception{int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(boolean missing:new boolean[]{false,true}){ErasureCaptureToken token=missing?null:new ErasureCaptureToken(n);ErasureCaptureConverter<Integer,ErasureCaptureToken> c=ErasureCaptureOwner.make(token);if(c.forward(Integer.valueOf(n))!=token||c.back(token).intValue()!=7)throw new AssertionError("binding, identity and value");java.lang.reflect.Method f=c.getClass().getDeclaredMethod("forward",Integer.class);java.lang.reflect.Method b=c.getClass().getDeclaredMethod("back",ErasureCaptureBound.class);if(f.getReturnType()!=ErasureCaptureBound.class||b.getReturnType()!=Integer.class)throw new AssertionError("original physical descriptor");rows++;}System.out.println(rows+":lexical:method:erasure");}}
`
	for _, prefix := range []string{"ErasureCapture", "RenamedCapture"} {
		t.Run(prefix, func(t *testing.T) {
			source := strings.ReplaceAll(fixture, "ErasureCapture", prefix)
			testIndependentFlatClosedCalleeFamily(t, source, prefix, "10:lexical:method:erasure\n", []string{prefix + "Owner$1"})
		})
	}
}

func TestAdversarialFlattenedMethodBoundDependencyRoundTrip(t *testing.T) {
	for _, kind := range []string{"dependent", "intersection", "recursive", "unbounded method", "instance shadow", "inherited class", "named local"} {
		t.Run(kind, func(t *testing.T) {
			source := flattenedMethodBindingFixture
			owner := "ErasureCaptureOwner$1"
			switch kind {
			case "dependent":
				source = strings.ReplaceAll(source, "static<T extends ErasureCaptureBound>", "static<U extends ErasureCaptureBound,T extends U>")
			case "intersection":
				source = strings.ReplaceAll(source, "implements ErasureCaptureBound", "implements ErasureCaptureBound,java.io.Serializable")
				source = strings.ReplaceAll(source, "static<T extends ErasureCaptureBound>", "static<T extends ErasureCaptureBound & java.io.Serializable>")
			case "recursive":
				source = strings.ReplaceAll(source, "implements ErasureCaptureBound", "implements ErasureCaptureBound,Comparable<ErasureCaptureToken>")
				source = strings.ReplaceAll(source, "public int number(){return value;}", "public int number(){return value;}public int compareTo(ErasureCaptureToken other){return Integer.compare(value,other.value);}")
				source = strings.ReplaceAll(source, "static<T extends ErasureCaptureBound>", "static<T extends ErasureCaptureBound & Comparable<T>>")
			case "unbounded method":
				source = strings.ReplaceAll(source, "static<T extends ErasureCaptureBound>", "static<T>")
				source = strings.ReplaceAll(source, "t==null?-1:t.number()", "7")
				source = strings.ReplaceAll(source, "missing?-1:n", "7")
				source = strings.ReplaceAll(source, "ErasureCaptureBound.class", "Object.class")
			case "instance shadow":
				source = strings.ReplaceAll(source, "static<T extends ErasureCaptureBound>", "<T extends ErasureCaptureBound>")
				source = strings.ReplaceAll(source, "ErasureCaptureOwner.make(token)", "new ErasureCaptureOwner<Number>().make(token)")
			case "inherited class":
				source = strings.ReplaceAll(source, "Owner<T extends Number>", "Owner<T extends ErasureCaptureBound>")
				source = strings.ReplaceAll(source, "static<T extends ErasureCaptureBound> ", "")
				source = strings.ReplaceAll(source, "ErasureCaptureOwner.make(token)", "new ErasureCaptureOwner<ErasureCaptureToken>().make(token)")
			case "named local":
				source = strings.ReplaceAll(source, "return new ErasureCaptureConverter<Integer,T>(){", "class Entry implements ErasureCaptureConverter<Integer,T>{")
				source = strings.ReplaceAll(source, "}};}}", "}}return new Entry();}}")
				owner = "ErasureCaptureOwner$1Entry"
			}
			testIndependentFlatClosedCalleeFamily(t, source, "ErasureCapture", "10:lexical:method:erasure\n", []string{owner})
		})
	}
}

func TestAdversarialFlattenedShadowedMethodFormalRoundTrip(t *testing.T) {

	for _, prefix := range []string{"ErasureCapture", "RenamedCapture"} {
		t.Run(prefix, func(t *testing.T) {
			source := strings.ReplaceAll(flattenedMethodBindingFixture, "ErasureCapture", prefix)
			testIndependentFlatClosedCalleeFamily(t, source, prefix, "10:lexical:method:erasure\n", []string{prefix + "Owner$1"})
		})
	}
}

func TestAdversarialFlattenedMethodReturnAndFormalBoundRoundTrip(t *testing.T) {
	const fixture = `interface ReturnCaptureBound{int number();}
class ReturnCaptureOwner<T extends Number>{static<T extends ReturnCaptureBound>Object make(){class Entry{public T value(){return null;}public<U extends T>U dependent(){return null;}}return new Entry();}}
class ReturnCaptureDriver{public static void main(String[]args)throws Exception{Object o=ReturnCaptureOwner.<ReturnCaptureBound>make();for(String name:new String[]{"value","dependent"}){java.lang.reflect.Method m=o.getClass().getDeclaredMethod(name);if(m.getReturnType()!=ReturnCaptureBound.class||m.invoke(o)!=null)throw new AssertionError("return-only/formal-bound lexical descriptor");}System.out.println("return:bound:lexical");}}`
	testIndependentFlatClosedCalleeFamily(t, fixture, "ReturnCapture", "return:bound:lexical\n", []string{"ReturnCaptureOwner$1Entry"})
}

const flattenedMethodBindingFixture = `
interface ErasureCaptureBound {int number();}
class ErasureCaptureToken implements ErasureCaptureBound {final int value;ErasureCaptureToken(int n){value=n;}public int number(){return value;}}
interface ErasureCaptureConverter<A,B>{B forward(A input);A back(B input);}
class ErasureCaptureOwner<T extends Number>{static<T extends ErasureCaptureBound> ErasureCaptureConverter<Integer,T> make(final T token){return new ErasureCaptureConverter<Integer,T>(){public T forward(Integer n){return token;}public Integer back(T t){return Integer.valueOf(t==null?-1:t.number());}};}}
class ErasureCaptureDriver {public static void main(String[]args)throws Exception{int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(boolean missing:new boolean[]{false,true}){ErasureCaptureToken token=missing?null:new ErasureCaptureToken(n);ErasureCaptureConverter<Integer,ErasureCaptureToken> c=ErasureCaptureOwner.make(token);if(c.forward(Integer.valueOf(n))!=token||c.back(token).intValue()!=(missing?-1:n))throw new AssertionError("binding, identity and value");java.lang.reflect.Method f=c.getClass().getDeclaredMethod("forward",Integer.class);java.lang.reflect.Method b=c.getClass().getDeclaredMethod("back",ErasureCaptureBound.class);if(f.getReturnType()!=ErasureCaptureBound.class||b.getReturnType()!=Integer.class)throw new AssertionError("original physical descriptor");rows++;}System.out.println(rows+":lexical:method:erasure");}}
`
