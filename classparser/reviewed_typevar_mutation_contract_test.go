package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"testing"
)

// Select by the exact JVM tuple: overloaded methods must not be matched by
// their name alone before checking the independent Signature attribute.
func assertReviewedTypeVarMethod(t *testing.T, raw []byte, name, descriptor, signature string) {
	t.Helper()
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	for _, member := range object.Methods {
		if cp.GetUtf8(int(member.NameIndex)).Value == name && cp.GetUtf8(int(member.DescriptorIndex)).Value == descriptor {
			assertReviewedGenericMember(t, object, []*MemberInfo{member}, name, descriptor, signature)
			return
		}
	}
	t.Fatalf("original exact method tuple missing %s%s", name, descriptor)
}
func assertReviewedClassSignature(t *testing.T, raw []byte, signature string) {
	t.Helper()
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	for _, attr := range object.Attributes {
		if sig, ok := attr.(*SignatureAttribute); ok {
			if got := cp.GetUtf8(int(sig.SignatureIndex)).Value; got != signature {
				t.Fatalf("original class Signature=%q; want %q", got, signature)
			}
			return
		}
	}
	t.Fatal("original class Signature missing")
}

// Bind source views to the original invocation owner/name/descriptor and opcode.
func assertReviewedTypeVarInvoke(t *testing.T, path, method, descriptor string, pc uint16, opcode int, owner, name, invokeDescriptor string) {
	t.Helper()
	_, code, object := reviewedFixtureMethod(t, path, method, descriptor)
	assertReviewedOpcode(t, code, pc, opcode)
	d := core.NewDecompiler(code.Code, nil)
	if err := d.ParseOpcode(); err != nil {
		t.Fatal(err)
	}
	op := d.OpcodeByPC(pc)
	index := int(op.Data[0])<<8 | int(op.Data[1])
	cp := NewConstantPoolWithConstant(&object.ConstantPool)
	var classIndex, pairIndex uint16
	switch ref := cp.IndexInfo(index).(type) {
	case *ConstantMethodrefInfo:
		classIndex, pairIndex = ref.ClassIndex, ref.NameAndTypeIndex
	case *ConstantInterfaceMethodrefInfo:
		classIndex, pairIndex = ref.ClassIndex, ref.NameAndTypeIndex
	default:
		t.Fatal("original invocation has no method reference")
	}
	pair, ok := cp.IndexInfo(int(pairIndex)).(*ConstantNameAndTypeInfo)
	if !ok {
		t.Fatal("original invocation has no name-and-type")
	}
	if cp.GetClassName(int(classIndex)) != owner || cp.GetUtf8(int(pair.NameIndex)).Value != name || cp.GetUtf8(int(pair.DescriptorIndex)).Value != invokeDescriptor {
		t.Fatal("original invocation tuple changed; review binding again")
	}
}

