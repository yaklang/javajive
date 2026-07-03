package nestedlambda;

import java.util.List;
import java.util.function.Predicate;
import java.util.function.Supplier;

// Regression seed for the nested-lambda parameter scoping fix (JDEC_LAMBDA_PARAM_SCOPE_OFF).
//
// javac forbids a lambda parameter from shadowing an enclosing lambda parameter that is in scope
// ("variable l0 is already defined"). The decompiler renders every lambda's parameters as a flat
// `l0,l1,...`, and because a nested lambda's arrow is materialised EAGERLY while the enclosing
// lambda's bytecode is still being parsed, the enclosing parameters are not yet named when the inner
// picks its names -- so both end up `l0` and the recompile fails. Real hits: spring-core
// MergedAnnotationPredicates.typeIn and DataBufferUtils.readAsynchronousFileChannel.
//
// Here the OUTER lambda parameter (an item) is referenced from INSIDE the INNER lambda body, so the
// two cannot share a name: the inner must be namespaced (`l2_0`) while the outer stays `l0`.
public class NestedLambdaParamSeed {
    public Supplier<Boolean> make(List<String> outer) {
        return () -> {
            return outer.stream().anyMatch((String inner) -> inner.equals(outer.get(0)) && present(inner));
        };
    }

    public Predicate<String> nested(String flag) {
        return (String a) -> {
            Predicate<String> p = (String b) -> a.equals(b) && b.startsWith(flag);
            return p.test(a);
        };
    }

    private static boolean present(String s) {
        return s != null;
    }
}
