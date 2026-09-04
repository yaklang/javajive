// Regression seed for factoryReturnRawBridge on Ordering.natural().reverse()
// (JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF). natural() infers Ordering<Comparable>;
// reverse() returns Ordering<Comparable> which is not Comparator<? super E>
// ("inference variable S has incompatible bounds"). The raw Comparator bridge
// `(Comparator<? super E>) (Comparator) Ordering.natural().reverse()` is an
// unchecked conversion. Real hit: guava Sets$DescendingSet.comparator.
// Recompile: javac --release 8 -d . OrderingReverseBridgeSeed.java
import java.util.Comparator;
import java.util.NavigableSet;

public class OrderingReverseBridgeSeed<E> {
    final NavigableSet<E> forward;

    OrderingReverseBridgeSeed(NavigableSet<E> forward) {
        this.forward = forward;
    }

    public Comparator<? super E> comparator() {
        Comparator<? super E> c = this.forward.comparator();
        if (c == null) {
            return (Comparator<? super E>) (Comparator) Ord.natural().reverse();
        }
        return c;
    }

    static class Ord<T> {
        static <C extends Comparable> Ord<C> natural() {
            return new Ord<C>();
        }

        <S extends T> Ord<S> reverse() {
            return new Ord<S>();
        }
    }
}
