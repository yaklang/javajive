package javaclassparser

import (
	"strings"
	"testing"
	"unicode"
)

// Compare independent ClassFile declaration evidence, not a guessed source
// spelling or a retired workaround toggle. The JVM oracle below separately
// verifies that same-erasure views do not add checks, effects or new objects.
func assertReviewedGenericMethod(t *testing.T, raw []byte, name, descriptor, signature string) {
	t.Helper()
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedGenericMember(t, object, object.Methods, name, descriptor, signature)
}
func assertReviewedGenericField(t *testing.T, raw []byte, name, descriptor, signature string) {
	t.Helper()
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	assertReviewedGenericMember(t, object, object.Fields, name, descriptor, signature)
}
func assertReviewedGenericMember(t *testing.T, object *ClassObject, members []*MemberInfo, name, descriptor, signature string) {
	t.Helper()
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	for _, member := range members {
		if cp.GetUtf8(int(member.NameIndex)).Value != name {
			continue
		}
		gotDescriptor := cp.GetUtf8(int(member.DescriptorIndex)).Value
		gotSignature := ""
		for _, attr := range member.Attributes {
			if sig, ok := attr.(*SignatureAttribute); ok {
				gotSignature = cp.GetUtf8(int(sig.SignatureIndex)).Value
			}
		}
		if gotDescriptor != descriptor || gotSignature != signature {
			t.Fatalf("original %s tuple: descriptor=%q signature=%q; want %q %q", name, gotDescriptor, gotSignature, descriptor, signature)
		}
		return
	}
	t.Fatalf("original declaration missing %s", name)
}
func compactReviewedGenericSource(source string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, source)
}

func TestAdversarialReviewedGenericReturnBindingsRoundTrip(t *testing.T) {
	// Enable active witness controls; the retired same-erasure spelling switch
	// stays off so the declaration algorithm must prove the field bridge itself.
	for _, key := range []string{"JDEC_CLASS_FORNAME_RET_CAST_OFF", "JDEC_CROSS_RECV_WILDCARD_RET_CAST_OFF", "JDEC_SAME_ERASURE_FIELD_RET_BRIDGE_OFF", "JDEC_PARAM_RETURN_CAST_OFF", "JDEC_GENERIC_RET_SUBTYPE_CAST_OFF", "JDEC_CLASSLIT_RET_CAST_OFF"} {
		t.Setenv(key, "")
	}
	t.Setenv("JDEC_SAME_ERASURE_FIELD_RET_BRIDGE_OFF", "1")
	roundTripGenericFlow(t, "ReviewedReturns", `import java.util.*;
class ReviewBox<X>{final Object value;ReviewBox(Object value){this.value=value;}}
class ReviewBase<X>{final Object value;ReviewBase(Object value){this.value=value;}}
class ReviewConcrete extends ReviewBase<Object>{ReviewConcrete(Object value){super(value);}}
class ReviewInner<K,V> extends ReviewBase<K>{ReviewInner(K value){super(value);}}
class ReviewProducer {
 static StringBuilder trace=new StringBuilder();
 static final ReviewBox<Integer> shared=new ReviewBox<>(42);
 static final IllegalStateException failure=new IllegalStateException("same");
 static Class<?> kind(){trace.append("K");return String.class;}
 static Class<?> fault(){trace.append("X");throw failure;}
 static ReviewBox<?> nothing(){trace.append("N");return null;}
 static ReviewBox<Integer> fixed(){trace.append("F");return shared;}
 static <X> ReviewBox<X> empty(){trace.append("E");return new ReviewBox<>(null);}
 static <X> ReviewBox<X> make(X value){trace.append("M");return new ReviewBox<>(value);}
 static ReviewBox<?> wildcard(Object value){trace.append("W");return new ReviewBox<>(value);}
 static Object value(int which){trace.append("V").append(which);if(which==2)throw new IllegalArgumentException();return which==0?null:"text";}
}
public class ReviewedReturns<T> {
 Class<T> kind(){return (Class<T>)ReviewProducer.kind();}
 Class<T> fault(){return (Class<T>)ReviewProducer.fault();}
 ReviewBox<T> nothing(){return (ReviewBox<T>)ReviewProducer.nothing();}
 ReviewBox<T> fixed(){return (ReviewBox<T>)(ReviewBox)ReviewProducer.fixed();}
 ReviewBox<T> empty(){return ReviewProducer.empty();}
 Class<T> literal(){return (Class<T>)(Class)Integer.class;}
 Class<T> lookup(String name)throws ClassNotFoundException{return (Class<T>)Class.forName(name);}
 ReviewBox<T> field(){return (ReviewBox<T>)(ReviewBox)ReviewProducer.shared;}
 ReviewBox<T> make(int which){return ReviewProducer.make((T)ReviewProducer.value(which));}
 ReviewBox<? super T> wildcard(int which){return (ReviewBox<? super T>)ReviewProducer.wildcard(ReviewProducer.value(which));}
 ReviewBase<T> concrete(Object value){return (ReviewBase<T>)(ReviewBase)new ReviewConcrete(value);}
 <K,V> ReviewBase<K> identity(K value){return new ReviewInner<K,V>(value);}
 public static void main(String[]args){
  ReviewedReturns<String> returns=new ReviewedReturns<>();
  System.out.println(returns.kind()==String.class);System.out.println(returns.literal().getName());
  System.out.println(returns.field()==(Object)ReviewProducer.shared);
  System.out.println(returns.fixed()==(Object)ReviewProducer.shared);System.out.println(returns.nothing()==null);System.out.println(returns.empty().value==null);
  try{returns.fault();}catch(Throwable error){System.out.println(error==ReviewProducer.failure);}
  for(String name:new String[]{"java.lang.String","java.lang.Integer","no.such.ReviewedType"})try{System.out.println(returns.lookup(name).getName());}catch(Throwable error){System.out.println(error.getClass().getName());}
  Object marker=new Object();System.out.println(returns.concrete(marker).value==marker);System.out.println(returns.<Object,Integer>identity(marker).value==marker);
  for(int which=0;which<3;which++){try{System.out.println(returns.make(which).value);}catch(Throwable error){System.out.println(error.getClass().getName());}try{System.out.println(returns.wildcard(which).value);}catch(Throwable error){System.out.println(error.getClass().getName());}}
  // Erased views do not introduce a payload check when the value is unused.
  System.out.println(returns.field().value);System.out.println(ReviewProducer.trace);
 }
}`, Precision, Compatibility, "legacy")
}
