package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"strings"
	"testing"
)

const nativeErasedStaticFieldFixture = `class FieldEffects{static String trace="";static int fail;static final RuntimeException failure=new RuntimeException("original");static final String token=new String("token");}
class FieldProducer implements java.util.concurrent.Callable<Object>{public Object call(){FieldEffects.trace+="C";if(FieldEffects.fail==1)throw FieldEffects.failure;return FieldEffects.token;}}
class FieldTaskParent<E>{final java.util.concurrent.Callable<E> action;FieldTaskParent(java.util.concurrent.Callable<E> action){FieldEffects.trace+="P";this.action=action;}E call()throws Exception{return action.call();}}
class ErasedFieldOwner<T>{private static final java.util.concurrent.Callable<Object> callback=new FieldProducer();class Task extends FieldTaskParent<T>{Task(){super((java.util.concurrent.Callable)callback);}}Task task(){return new Task();}}
class ErasedFieldDriver{public static void main(String[]args)throws Exception{ErasedFieldOwner<String> owner=new ErasedFieldOwner<String>();FieldEffects.trace="";FieldTaskParent<String> task=owner.task();if(!FieldEffects.trace.equals("P")||task.action==null)throw new AssertionError("constructor binding/once");FieldEffects.trace="";if(task.call()!=FieldEffects.token||!FieldEffects.trace.equals("C"))throw new AssertionError("erased producer/identity");FieldEffects.fail=1;FieldEffects.trace="";try{task.call();throw new AssertionError("missing failure");}catch(RuntimeException e){if(e!=FieldEffects.failure||!FieldEffects.trace.equals("C"))throw new AssertionError("callback identity/once");}System.out.println("private:field:erased:static:constructor:binding");}}`

func TestNativeAccessorErasedStaticFieldRoundTrip(t *testing.T) {
	testNativeErasedFieldFixture(t, nativeErasedStaticFieldFixture, "ErasedFieldOwner")
}
func TestNativeAccessorErasedStaticFieldRenamedRoundTrip(t *testing.T) {
	source := strings.ReplaceAll(nativeErasedStaticFieldFixture, "ErasedFieldOwner", "OtherRawFieldScope")
	testNativeErasedFieldFixture(t, source, "OtherRawFieldScope")
}

func TestNativeAccessorErasedInstanceFieldRoundTrip(t *testing.T) {
	source := strings.Replace(nativeErasedStaticFieldFixture, "class ErasedFieldOwner<T>{private static final", "class ErasedFieldOwner{private final", 1)
	source = strings.Replace(source, "class Task extends FieldTaskParent<T>{Task(){super((java.util.concurrent.Callable)callback);}}Task task(){return new Task();}", "static class Task<T> extends FieldTaskParent<T>{Task(ErasedFieldOwner owner){super((java.util.concurrent.Callable)owner.callback);}}<U>Task<U> task(){return new Task<U>(this);}", 1)
	source = strings.Replace(source, "ErasedFieldOwner<String> owner=new ErasedFieldOwner<String>();", "ErasedFieldOwner owner=new ErasedFieldOwner();", 1)
	source = strings.Replace(source, "FieldTaskParent<String> task=owner.task();", "FieldTaskParent<String> task=owner.<String>task();", 1)
	testNativeErasedFieldFixture(t, source, "ErasedFieldOwner")
}

func testNativeErasedFieldFixture(t *testing.T, source, owner string) {
	t.Helper()
	testNativePrivateSetterCompiledFixture(t, owner, "ErasedFieldDriver", "private:field:erased:static:constructor:binding\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, source, debug) }, func(t *testing.T, name string, original, rebuilt []byte) {
		if name != owner+"$Task.class" {
			return
		}
		counts := func(raw []byte) map[string]int {
			obj, e := Parse(raw)
			if e != nil {
				t.Fatal(e)
			}
			out := map[string]int{}
			for _, m := range obj.Methods {
				method, _ := sourceBridgeUTF8(obj, m.NameIndex)
				if method != "<init>" {
					continue
				}
				desc, _ := sourceBridgeUTF8(obj, m.DescriptorIndex)
				out[desc] = 0
				for _, a := range m.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						decoder := core.NewDecompiler(code.Code, func(i int) values.JavaValue { return GetValueFromCP(obj.ConstantPool, i) })
						if e := decoder.ParseOpcode(); e != nil {
							t.Fatal(e)
						}
						for _, op := range decoder.Opcodes() {
							if op.Instr.OpCode == core.OP_CHECKCAST {
								out[desc]++
							}
						}
					}
				}
			}
			if len(out) == 0 {
				t.Fatal("missing original constructor packet")
			}
			return out
		}
		before, after := counts(original), counts(rebuilt)
		if len(before) != len(after) {
			t.Fatal("constructor packet set changed")
		}
		for desc, n := range before {
			if n != 0 || after[desc] != 0 {
				t.Fatalf("source raw field view adds runtime cast: %s original=%d rebuilt=%d", desc, n, after[desc])
			}
		}
	})
}
