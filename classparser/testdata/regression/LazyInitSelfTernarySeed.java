import java.util.ArrayList;
import java.util.List;

public class LazyInitSelfTernarySeed {
    // Lazy-init self-guard: `list = (list != null) ? list : new ArrayList()`. javac compiles this to a
    // conditional store whose control-flow merge types the slot as the LUB of its null-init (Object) arm
    // and the concrete `new ArrayList()` arm — i.e. Object. Without the narrowing fix the local is
    // declared `Object list` and the later `list.add(x)` fails to recompile ("cannot find symbol").
    // The fix (JDEC_LAZY_INIT_SELF_TERNARY_OFF) narrows the declaration to the concrete arm (ArrayList).
    public int probe(int[] items) {
        List list = null;
        for (int x : items) {
            list = (list != null) ? list : new ArrayList();
            list.add(x);
        }
        return (list == null) ? 0 : list.size();
    }
}
