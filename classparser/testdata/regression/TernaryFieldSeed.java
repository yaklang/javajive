// Seed for the ternary wildcard-field-store cast (part of wildcardFieldStoreCast,
// kill-switch JDEC_WILDCARD_FIELD_CAST_OFF).
//
// `this.comparator = c != null ? c : NATURAL_ORDER` stores a CONDITIONAL into a field declared
// `Comparator<? super K>` (a wildcard parameterization mentioning the class type variable K). One arm
// (`c`) is exactly that type, the OTHER arm (`NATURAL_ORDER`) is a `Comparator<Comparable>`. A ternary
// is a poly expression whose type JavaJive computes as the merge of its arms (TernaryExpression.Type ->
// MergeTypes), and an unresolved merge silently keeps the FIRST arm's type, so the value reports the
// exact `Comparator<? super K>` -- hiding that the NATURAL_ORDER arm is NOT assignment-compatible. javac
// rejects the bare store with "bad type in conditional expression". An explicit
// `(Comparator<? super K>)(...)` re-targets the whole conditional as a poly expression so BOTH arms are
// checked against (and unchecked-converted to) the field type. Mirrors gson LinkedTreeMap /
// LinkedHashTreeMap NATURAL_ORDER.
import java.util.Comparator;

public class TernaryFieldSeed<K> {
    static final Comparator<Comparable> NATURAL_ORDER = naturalOrder();
    Comparator<? super K> comparator;

    @SuppressWarnings("unchecked")
    public TernaryFieldSeed(Comparator<? super K> c) {
        // The source cast is required to compile; generics erase it from bytecode (both arms erase to
        // raw Comparator, no checkcast emitted), so the decompiler must RE-ADD it to recompile.
        this.comparator = c != null ? c : (Comparator<? super K>) NATURAL_ORDER;
    }

    @SuppressWarnings("unchecked")
    static Comparator<Comparable> naturalOrder() {
        return (Comparator<Comparable>) (Comparator<?>) Comparator.naturalOrder();
    }
}
