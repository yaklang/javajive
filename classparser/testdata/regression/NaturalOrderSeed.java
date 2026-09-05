import java.lang.reflect.Method;
import java.util.Comparator;

// Regression seed for Comparator method-reference instantiated-type upgrade
// (kill-switch JDEC_METHODREF_INSTANTIATED_TYPE_OFF).
//
// okhttp3.internal.Util: `NATURAL_ORDER = String::compareTo` lives in <clinit> after a local
// (`Method m = null`), which is the hoist-barrier that keeps the store as an assignment rather
// than a field initializer. Without upgrading the method-ref's type from raw Comparator, that
// store is wrapped in a raw `(Comparator)` cast (wildcardObjectAssignRawBridge: Comparator<String>
// field vs raw Comparator value), and javac rejects `(Comparator)(String::compareTo)` as
// "invalid method reference" (raw SAM is compare(Object, Object)).
public class NaturalOrderSeed {
    public static final Comparator<String> NATURAL_ORDER;
    public static final Method ADD_SUPPRESSED;
    static {
        Method m = null;
        NATURAL_ORDER = String::compareTo;
        try {
            m = Throwable.class.getDeclaredMethod("addSuppressed", Throwable.class);
        } catch (Exception e) {
            m = null;
        }
        ADD_SUPPRESSED = m;
    }
}
