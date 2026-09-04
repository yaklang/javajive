// Regression seed for enclosingTypeVarArgCast on Range.closed
// (JDEC_ENCLOSING_TYPEVAR_ARG_CAST_OFF). Ordering.natural().max/min returns
// Comparable (erased bound of C extends Comparable); Range.closed(C,C) then
// infers Range<Comparable> which cannot convert to Range<C> ("ContiguousSet.create
// cannot be applied"). Re-emit (C) on the erased-bound args. Real hit: guava
// RegularContiguousSet.intersection.
// Recompile: javac --release 8 -d . ClosedTypeVarArgSeed.java
public class ClosedTypeVarArgSeed<C extends Comparable> {
    static class Range<C extends Comparable> {
        static <C extends Comparable> Range<C> closed(C a, C b) {
            return new Range<C>();
        }
    }

    static class Contig<C extends Comparable> {
        static <C extends Comparable> Contig<C> create(Range<C> r, Object domain) {
            return new Contig<C>();
        }
    }

    Object domain;

    C first() {
        return null;
    }

    Contig<C> intersection(ClosedTypeVarArgSeed<C> other) {
        Comparable a = this.first();
        Comparable b = other.first();
        return Contig.create(Range.closed((C) a, (C) b), this.domain);
    }

    // Static convenience: class C is NOT in scope. Must not emit (C) on
    // Range.closed(Integer, Integer) (guava ContiguousSet.closed(int,int)).
    public static Range<Integer> closedInts(int a, int b) {
        return Range.closed(Integer.valueOf(a), Integer.valueOf(b));
    }
}
