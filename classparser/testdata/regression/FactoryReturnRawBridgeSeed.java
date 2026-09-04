// Regression seed for factoryReturnRawBridge. A bounded type-variable return
// `Imm<K,V>` (K extends Enum<K>) filled by `Imm.of()` / `Imm.of(k,v)` infers
// Object and javac reports incompatible bounds / ImmutableMap-style conversion
// failure. The raw-erasure bridge `(Imm<K, V>) (Imm) Imm.of(...)` is an
// unchecked conversion. Kill-switch: JDEC_FACTORY_RETURN_RAW_BRIDGE_OFF.
// Recompile: javac --release 8 -d . FactoryReturnRawBridgeSeed.java
public class FactoryReturnRawBridgeSeed {
    static class Imm<K, V> {
        static <K, V> Imm<K, V> of() {
            return new Imm<K, V>();
        }

        static <K, V> Imm<K, V> of(K k, V v) {
            return new Imm<K, V>();
        }
    }

    static <K extends Enum<K>, V> Imm<K, V> asImm(K k, V v) {
        if (k == null) {
            return Imm.of();
        }
        return Imm.of(k, v);
    }
}
