public class DescKeyedSigSeed<E> {
    void add(E e) {
    }

    @SafeVarargs
    final void add(E... es) {
    }

    @SuppressWarnings("unchecked")
    void run(Object o) {
        this.add((E) o);
    }
}
