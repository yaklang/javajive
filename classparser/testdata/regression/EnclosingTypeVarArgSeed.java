// Regression seed for enclosingTypeVarArgCast (JDEC_ENCLOSING_TYPEVAR_ARG_CAST_OFF).
// `Holder$View<K,V>.wrap` reads `entry.getKey()` (erased Object) and passes it to
// `Maps.immutableEntry(K,V)` / `Holder.wrapCollection(K, ...)`. The source's `(K)`
// casts drop in bytecode; without them javac infers immutableEntry's K from Object
// ("inference variable K#1 has incompatible bounds"). Real hit: guava
// AbstractMapBasedMultimap$AsMap.wrapEntry.
// Recompile: javac --release 8 -d . EnclosingTypeVarArgSeed.java
import java.util.Collection;
import java.util.Collections;
import java.util.Map;

public class EnclosingTypeVarArgSeed<K, V> {
    static class Maps {
        static <K, V> Map.Entry<K, V> immutableEntry(K k, V v) {
            return null;
        }
    }

    Collection<V> wrapCollection(K key, Collection<V> c) {
        return c;
    }

    class View {
        Map.Entry<K, Collection<V>> wrap(Map.Entry<K, Collection<V>> e) {
            Object k = e.getKey();
            return Maps.immutableEntry((K) k, wrapCollection((K) k, e.getValue()));
        }
    }

    static Map.Entry<String, Collection<String>> unused(Map.Entry<String, Collection<String>> e) {
        return e;
    }
}
