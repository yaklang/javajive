// Seed for parameterizedLocalReassignRawCast (JDEC_PARAM_LOCAL_REASSIGN_RAW_CAST_OFF).
//
// `result` is a local declared (from the initial `= type` store) at the invariant parameterization
// `Class<T>`, then REASSIGNED `result = result.getSuperclass()`. Class.getSuperclass() truly returns
// `Class<? super T>` (captured to `Class<CAP super T>`), which is NOT assignable to the invariant
// `Class<T>` -- javac rejects the bare reassignment with "Class<CAP#1> cannot be converted to Class<T>".
// The original source carried a raw `(Class)` cast that bytecode erased; the fix re-inserts it, and the
// raw type unchecked-converts back to `Class<T>` (legal, runtime-identical). Real hit: objenesis
// SerializationInstantiatorHelper / PercSerializationInstantiator.
import java.io.Serializable;

public class LocalReassignRawSeed {
    public static <T> Class<? super T> nonSerializableSuper(Class<T> type) {
        Class<? super T> result = type;
        while (Serializable.class.isAssignableFrom(result)) {
            result = result.getSuperclass();
            if (result == null) {
                throw new Error("Bad class hierarchy");
            }
        }
        return result;
    }
}
