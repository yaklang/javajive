import java.util.Collections;
import java.util.List;
import java.util.function.Function;

// Regression seed for the method-reference instantiated-type upgrade
// (kill-switch JDEC_METHODREF_INSTANTIATED_TYPE_OFF).
//
// `Collections::synchronizedList` is a method reference whose target functional interface is
// `Function<List, List>`. In bytecode the invokedynamic's instantiatedMethodType (3rd bootstrap arg)
// records the specialization `(List)List`, but the reference itself carries no type. Without the
// upgrade the decompiler types the local as the RAW `Function`, whose SAM is `apply(Object)`, and the
// reconstructed `Function f = Collections::synchronizedList` fails to recompile with javac
// "incompatible types: invalid method reference". Mirrors fastjson2 ObjectReaderImpl{List,ListStr,Map,
// MapMultiValueType} `var = Collections::synchronized*/unmodifiable*`. Return type is Object (not
// Function) so the only `Function` token in the output is the local declaration under test.
public class MethodRefInstantiatedSeed {
    static Object wrap(int t, List src) {
        Function<List, List> f = Collections::synchronizedList;
        if (t > 0) {
            f = Collections::unmodifiableList;
        }
        return f.apply(src);
    }
}
