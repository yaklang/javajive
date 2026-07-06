// Regression seed for reachingRefSlotJDKSubtypeArmMerge (JDEC_REF_SLOT_JDK_SUBTYPE_ARM_MERGE_OFF).
//
// The idiom below compiles to a slot holding a JDK SUPERTYPE (Map, via the cast in the `if` arm) on one
// control-flow arm and a JDK SUBTYPE allocation (new HashMap()) on the other, both flowing into the
// post-merge `m.containsKey(..)` read. Without the merge the subtype arm splits off a fresh variable and
// the post-merge read is left unassigned on that path -> javac "variable might not have been
// initialized". Mirrors jsoup Whitelist.addProtocols/addAttributes.
import java.util.HashMap;
import java.util.Map;

public class JDKSubtypeArmMergeSeed {
    private final Map<String, Map<String, Integer>> outer = new HashMap<String, Map<String, Integer>>();

    void add(String k, String a, int v) {
        Map<String, Integer> inner;
        if (this.outer.containsKey(k)) {
            inner = this.outer.get(k);
        } else {
            inner = new HashMap<String, Integer>();
            this.outer.put(k, inner);
        }
        inner.put(a, Integer.valueOf(v));
    }
}
