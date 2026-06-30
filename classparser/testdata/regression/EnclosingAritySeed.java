// Seed for the enclosing-arity reconciliation fix (JDEC_INNER_ENCLOSING_ARITY_OFF).
//
// Both inner classes are NON-STATIC members of the generic `EnclosingAritySeed<K, V>`, so javac encodes
// the FULL enclosing type-argument set in their field generic signatures
// (`LEnclosingAritySeed<TK;TV;>.Inner;`), and reference sites therefore render `...Inner<K, V>` /
// `...KeyView<K, V>`. The usage scan that injects enclosing variables onto a flattened inner unit only
// recovers the SUBSET each inner body actually mentions:
//
//   - `Inner` mentions NEITHER K nor V  -> usage scan empty -> would declare bare `class ...Inner`
//     (mirrors gson TreeTypeAdapter$GsonContextImpl -> "type ...Inner does not take parameters").
//   - `KeyView` mentions ONLY K (via `extends AbstractSet<K>`) -> usage scan {K} -> would declare
//     `class ...KeyView<K>` (mirrors gson LinkedTreeMap$KeySet -> "wrong number of type arguments;
//     required 1").
//
// The fix adopts the nearest enclosing class's FULL ordered formal set `<K, V>` when the used set is a
// subset of it, so declaration and reference arities (and order) agree.
import java.util.AbstractSet;
import java.util.Iterator;

public class EnclosingAritySeed<K, V> {
    final Inner inner = new Inner();
    final KeyView keyView = new KeyView();
    K k;
    V v;

    K getK() {
        return this.k;
    }

    final class Inner {
        String describe() {
            return "inner";
        }
    }

    final class KeyView extends AbstractSet<K> {
        public Iterator<K> iterator() {
            return null;
        }

        public int size() {
            return 0;
        }
    }
}
