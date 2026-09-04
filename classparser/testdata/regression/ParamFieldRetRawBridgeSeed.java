// Regression seed for parameterizedFieldReturnRawBridge. A field declared
// `Holder<K, ? extends SubBox<V>>` returned as `Holder<K, Box<V>>` is rejected
// after wildcard capture (CAP#1 cannot convert to Box<V>). The raw-erasure
// bridge `(Holder<K, Box<V>>) (Holder) this.map` is an unchecked conversion.
// Kill-switch: JDEC_PARAM_FIELD_RET_RAW_BRIDGE_OFF.
// Recompile: javac --release 8 -d . ParamFieldRetRawBridgeSeed.java
public class ParamFieldRetRawBridgeSeed<K, V> {
    static class Box<T> {}
    static class SubBox<T> extends Box<T> {}
    static class Holder<A, B> {}

    final Holder<K, ? extends SubBox<V>> map = null;

    public Holder<K, Box<V>> asMap() {
        return (Holder<K, Box<V>>) (Holder) this.map;
    }
}
