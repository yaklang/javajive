// Regression seed for concreteParamReturnSubtypeRawCast Shape 3: a method declared to return the concrete
// parameterization `Map<String,Object>` returns `System.getenv()`, whose JDK signature is the FIXED
// `Map<String,String>` -- same erasure, different args, so the bare return never converts and the direct
// parameterized cast is inconvertible (invariant). The source carried the raw `(Map)` cast, which the
// bytecode drops (erased checkcast is a no-op). Mirrors spring AbstractEnvironment.getSystemEnvironment().
//
// Recompile: javac -d . GetenvReturnSeed.java
import java.util.Map;

public class GetenvReturnSeed {
    @SuppressWarnings({"unchecked", "rawtypes"})
    public Map<String, Object> env() {
        return (Map) System.getenv();
    }

    // Control: the declared return matches getenv()'s fixed parameterization exactly -- NO cast wanted.
    public Map<String, String> envExact() {
        return System.getenv();
    }
}
