public class ClassEnumRetSeed {
    @SuppressWarnings("unchecked")
    static Class<Enum<?>> wrap(Class<?> c) {
        return (Class<Enum<?>>) (Class) c;
    }
}
