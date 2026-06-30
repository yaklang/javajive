// Jar-internal generic interface for the `? super E` consumer-cast seed (see SuperWildcardSeed).
// `apply(T)` is a CONSUMER: a value flowing into it is the wildcard's lower bound. Erasure rewrites the
// descriptor to apply(Object), dropping the source `(E)` cast the decompiler must recover.
public interface SuperWildcardSink<T> {
    boolean apply(T input);
}
