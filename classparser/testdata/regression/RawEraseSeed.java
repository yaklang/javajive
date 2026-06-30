// Seed for the undeclared-enclosing-type-variable raw-erase fix (JDEC_INNER_RAW_ERASE_OFF).
//
// `Itr` is a NON-STATIC inner class of the generic `RawEraseSeed<K, V>` that ALSO declares its OWN
// formal type parameter `<T>` (it is an Iterator<T>). Its field/return signatures still mention the
// enclosing variables K, V (`Node<K, V>`), which javac encodes in their generic signatures. When `Itr`
// is flattened to a top-level `RawEraseSeed$Itr<T>` unit, those K, V lose their declaration:
//
//   - The enclosing-arity reconciliation CANNOT help here -- appending <K, V> would change the arity of
//     the `Itr<ElementType>` reference sites ("wrong number of type arguments").
//   - Rendering `Node<K, V>` verbatim emits undeclared K, V -> javac "cannot find symbol: class K".
//
// The fix raw-erases those parameterizations (`Node<K, V>` -> `Node`): legal, runtime-identical, and
// exactly what the bytecode-derived local already is. This mirrors gson LinkedTreeMap$LinkedTreeMapIterator
// and guava's MapMakerInternalMap$HashIterator / Segment own-param inner-class family.
import java.util.Iterator;

public class RawEraseSeed<K, V> {
    static final class Node<X, Y> {
        Node<X, Y> next;
    }

    abstract class Itr<T> implements Iterator<T> {
        Node<K, V> next;
        Node<K, V> lastReturned;

        final Node<K, V> step() {
            Node<K, V> r = this.next;
            this.next = r.next;
            this.lastReturned = r;
            return r;
        }
    }
}
