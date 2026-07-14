// Regression seed for sameClassStaticMethodTypeVarArgCast: an instance method reads a field typed by
// the BARE class type variable T at its erased Object static type (the field's generic Signature is T
// but the descriptor is Object), then passes it to a SAME-CLASS STATIC generic method whose OWN
// method-scope type variable is ALSO named T (shadowing the class T at the declaration, but at the
// instance-method call site the source's `(T)` cast resolves to the CLASS-scope T). The bytecode erases
// the unchecked cast (no checkcast for a type variable), so the naive decompile renders a bare argument
// that javac feeds as Object to the generic static method, breaking inference ("incompatible bounds").
// Mirrors commons-lang3 Range.intersectionWith -> Range.<T>between(T, T, Comparator<T>).
//
// Recompile: javac -d . SameClassStaticTypeVarArgSeed.java
public class SameClassStaticTypeVarArgSeed<T> {
    final T minimum;
    final T maximum;

    public SameClassStaticTypeVarArgSeed(T min, T max) {
        this.minimum = min;
        this.maximum = max;
    }

    public static <T> SameClassStaticTypeVarArgSeed<T> between(T min, T max) {
        return new SameClassStaticTypeVarArgSeed<T>(min, max);
    }

    @SuppressWarnings("unchecked")
    public SameClassStaticTypeVarArgSeed<T> intersectionWith(SameClassStaticTypeVarArgSeed<T> other) {
        T min = (this.minimum.hashCode() < other.minimum.hashCode()) ? other.minimum : this.minimum;
        T max = (this.maximum.hashCode() < other.maximum.hashCode()) ? this.maximum : other.maximum;
        return between((T) min, (T) max);
    }
}