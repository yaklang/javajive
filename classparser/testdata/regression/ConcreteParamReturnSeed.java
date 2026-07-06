import java.util.Map;

// Regression seed for concreteParamReturnSubtypeRawCast: a method whose declared return type is a
// CONCRETE parameterization (`Map<String,Object>`) returns a value whose static type is a non-generic
// subtype of that erasure with a FIXED, distinct parameterization (`Properties` is `Map<Object,Object>`).
// The source needs a raw `(Map)` cast; the erased checkcast is a no-op the bytecode drops, so the
// decompiler must SYNTHESIZE the cast from generic analysis. Mirrors spring
// AbstractEnvironment.getSystemProperties().
//
// Recompile: javac -d . ConcreteParamReturnSeed.java
public class ConcreteParamReturnSeed {
    @SuppressWarnings({"unchecked", "rawtypes"})
    public Map<String, Object> getSystemProperties() {
        return (Map) System.getProperties();
    }
}
