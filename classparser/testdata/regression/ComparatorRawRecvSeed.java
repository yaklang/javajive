// Seed for the raw `((Comparator)(recv))` receiver cast on `recv.compare(a, b)` (JDEC_COMPARATOR_RAW_RECV_OFF).
//
// A wildcard `Comparator<?>` receiver binds compare to `compare(CAP, CAP)`, which rejects Object
// arguments. Guava's source widens the receiver first (`((Comparator<Object>) c).compare(a, b)`), whose
// cast erases to a no-op checkcast and vanishes from bytecode; the decompiler then renders bare
// `c.compare(a, b)` and javac rejects it ("Object cannot be converted to CAP#1"). Mirrors guava
// ImmutableSortedSet.unsafeCompare, TreeMultiset, ImmutableSortedMap$Builder. The fix re-emits a raw
// `((Comparator)(recv))` receiver cast (an unchecked, behaviour-preserving compare(Object, Object)).
import java.util.Comparator;

public class ComparatorRawRecvSeed {
    static int cmp(Comparator<?> c, Object a, Object b) {
        return ((Comparator<Object>) c).compare(a, b);
    }
}
