// Regression seed for factoryReturnRawBridge on Collections.unmodifiableList
// (JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF). Arrays.asList(Object[]) infers List<Object>,
// so unmodifiableList infers List<Object>; returning List<E extends T> or Iterable<L>
// then fails javac ("inference variable T#1 has incompatible bounds"). The
// raw-erasure bridge `(List<E>) (List) unmodifiableList(...)` /
// `(Iterable<L>) (Iterable) unmodifiableList(...)` is an unchecked conversion.
// Real hits: guava Ordering.leastOf, Striped.bulkGet.
// Recompile: javac --release 8 -d . UnmodifiableListBridgeSeed.java
import java.util.Arrays;
import java.util.Collection;
import java.util.Collections;
import java.util.List;

public class UnmodifiableListBridgeSeed<T> {
    public <E extends T> List<E> leastOf(Collection<E> c, int k) {
        Object[] arr = c.toArray();
        if (arr.length > k) {
            arr = Arrays.copyOf(arr, k);
        }
        return (List<E>) (List) Collections.unmodifiableList(Arrays.asList(arr));
    }

    public Iterable<T> bulkGet(Object[] keys) {
        return (Iterable<T>) (Iterable) Collections.unmodifiableList(Arrays.asList(keys));
    }
}
