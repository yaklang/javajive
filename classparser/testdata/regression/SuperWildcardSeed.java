// Seed for the lower-bounded wildcard consumer-cast (JDEC_GENERIC_SUPERWILDCARD_OFF).
//
// `sink` is declared `SuperWildcardSink<? super E>`. Calling `sink.apply(x)` binds the formal `T` to the
// captured `? super E`, a CONSUMER position that accepts an `E`. The source casts the erased `Object`
// argument to `(E)` (`@SuppressWarnings("unchecked")`), but generics erase that cast from bytecode (both
// the field's apply and the argument erase to Object, no checkcast emitted), so the decompiler must RE-ADD
// `(E)` to recompile -- otherwise javac reports "Object cannot be converted to CAP#1 (? super E)". Mirrors
// guava Collections2$FilteredCollection / Multisets$FilteredMultiset / Maps$FilteredKeyMap
// `this.predicate.apply((E) element)`.
public class SuperWildcardSeed<E> {
    SuperWildcardSink<? super E> sink;

    @SuppressWarnings("unchecked")
    public boolean check(Object element) {
        return this.sink.apply((E) element);
    }
}
