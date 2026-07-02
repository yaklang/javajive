// 种子: 泛型类 X<K,V> 有字段 X<V,K> inverse(型实参交换), 无参构造器 `this.inverse = (X) this`。
// this 的值类型是原始 X(擦除后无实参), 但 javac 按类自身参数化 X<K,V> 定型, 与字段 X<V,K> 同 raw 异参
// (Map/类不变型)不可转; 源码原带被字节码擦除的 raw (X) 造型(this 已是 X, 造型到 raw X 为 no-op 不发
// checkcast), 反编译得裸 `this.inverse = this`, javac 报 `X<K,V> cannot be converted to X<V,K>`。
// 治法(JDEC_PARAM_FIELD_RAW_CAST_OFF 的 this 值扩展): 重建 this 的自身参数化 X<K,V> 后, 按同擦除异参
// 补该字段 raw 擦除名的造型 (X)(this)。镜像 guava RegularImmutableBiMap 的 `this.inverse = this`
// (字段声明 RegularImmutableBiMap<V,K>)。
// 两个构造器对 inverse 赋不同值, 阻止反编译器把赋值折叠进字段初始化器(与 guava RegularImmutableBiMap
// 的多构造器情形一致), 使 `this.inverse = this` 留在无参构造器体内、走 AssignVar 路径触发治本。
final class FieldThisRawCastSeed<K, V> {
    final FieldThisRawCastSeed<V, K> inverse;

    FieldThisRawCastSeed() {
        this.inverse = (FieldThisRawCastSeed) this;
    }

    FieldThisRawCastSeed(FieldThisRawCastSeed<V, K> other) {
        this.inverse = other;
    }
}
