// Regression seed for reachingRefSlotCrossClassSiblingArmMerge
// (JDEC_REF_SLOT_CROSSCLASS_SIBLING_ARM_MERGE_OFF).
//
// Two disjoint if/else arms store SIBLING jar-internal types (SiblingArmLeft / SiblingArmRight, both
// extending SiblingArmBase — neither a subtype of the other) into one slot, both flowing into the
// post-merge `sink(node)` read. Neither the JDK sibling merge (JDK-table LUB) nor the jar-internal
// subtype merge (needs one arm to subtype the other) covers a jar-internal sibling pair, so without this
// merge the later arm splits off a fresh variable and the post-merge read is left unassigned on that path
// -> javac "variable might not have been initialized". Mirrors jsoup HtmlTreeBuilder.insert
// (TextNode / DataNode, both extending LeafNode). The sibling relation is proven from SiblingArmBase's
// bytes supplied via the test resolver.
public class CrossClassSiblingArmMergeSeed {
    static void sink(SiblingArmBase b) {
    }

    void run(boolean cond) {
        SiblingArmBase node;
        if (cond) {
            node = new SiblingArmLeft();
        } else {
            node = new SiblingArmRight();
        }
        sink(node);
    }
}
