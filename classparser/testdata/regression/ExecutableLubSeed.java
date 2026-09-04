import java.lang.reflect.Constructor;
import java.lang.reflect.Executable;
import java.lang.reflect.Method;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

// Regression seed for Method|Constructor sequential reassignment LUB.
// Spring ObjectToObjectConverter.getValidatedExecutable declares the slot from
// determineToMethod()'s Method return, then reassigns determineFactoryConstructor()'s
// Constructor<?> into the same local. javac rejects "Constructor<CAP#1> cannot be
// converted to Method". The denotable LUB is java.lang.reflect.Executable (Method and
// Constructor both extend it; Field does not). Kill-switch:
// JDEC_REF_SLOT_EXECUTABLE_ARM_MERGE_OFF.
// Recompile: javac --release 8 -d . ExecutableLubSeed.java
public class ExecutableLubSeed {
    private static final Map<Class<?>, Executable> cache = new ConcurrentHashMap<Class<?>, Executable>();

    static Method determineToMethod(Class<?> t, Class<?> s) {
        return null;
    }

    static Method determineFactoryMethod(Class<?> t, Class<?> s) {
        return null;
    }

    static Constructor<?> determineFactoryConstructor(Class<?> t, Class<?> s) {
        return null;
    }

    static Executable getValidatedExecutable(Class<?> t, Class<?> s) {
        // Source uses Executable so javac accepts Method and Constructor stores.
        // Bytecode erases the slot; the decompiler types it from determineToMethod's
        // Method return, then rejects the Constructor<?> store.
        Executable m = determineToMethod(t, s);
        if (m == null) {
            m = determineFactoryMethod(t, s);
            if (m == null) {
                m = determineFactoryConstructor(t, s);
                if (m == null) {
                    return null;
                }
            }
        }
        cache.put(t, m);
        return m;
    }

    public static void main(String[] args) {
        System.out.println(getValidatedExecutable(String.class, Integer.class));
    }
}
