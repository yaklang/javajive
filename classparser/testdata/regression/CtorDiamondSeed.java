// Seed for the two coupled generic-constructor / method-reference fixes (fastjson2
// ObjectReaderImplFromString `new ...(Duration.class, Duration::parse)` and the URI/Charset/Pattern/
// ZoneId/... family).
//
// `CtorDiamondBox<T>` is a GENERIC class whose constructor takes `(Class<T>, Function<String,T>)`.
// `make()` instantiates it with a class literal and a method reference. Bytecode erases both: the call
// is a RAW `new CtorDiamondBox(...)` and the class literal flows in typed as the represented class.
//
//  1. classLiteralArgToClassParam: the class literal `Integer.class` is a `Class<Integer>`, but its
//     value type is reported as the represented class `Integer`, so the arg-cast logic wraps it as
//     `(Class)(Integer.class)` -- a raw cast that ERASES the generic and would collapse the
//     constructor's inference.
//  2. genericCtorDiamond: a RAW `new CtorDiamondBox(...)` erases the `Function<String,T>` parameter to
//     raw `Function`, so javac rejects the method reference ("invalid method reference"). The diamond
//     `<>` restores inference (Class<Integer> -> T=Integer -> Function<String,Integer>).
//
// Together they recompile to `new CtorDiamondBox<>(Integer.class, Integer::valueOf)`.
import java.util.function.Function;

class CtorDiamondBox<T> {
    CtorDiamondBox(Class<T> cls, Function<String, T> fn) {
    }
}

public class CtorDiamondSeed {
    static CtorDiamondBox<Integer> make() {
        return new CtorDiamondBox<>(Integer.class, Integer::valueOf);
    }
}
