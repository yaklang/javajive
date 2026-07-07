// Regression seed for the AtomicReference<V> V-parameter argument cast
// (jdkMethodParamTypeArgIndex AtomicReference branch; kill-switch JDEC_ATOMIC_REF_PARAM_OFF).
//
// A field `AtomicReference<T> reference` whose `get()` is read into an Object-typed local, then
// passed back to `compareAndSet(V, V)` / `getAndSet(V)` / `set(V)` (descriptor erased to
// compareAndSet/set(Object,Object)/set(Object)), renders as a bare Object argument and javac rejects
// "Object cannot be converted to T" (commons-lang3 AtomicInitializer<T>
// `this.reference.compareAndSet(null, var1)`). The argument already flowed into the V slot in
// bytecode, so re-emitting the source's unchecked `(T)` cast is behaviour-preserving.
public class AtomicRefVParamSeed<T> {
    private final java.util.concurrent.atomic.AtomicReference<T> reference =
        new java.util.concurrent.atomic.AtomicReference<>();

    public T get(java.util.function.Supplier<T> init) {
        Object var1 = this.reference.get();
        if (var1 == null) {
            var1 = init.get();
            if (!this.reference.compareAndSet(null, (T) var1)) {
                var1 = this.reference.get();
            }
        }
        return (T) var1;
    }
}
