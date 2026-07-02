// 种子: 内部类构造器形参还原为具体参数化类型 Iterable<InnerFieldRawCastSub<T>> 后, 存入声明为另一具体
// 参数化 Iterable<InnerFieldRawCastBase<T>> 的同类字段 —— 同 raw 擦除 (Iterable) 但型实参不同, 泛型不变性使
// javac 报「Iterable<Sub<T>> cannot be converted to Iterable<Base<T>>」。源码原带一处被字节码擦除的 raw
// `(Iterable)` 造型。治法 (JDEC_PARAM_FIELD_RAW_CAST_OFF): 同类具体参数化字段的同擦除异参存值, 补 raw 造型。
// 镜像 guava TreeRangeMap$AsMapOfRanges 的 `this.entryIterable = (Iterable) entryIterable`。
public class InnerFieldRawCastSeed<K> {
    class Holder<T> {
        final Iterable<InnerFieldRawCastBase<T>> items;

        Holder(Iterable<InnerFieldRawCastSub<T>> src) {
            this.items = (Iterable) src;
        }
    }
}

class InnerFieldRawCastBase<T> {
}

class InnerFieldRawCastSub<T> extends InnerFieldRawCastBase<T> {
}
