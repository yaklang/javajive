// Seed for the null dup-fold suppression (JDEC_NULL_DUP_FOLD_OFF).
//
// `n.left = n.right = n.parent = null` is a CHAINED field assignment: javac emits a single `aconst_null`
// duplicated down the stack with `dup_x1` and stored into all three fields. JavaJive's dup machinery folds
// that duplicated value into a shared temp typed by null's static type -- `Object var = null` -- and then
// stores the Object temp into the `Node`-typed fields, which javac rejects ("Object cannot be converted to
// Node"). Because `null` is a free, side-effect-free, immutable constant, the fix leaves it on the stack so
// each store re-materializes `null` directly (assignable to every reference type without a cast). Mirrors
// gson LinkedHashTreeMap$AvlBuilder.add / clear / removeInternal.
public class NullDupSeed {
    static final class Node {
        Node left;
        Node right;
        Node parent;
        int height;
    }

    void add(Node n) {
        n.left = n.right = n.parent = null;
        n.height = 1;
    }
}
