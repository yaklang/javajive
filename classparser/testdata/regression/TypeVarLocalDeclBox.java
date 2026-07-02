// 兄弟单元: 泛型盒子 Box<C extends Comparable>, 仅用于给 TypeVarLocalDeclSeed.make 提供一个
// 类型变量参数化的返回上下文 (Box<C>)。TypeVarLocalDeclSeed.of(C) 返回 Box<C>: 只有当实参局部
// 声明为 C 时, of(next) 才推断出 Box<C> 与声明返回吻合; 若局部退化为擦除边界 Comparable, of 推断
// Box<Comparable>, 与 Box<C> 不变型冲突 -> 无造型可救, kill-switch OFF 时不可编译。
final class TypeVarLocalDeclBox<C extends Comparable> {
    final C value;

    TypeVarLocalDeclBox(C value) {
        this.value = value;
    }
}