func TestAdversarialReviewedTypeVariableMutationRoundTrip(t *testing.T) {
	for _, key := range []string{"JDEC_TYPEVAR_ARRAY_REASSIGN_OFF", "JDEC_TYPEVAR_ARRAY_ELEM_STORE_CAST_OFF", "JDEC_TYPEVAR_LOCAL_REASSIGN_OFF", "JDEC_SUPER_CTOR_TYPEVAR_ARG_OFF", "JDEC_GENERIC_SUPERWILDCARD_OFF"} {
		t.Setenv(key, "1")
	}
	for _, key := range []string{"JDEC_THIS_REPARAM_CAST_OFF", "JDEC_WILDCARD_RET_CAST_OFF"} {
		t.Setenv(key, "")
	}
	roundTripGenericFlow(t, "ReviewedMutations", `import java.util.*;
import java.lang.reflect.Array;
interface MutationSink<A>{boolean apply(A value);}
interface MutationStep<A>{A apply(A value);}
class MutationBase<A>{final A seed;MutationBase(A seed){this.seed=seed;}}
class MutationSupport {
 static StringBuilder trace=new StringBuilder();
 static final IllegalArgumentException failure=new IllegalArgumentException("original");
 static final Object marker=new Object();
 static final List<?> shared=Arrays.asList(marker);
 static Object item(Object[] items,int index){trace.append("V").append(index);return items[index];}
 static int position(int index){trace.append("I").append(index);return index;}
 static Object foreign(Object value){trace.append("F");return marker;}
 static Object fault(Object value){trace.append("X");throw failure;}
 static boolean accept(Object value){trace.append(value==marker?"M":value==null?"N":"A");return true;}
 static void report(Throwable error){System.out.println(error.getClass().getName()+":"+(error==failure)+":"+trace);}
}
public class ReviewedMutations<T> extends MutationBase<T> {
 MutationSink<? super T> sink;
 MutationStep[] steps;
 ReviewedMutations(Object seed){super((T)seed);sink=MutationSupport::accept;}
 <A>A[] fill(A[] target,Object[]items){
  if(target.length<items.length)target=(A[])Array.newInstance(target.getClass().getComponentType(),items.length);
  for(int i=0;i<items.length;i++)target[MutationSupport.position(i)]=(A)MutationSupport.item(items,i);
  if(target.length>items.length)target[items.length]=null;
  return target;
 }
 <A extends Number>A[] bounded(A[] target,Object[]items){
  target[MutationSupport.position(0)]=(A)MutationSupport.item(items,0);return target;
 }
 T change(T value){for(MutationStep step:steps)value=(T)step.apply(value);return value;}
 boolean consume(Object value){return sink.apply((T)value);}
 List<?> helper(){return MutationSupport.shared;}
 <A>List<A> view(boolean nil){if(nil)return null;return (List<A>)helper();}
 <A extends T>ReviewedMutations<A> narrow(){return (ReviewedMutations<A>)(ReviewedMutations)this;}
 ReviewedMutations<T> self(){return this;}
 public static void main(String[]args){
  ReviewedMutations<String> subject=new ReviewedMutations<>(MutationSupport.marker);
  System.out.println(subject.seed==(Object)MutationSupport.marker);System.out.println(subject.self()==subject);System.out.println(subject.<String>narrow()==subject);
  System.out.println(subject.<String>view(false)==(Object)MutationSupport.shared);System.out.println(subject.<String>view(true)==null);
  for(Object value:new Object[]{MutationSupport.marker,null,"text"}){MutationSupport.trace.setLength(0);System.out.println(subject.consume(value)+":"+MutationSupport.trace);}
  subject.steps=new MutationStep[]{MutationSupport::foreign};MutationSupport.trace.setLength(0);System.out.println(subject.change(null)==(Object)MutationSupport.marker);System.out.println(MutationSupport.trace);
  subject.steps=new MutationStep[]{MutationSupport::foreign,MutationSupport::fault};MutationSupport.trace.setLength(0);try{subject.change("before");}catch(Throwable error){MutationSupport.report(error);}
  String[] big=new String[]{"old","old","tail"};MutationSupport.trace.setLength(0);System.out.println(subject.fill(big,new Object[]{"new"})==big);System.out.println(Arrays.toString(big)+":"+MutationSupport.trace);
  String[] small=new String[0];MutationSupport.trace.setLength(0);String[] made=subject.fill(small,new Object[]{"a","b"});System.out.println((made!=small)+":"+made.getClass().getName()+":"+Arrays.toString(made)+":"+MutationSupport.trace);
  MutationSupport.trace.setLength(0);try{subject.fill(new String[1],new Object[]{MutationSupport.marker});}catch(Throwable error){MutationSupport.report(error);}
  MutationSupport.trace.setLength(0);try{subject.fill((Object[])null,new Object[]{"x"});}catch(Throwable error){MutationSupport.report(error);}
  MutationSupport.trace.setLength(0);Object[] shared=new Object[1];System.out.println(subject.fill(shared,new Object[]{MutationSupport.marker})==shared);System.out.println(shared[0]==MutationSupport.marker);
  for(Object value:new Object[]{"wrong",Double.valueOf(2),null}){MutationSupport.trace.setLength(0);try{subject.bounded(new Integer[1],new Object[]{value});System.out.println("stored:"+MutationSupport.trace);}catch(Throwable error){MutationSupport.report(error);}}
 }
}`, Precision, Compatibility, "legacy")
}
