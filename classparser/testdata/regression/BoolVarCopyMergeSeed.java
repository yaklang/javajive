// Seed for reachingBoolVarCopyMerge (fastjson2 FieldWriterList.writeList /
// ObjectWriterImplList.writeList shape): a boolean local in a loop body is assigned by a SIBLING arm
// that either (a) copies an earlier boolean-default variable compiled to iconst_1/0, or (b) stores the
// result of a comparison ternary `(c && cond) ? 1 : 0`; the OTHER sibling arm stores a genuinely
// boolean-typed value (a Z-returning call). javac emits the first arm's store as iload/istore or
// iconst/istore (int category), so the decompiler's DFS mints an int version of the slot there; the
// boolean arm then refuses to merge (no int<->boolean conversion) and splits off a fresh boolean
// variable. The post-merge use (`if (itemRefDetect)`, `previous = itemRefDetect`) then renders
// `int = boolean` / `boolean != int` and javac rejects it ("boolean cannot be converted to int").
// The fix re-types the copy arm's int ref (and its proven-boolean int-0/1 default) to boolean when a
// phi proves the two arms are one source variable.
import java.util.List;

public class BoolVarCopyMergeSeed {
    long features;
    long mask = 1L;

    public boolean isRefDetect() { return false; }

    public void writeList(List list) {
        boolean previousItemRefDetect = (features & mask) != 0L;
        for (int i = 0; i < list.size(); i++) {
            Object item = list.get(i);
            if (item == null) continue;
            boolean itemRefDetect;
            if (i == 0) {
                // Shape (b): copy of an earlier boolean-default variable (`previous` was compiled to
                // iconst_1/0); emitted as iload previous; istore itemRefDetect.
                itemRefDetect = previousItemRefDetect;
            } else {
                // Shape (a): genuinely boolean-typed value (Z-returning call) on the sibling arm.
                itemRefDetect = isRefDetect();
                if (itemRefDetect) {
                    itemRefDetect = !item.getClass().isInstance(item);
                }
                previousItemRefDetect = itemRefDetect;
            }
            if (itemRefDetect && item.hashCode() > 0) {
                continue;
            }
        }
    }
}
