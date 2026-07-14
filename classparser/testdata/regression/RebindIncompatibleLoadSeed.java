// Seed for rebindIncompatibleLoadForSink (fastjson2 JDKUtils.<clinit> shape): one JVM slot carries
// incompatible reference types across disjoint try/catch blocks. The DFS-stale ref makes the putstatic
// read the wrong branch; the two-pass rebinding re-points the sink at the compatible ref.
import java.util.function.Predicate;
import java.util.function.Function;

public class RebindIncompatibleLoadSeed {
    public static Predicate<String> PRED;
    public static Function<String,String> FN;
    public static Throwable ERR;

    static Object slot;

    static {
        slot = null;
        try {
            slot = (Predicate<String>) (s -> s.isEmpty());
        } catch (Throwable e) {
            ERR = e;
        }
        try {
            slot = (Function<String,String>) (s -> s);
        } catch (Throwable e) {
            ERR = e;
        }
        PRED = (Predicate<String>) slot;
        FN = (Function<String,String>) slot;
    }
}
