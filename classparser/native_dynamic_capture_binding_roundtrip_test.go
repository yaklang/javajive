package javaclassparser

import (
	"strings"
	"testing"
)

const nativeDynamicCaptureBindingFixture = `class SnapshotOwner{private Object token;SnapshotOwner(Object value){token=value;}static class Reader{Object get(SnapshotOwner value){return value.token;}}}
class SnapshotOwnerConsumer{static int reserved;final SnapshotOwner.Reader reader;SnapshotOwnerConsumer(SnapshotOwner.Reader input){reader=input;}boolean consume(Object value,java.util.function.Predicate<Object> predicate){return predicate.test(value);}boolean match(Object value,Object input){return consume(value,item->item==input);}}
class SnapshotDriver{public static void main(String[]args){Object token=new Object();SnapshotOwner owner=new SnapshotOwner(token);SnapshotOwnerConsumer consumer=new SnapshotOwnerConsumer(new SnapshotOwner.Reader());if(consumer.reader.get(owner)!=token)throw new AssertionError("private binding");int rows=0;for(Object a:new Object[]{null,token,"text"})for(Object b:new Object[]{null,token,"text"}){if(consumer.match(a,b)!=(a==b))throw new AssertionError("capture identity");rows++;}System.out.println(rows+":dynamic:capture:source:binding");}}`

func TestNativeDynamicCaptureSourceBindingRoundTrip(t *testing.T) {
	testNativePrivateSetterCompiledFixture(t, "SnapshotOwner", "SnapshotDriver", "9:dynamic:capture:source:binding\n", func(t *testing.T, debug string) map[string][]byte {
		return nativeCompileDebugClasses(t, nativeDynamicCaptureBindingFixture, debug)
	})
}
func TestNativeDynamicCaptureSourceBindingRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeDynamicCaptureBindingFixture, "SnapshotOwner", "AdjacentScope")
	f = strings.ReplaceAll(f, "item->item==input", "element->element==input")
	testNativePrivateSetterCompiledFixture(t, "AdjacentScope", "SnapshotDriver", "9:dynamic:capture:source:binding\n", func(t *testing.T, debug string) map[string][]byte { return nativeCompileDebugClasses(t, f, debug) })
}
