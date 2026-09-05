interface Ser<T> {}
public class WildcardObjectRetSeed {
    static Ser<?> helper() { return null; }

    @SuppressWarnings("unchecked")
    public static Ser<Object> wrap() {
        return (Ser<Object>) (Ser) helper();
    }

    @SuppressWarnings("unchecked")
    public static Ser<Object> assign(Ser<?> in) {
        Ser<Object> out = (Ser<Object>) (Ser) in;
        return out;
    }
}
