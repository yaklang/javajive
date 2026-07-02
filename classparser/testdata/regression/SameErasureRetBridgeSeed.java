// 种子: 泛型类 X<C extends Comparable> 有静态字段 X<Comparable> ALL(具体参数化), 与泛型方法
// `<C extends Comparable> X<C> all() { return (X<C>) (X) ALL; }`。源码原带 raw 桥接 `(X<C>)(X)ALL`:
// ALL 已是 X, 造型到 raw X 为 no-op 不发 checkcast, `(X<C>)` 亦擦成 no-op, 故字节码里两处造型全消失,
// 反编译得裸 `return ALL;`。typeVarReturnCast 见返回类型 `X<C>` 提及型变遂补 `(X<C>)`, 但 ALL 的**值类型**
// 是描述符擦除的 raw X(字段读值无实参), 故看不出需要桥接; javac 却按 ALL 的**声明泛型** `X<Comparable>`
// 定型, 判 `(X<C>)(X<Comparable>)` inconvertible(不变型, 具体实参可证不同)。治法
// (JDEC_SAME_ERASURE_FIELD_RET_BRIDGE_OFF): nestedGenericRawBridge 的同擦除分支——值是同类字段(经
// FieldSignature 取其声明泛型)且声明泛型与返回类型同擦除异具体参时, 补 raw 桥接 `(X)`。镜像 guava
// Range.all() 的 `return (Range<C>)(Range) ALL`。raw 桥接对同擦除恒合法, 只会修好绝不新增错误。
final class SameErasureRetBridgeSeed<C extends Comparable> {
    static final SameErasureRetBridgeSeed<Comparable> ALL = new SameErasureRetBridgeSeed<Comparable>();

    static <C extends Comparable> SameErasureRetBridgeSeed<C> all() {
        return (SameErasureRetBridgeSeed<C>) (SameErasureRetBridgeSeed) ALL;
    }
}
