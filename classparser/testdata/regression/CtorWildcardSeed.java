// Seed for the same-class `this(...)` constructor wildcard argument cast (JDEC_CTOR_WILDCARD_CAST_OFF).
//
// The no-arg constructor delegates `this((Comparator<? super K>) NATURAL_ORDER)`. NATURAL_ORDER is a
// `Comparator<Comparable>`; the target constructor's parameter is `Comparator<? super K>`. Generics erase
// BOTH to raw `Comparator` and emit no checkcast, so the source cast vanishes from bytecode -- the
// decompiler must RE-ADD `(Comparator<? super K>)` on the self-call argument, else javac rejects
// "Comparator<Comparable> cannot be converted to Comparator<? super K>". A `this(...)` self-call always
// runs inside an instance constructor where K is in scope, so the cast is denotable. Mirrors gson
// LinkedTreeMap / LinkedHashTreeMap `this(NATURAL_ORDER)`.
import java.util.Comparator;

public class CtorWildcardSeed<K> {
    @SuppressWarnings("unchecked")
    static final Comparator<Comparable> NATURAL_ORDER =
            (Comparator<Comparable>) (Comparator<?>) Comparator.naturalOrder();

    final Comparator<? super K> comparator;

    @SuppressWarnings("unchecked")
    public CtorWildcardSeed() {
        this((Comparator<? super K>) NATURAL_ORDER);
    }

    public CtorWildcardSeed(Comparator<? super K> c) {
        this.comparator = c;
    }
}
