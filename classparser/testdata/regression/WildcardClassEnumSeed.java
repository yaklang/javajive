public class WildcardClassEnumSeed {
    static void take(Class<Enum<?>> c) {}

    @SuppressWarnings("unchecked")
    static void go(Class<?> c) {
        take((Class) c);
    }
}
