public class ParamRetCastSeed<E> {
    Object raw;

    static <T> ParamRetCastBox<T> box(T v, int n) {
        return null;
    }

    static ParamRetCastBox<?> rawBox(Object marker) {
        return null;
    }

    @SuppressWarnings("unchecked")
    ParamRetCastBox<E> make() {
        return box((E) this.raw, 1);
    }

    // Wildcard target arg via a STATIC call: value `rawBox()` is `ParamRetCastBox<?>`, target
    // `ParamRetCastBox<? super E>`. typeVarReturnCast bails on static calls (its wildcard branch is
    // this-receiver only), so this exercises parameterizedReturnCast's wildcard path. The `X<?>` ->
    // `X<? super E>` unchecked cast is legal; bytecode erases the source cast, so the decompiler must
    // re-inject it (mirrors guava `TypeToken<? super T> boundAsSuperclass(){ return of(bound); }`).
    @SuppressWarnings("unchecked")
    ParamRetCastBox<? super E> makeSuper() {
        return (ParamRetCastBox<? super E>) rawBox(this.raw);
    }
}

class ParamRetCastBox<T> {
}
