public class WildcardFieldSeed<T> {
    final Class<? super T> rawType;
    final Object type;

    @SuppressWarnings("unchecked")
    WildcardFieldSeed(Object t) {
        this.type = t;
        this.rawType = (Class<? super T>) raw(this.type);
    }

    @SuppressWarnings("unchecked")
    WildcardFieldSeed() {
        this.type = null;
        this.rawType = (Class<? super T>) raw(this.type);
    }

    static Class<?> raw(Object x) {
        return Object.class;
    }
}
