// Seed for the override-parameter raw-erase (case b) + raw-receiver typevar-local bail.
//
// `OverrideParamEraseSeed<K, V>` has a NON-STATIC own-formal inner base `Task<T>` whose method
// `execute(Ref<K,V>, Ent<K,V>)` mentions the ENCLOSING variables K/V. Flattened, Task raw-erases those
// parameters (`Ref<K,V>` -> `Ref`) because it cannot declare K/V (arity of its own `<T>` sites).
//
// `Sub` is a NON-STATIC inner class with NO own formal type parameters that `extends Task<V>` and
// OVERRIDES execute. It DECLARES K/V via enclosing-arity injection, so without care its override renders
// `execute(Ref<K,V>, Ent<K,V>)` (generic) against Task's raw base -> javac "name clash ... same erasure,
// yet neither overrides". The case-(b) fix erases the override parameters to match the base.
//
// Inside Sub, `V v = e.getValue()` reads a raw-rendered receiver `Ent e` (its K/V erased in the param),
// so `getValue()` is an UNCHECKED invocation returning the erased bound (Object): the local must be
// declared `Object v` (not `V v`), with the `(V)` supplied by the return cast. Exercises both fixes.
public class OverrideParamEraseSeed<K, V> {
    static final class Ref<A, B> {
    }

    static final class Ent<A, B> {
        B getValue() {
            return null;
        }
    }

    abstract class Task<T> {
        protected T execute(Ref<K, V> r, Ent<K, V> e) {
            return null;
        }
    }

    class Sub extends Task<V> {
        protected V execute(Ref<K, V> r, Ent<K, V> e) {
            V v = e.getValue();
            return v;
        }
    }
}
