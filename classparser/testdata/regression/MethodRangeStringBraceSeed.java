// Regression seed for applyLineBraces: a method that reconstructs `new StringBuilder("{")`
// must not swallow later methods when counting braces. Naive `{`/`}` counting merged wrap()
// through class-end, so nameOf's `String var1` was treated as a lambda capture of get()'s
// Method parameter (`final String var1_f1 = var1`). Mirrors spring
// SynthesizedMergedAnnotationInvocationHandler.toString / getAttributeValue / getName.
// Recompile: javac --release 8 -d . MethodRangeStringBraceSeed.java
import java.lang.reflect.Method;
import java.util.Map;
import java.util.concurrent.ConcurrentHashMap;

public class MethodRangeStringBraceSeed {
    private final Map<String, Object> cache = new ConcurrentHashMap<String, Object>();

    private String wrap(Object v) {
        if (v == null) {
            return "null";
        }
        if (v.getClass().isArray()) {
            return new StringBuilder("{").append(v).append('}').toString();
        }
        return String.valueOf(v);
    }

    private Object get(Method m) {
        // Extra use of v after computeIfAbsent so the decompiler cannot fold the
        // assignment into `return computeIfAbsent(...)` (that form is skipped as a
        // capture site because the lambda sits on a `return` line).
        Object v = this.cache.computeIfAbsent(m.getName(), (k) -> {
            return m.getReturnType();
        });
        if (v.getClass().isArray()) {
            return v;
        }
        return v;
    }

    private static String nameOf(Class<?> c) {
        String n = c.getCanonicalName();
        return n != null ? n : c.getName();
    }

    public static void main(String[] args) throws Exception {
        MethodRangeStringBraceSeed s = new MethodRangeStringBraceSeed();
        System.out.println(s.wrap(new int[] { 1 }));
        System.out.println(s.get(String.class.getMethod("length")));
        System.out.println(nameOf(String.class));
    }
}
