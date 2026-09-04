// Regression seed for wildcardArgInvariantAddCast (JDEC_WILDCARD_ARG_ADD_CAST_OFF).
// `List<Cell<R,C,V>>.add(Cell<? extends R, ? extends C, ? extends V>)` is rejected
// after wildcard capture (CAP#1 cannot convert to Cell<R,C,V>). The source's
// `(Cell<R,C,V>)` / raw `(Cell)` cast erases; re-emit raw `(Cell)`. Real hit:
// guava ImmutableTable$Builder.put(Table.Cell).
// Recompile: javac --release 8 -d . WildcardArgAddCastSeed.java
import java.util.ArrayList;
import java.util.List;

public class WildcardArgAddCastSeed<R, C, V> {
    interface Cell<R, C, V> {}

    static class ImmCell<R, C, V> implements Cell<R, C, V> {}

    final List<Cell<R, C, V>> cells = new ArrayList<Cell<R, C, V>>();

    void put(Cell<? extends R, ? extends C, ? extends V> cell) {
        if (cell instanceof ImmCell) {
            this.cells.add((Cell<R, C, V>) cell);
        }
    }
}
