// Seed for lambdaArgFunctionalCast (kill-switch JDEC_LAMBDA_RAWRECV_CAST_OFF), mirroring fastjson2
// JSONSchema.of `((ObjectReaderAdapter) reader).apply((Consumer<FieldReader>) e -> { ... })`.
//
// `RawRecvBox<T>` is a GENERIC class whose method `apply(Consumer<Elem>)` takes a functional interface
// whose type argument (`Elem`) never mentions the class type variable `T`. `run` casts an Object to the
// RAW type `RawRecvBox` and calls `apply` through it. Calling any method through a RAW reference erases
// the ENTIRE method signature (JLS 4.8), so `Consumer<Elem>` collapses to raw `Consumer` (SAM
// `accept(Object)`). The lambda body dereferences `e.flag`, so the source must give the lambda a target
// type via an explicit `(Consumer<Elem>) e -> ...` cast -- without it `e` would be typed Object and
// `e.flag` would not resolve.
//
// The decompiler recovers the SAM parameter type `Elem` and renders `apply((Elem l0) -> ...)` but drops
// the functional-interface cast (the descriptor parameter is already erased), so the raw receiver
// rejects the explicitly-typed lambda ("incompatible parameter types in lambda expression").
// lambdaArgFunctionalCast re-emits the `(Consumer<Elem>)` cast so it recompiles.
import java.util.function.Consumer;
import java.util.List;

class Elem {
    public boolean flag;
    public String name;
}

class RawRecvBox<T> {
    public void apply(Consumer<Elem> c) {
    }
}

public class RawRecvLambdaSeed {
    static void run(Object reader, List<String> out) {
        RawRecvBox box = (RawRecvBox) reader;
        box.apply((Consumer<Elem>) e -> {
            if (e.flag) {
                out.add(e.name);
            }
        });
    }
}
