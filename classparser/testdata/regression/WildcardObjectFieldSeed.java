public class WildcardObjectFieldSeed {
    Ser<Object> field;

    Ser<?> secondary(Ser<?> s) { return s; }

    @SuppressWarnings("unchecked")
    void fill(Ser<?> in) {
        this.field = (Ser<Object>) (Ser) this.secondary(in);
    }
}
