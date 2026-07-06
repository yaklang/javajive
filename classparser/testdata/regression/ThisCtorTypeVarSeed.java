// Regression seed for thisCtorTypeVarArgCast: a convenience constructor delegates via `this(...)` to a
// sibling constructor whose second formal is the BARE class type variable T, passing `(T) new Object()`.
// The bytecode erases the unchecked cast (no checkcast for a type variable), so the naive decompile renders
// `this(name, new Object())`, which javac rejects ("Object cannot be converted to T"). Mirrors spring
// PropertySource(String) -> PropertySource(String, T).
//
// Recompile: javac -d . ThisCtorTypeVarSeed.java
public class ThisCtorTypeVarSeed<T> {
    final String name;
    final T source;

    public ThisCtorTypeVarSeed(String name, T source) {
        this.name = name;
        this.source = source;
    }

    @SuppressWarnings("unchecked")
    public ThisCtorTypeVarSeed(String name) {
        this(name, (T) new Object());
    }
}
