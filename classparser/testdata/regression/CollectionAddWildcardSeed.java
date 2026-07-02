// Seed for the raw `((Collection)(recv))` receiver cast on `recv.add(x)` with a wildcard
// `Collection<? super E>` receiver (JDEC_COLLECTION_ADD_RAW_RECV_OFF).
//
// add binds to `add(CAP)` on the wildcard receiver; the source casts the element (`c.add((E) o)`), whose
// cast erases to a no-op and vanishes; the decompiler renders bare `c.add(o)` (o: Object) and javac
// rejects it ("Object cannot be converted to CAP#1"). Mirrors guava Queues.drainUninterruptibly
// `var1.add(var7)` with `var1: Collection<? super E>`. The fix re-emits a raw `((Collection)(recv))`
// receiver cast (an unchecked, behaviour-preserving add(Object)).
import java.util.Collection;

public class CollectionAddWildcardSeed<E> {
    void drain(Collection<? super E> c, Object o) {
        c.add((E) o);
    }
}
