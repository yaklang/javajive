package rawassign;

import java.util.Collection;
import java.util.Collections;
import java.util.function.Function;

// Regression seed for the raw-functional-interface reassignment cast fix (JDEC_LAMBDA_ASSIGN_CAST_OFF).
// `builder` is a RAW java.util.function.Function field (no Signature attribute). The local `b` is FIRST
// declared from that raw field (`Function b = this.builder;`), so it is declared RAW and never adopts a
// parameterized type. Later it is REASSIGNED a method reference and an explicitly-typed lambda, each of
// which the javac-visible source spells with an explicit parameterized cast to bind against the raw SAM
// (verified against fastjson2 2.0.43 ObjectReaderImplList:
//   builder = (Function<Collection, Collection>) Collections::unmodifiableCollection;
//   builder = (Function<Collection, Collection>) ((Collection list) -> Collections.singleton(...));).
// Without the cast javac rejects the method reference ("invalid method reference") and the lambda
// ("incompatible parameter types in lambda expression").
public class RawAssignLambdaSeed {
    final Function builder;

    public RawAssignLambdaSeed(Function builder) {
        this.builder = builder;
    }

    public Function pick(int k) {
        Function b = this.builder;
        if (k == 1) {
            b = (Function<Collection, Collection>) Collections::unmodifiableCollection;
        } else if (k == 2) {
            b = (Function<Collection, Collection>) ((Collection list) -> Collections.singleton(list.iterator().next()));
        }
        return b;
    }
}
