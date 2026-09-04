// Regression seed for ternary-as-array-indexee parentheses
// (JDEC_TERNARY_ARRAY_INDEX_PARENS_OFF). `(cond ? a : b)[i]` must wrap the
// ternary: `?:` binds looser than `[]`, so `cond ? a : b[i]` parses as
// `cond ? a : (b[i])` (int[] vs int → "bad type in conditional expression").
// Real hit: spring TypeMappedAnnotation.getValue.
// Recompile: javac --release 8 -d . TernaryArrayIndexSeed.java
public class TernaryArrayIndexSeed {
    final int[] resolvedMirrors;
    final int[] resolvedRootMirrors;

    TernaryArrayIndexSeed(int[] a, int[] b) {
        this.resolvedMirrors = a;
        this.resolvedRootMirrors = b;
    }

    int getValue(int index, boolean far) {
        return (far ? this.resolvedMirrors : this.resolvedRootMirrors)[index];
    }
}
