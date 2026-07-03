// Seed for the STANDALONE-position undeclared-enclosing-type-variable erase fix
// (JDEC_INNER_STANDALONE_ERASE_OFF), the companion of RawEraseSeed's type-ARGUMENT raw-erase.
//
// `Itr` is a NON-STATIC inner class of the generic `StandaloneEraseSeed<K, V>` that ALSO declares its
// OWN formal type parameter `<T>`. It references the enclosing variables K, V as STANDALONE types (not
// as `Foo<K>` type ARGUMENTS): the field `K key`, the concrete method return `K peek()`, and the
// abstract method parameters `out(K, V)`. When `Itr` is flattened to a top-level `StandaloneEraseSeed$Itr`
// unit those K, V lose their declaration, and (unlike `Foo<K>` which raw-erase strips to `Foo`) a bare
// standalone `K` has no `<...>` to strip, so it renders as an undeclared `K` -> javac "cannot find
// symbol: class K".
//
// The fix renders the JVM erasure of the variable instead (java.lang.Object for these unbounded ones).
// EXCEPTION: an ABSTRACT method's parameters are left as the bare variable, because erasing them to
// Object would make a no-own-formal sibling override that declares its own K, V no longer override it
// ("same erasure, yet neither overrides"; guava AbstractMapBasedMultimap$1). This mirrors guava's
// AbstractMapBasedMultimap$Itr (`K key`) and MapMakerInternalMap$HashIterator (`E nextEntry`, advanceTo).
import java.util.Iterator;

public class StandaloneEraseSeed<K, V> {
    abstract class Itr<T> implements Iterator<T> {
        K key;

        abstract T out(K k, V v);

        K peek() {
            return this.key;
        }
    }
}
