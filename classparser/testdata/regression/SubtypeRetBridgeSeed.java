// 种子: 泛型方法返回 X<C>(C 为方法作用域类型变量), 方法体返回一个 X 的 final 非泛型子类(经静态工厂
// 调用 get() 取得), 该子类把超类型实参固定为 X<Comparable>。源码原带被字节码擦除的 raw 桥接造型
// (X<C>)(X): 直接 (X<C>) 造型会 inconvertible(final 子类只实现 X<Comparable>), 非受检转换也不适用, 唯有
// 经 raw 中转 (X<C>)(X)value 合法。字节码里 upcast 到超类不发 checkcast, 故反编译得裸 `return get();`;
// 需从子类的 SiblingClassSig 判定其为非泛型子类(固定超类实参)并重新发出桥接造型。areturn 校验保证
// value <: X, 故 (X)value 永不 inconvertible。镜像 guava Cut.aboveAll()/belowAll()。
// 治法 kill-switch: JDEC_GENERIC_SUBTYPE_RET_BRIDGE_OFF。
public class SubtypeRetBridgeSeed {
    static <C extends Comparable> SubtypeRetBridgeBase<C> top() {
        return (SubtypeRetBridgeBase<C>) (SubtypeRetBridgeBase) SubtypeRetBridgeTop.get();
    }
}

class SubtypeRetBridgeBase<C extends Comparable> {
}

final class SubtypeRetBridgeTop extends SubtypeRetBridgeBase<Comparable> {
    static final SubtypeRetBridgeTop INSTANCE = new SubtypeRetBridgeTop();

    static SubtypeRetBridgeTop get() {
        return INSTANCE;
    }
}
