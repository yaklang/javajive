// Seed for same-class this.method() receiver generic-return recovery (JDEC_GENERIC_PARAM_RECV_METHOD_OFF).
//
// `box()` returns List<E>, but the call-site VALUE is erased to raw List (jar-internal returns are not
// instantiated), so a downstream param resolver sees no type args and cannot re-emit the erased `(E)`
// cast on `box().add((E) o)`; javac -- re-resolving add against Collection<E>.add(E) -- then rejects the
// bare Object arg ("Object cannot be converted to E"). Mirrors guava Multisets$EntrySet
// `this.multiset().setCount(objVal, cnt, 0)`. receiverParamTypeArgs now recovers List<E> from box()'s
// generic RETURN signature so the `(E)` cast is re-emitted. Uses a JDK List<E> receiver so the JDK
// param table resolves add(E) without needing sibling-jar context (reproduces in single-class mode).
import java.util.List;

public class RecvMethodGenSeed<E> {
    List<E> box() {
        return null;
    }

    void put(Object o) {
        box().add((E) o);
    }
}
