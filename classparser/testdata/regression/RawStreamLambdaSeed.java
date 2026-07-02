package rawstream;

import java.util.List;
import java.util.Objects;
import java.util.stream.Collectors;

// Regression seed for the raw-JDK-Stream-receiver lambda cast fix (JDEC_LAMBDA_RAW_JDK_RECV_CAST_OFF).
// Compiled from THREE sources in one package; only RawStreamLambdaSeed.class is kept as the seed (the
// clash reproduces from its OWN constant pool):
//
//   // rawstream/RawStreamItem.java
//   package rawstream; public interface RawStreamItem { Object val(); }
//
//   // rawstream/RawStreamBox.java
//   package rawstream; import java.util.List;
//   public interface RawStreamBox { List<RawStreamItem> getItems(); }
//
//   // rawstream/RawStreamLambdaSeed.java  (this unit)
//
// The stream receiver `box.getItems()` has descriptor return raw java.util.List (the List<RawStreamItem>
// generic lives only in RawStreamBox's Signature, which the decompiler does not consult for a value's
// type), so `.stream()` is a RAW Stream. The `.map((RawStreamItem i) -> i.val())` lambda is recorded
// with the specific instantiated parameter type RawStreamItem in the invokedynamic (the source stream WAS
// parameterized), so the decompiler renders the explicit `(RawStreamItem l0)` -- which no longer binds to
// the raw Stream.map's erased `apply(Object)` SAM ("incompatible parameter types in lambda expression").
// Compiled with `javac --release 8 -g:none` so no LocalVariableTypeTable rescues the generic. Real hit:
// fastjson2 JSONPathSegment$CycleNameSegment$MapRecursive (getFieldWriters().stream().filter(
// Objects::nonNull).map((FieldWriter l) -> l.getFieldValue(...))).
public class RawStreamLambdaSeed {
    public List<Object> collect(RawStreamBox box) {
        // Two uses of `list` keep it a NAMED local so the decompiler emits an explicit raw `List list`
        // declaration (rather than inlining box.getItems() and letting javac re-resolve its generic
        // return from the classpath Signature). javac then binds the stream to that raw declared type.
        List<RawStreamItem> list = box.getItems();
        if (list == null) {
            return null;
        }
        return list.stream().filter(Objects::nonNull).map((RawStreamItem i) -> i.val()).collect(Collectors.toList());
    }
}
