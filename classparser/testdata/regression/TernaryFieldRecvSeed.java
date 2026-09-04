// Regression seed for ternary-as-field-receiver parentheses
// (JDEC_TERNARY_FIELD_RECV_PARENS_OFF). `(cond ? a : b).field` must wrap the
// ternary: `?:` binds looser than `.`, so `(cond) ? (a) : (b).field` parses as
// `(cond) ? (a) : ((b).field)` (Range vs Cut → Object; Range.create cannot be
// applied). Real hit: guava Range.gap/span.
// Recompile: javac --release 8 -d . TernaryFieldRecvSeed.java
public class TernaryFieldRecvSeed {
    static class Cut {
        final int v;
        Cut(int v) { this.v = v; }
    }

    final Cut lowerBound;
    final Cut upperBound;

    TernaryFieldRecvSeed(Cut lower, Cut upper) {
        this.lowerBound = lower;
        this.upperBound = upper;
    }

    static int create(Cut a, Cut b) {
        return a.v + b.v;
    }

    int gap(TernaryFieldRecvSeed other) {
        boolean first = this.lowerBound.v < other.lowerBound.v;
        TernaryFieldRecvSeed left = first ? this : other;
        return create(left.upperBound, (first ? other : this).lowerBound);
    }
}
