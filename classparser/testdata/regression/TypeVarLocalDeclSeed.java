// 种子: 泛型类 X<C extends Comparable> 的方法 `Box<C> make(Domain<C> domain)`, 方法体先声明
// 局部 `C next = domain.next(this.endpoint);` 再 `return of(next);` (of 是 `<C> Box<C> of(C)`)。
// domain.next(C) 真实返回类型变量 C, 但字节码把返回擦除到边界 Comparable 存入局部槽 (release 编译
// 无 LocalVariableTypeTable), 反编译器据存值静态类型把局部声明成 `Comparable next`, 于是 of(next)
// 推断 Box<Comparable>, 与声明返回 Box<C> 不变型冲突 -> `Box<Comparable> cannot be converted to
// Box<C>`。治法(JDEC_TYPEVAR_LOCAL_DECL_OFF): 经 SiblingClassSig 恢复 next 的真实实例化返回是在作用域
// 内的类型变量 C, 且调用非 unchecked (next 形参是类型变量而非参数化, 无原始实参), 遂把局部声明治本为
// `C next`; javac 自行把 RHS 重新推断为 C, 无需 RHS 造型。镜像 guava Cut$AboveValue.withLowerBoundType。
// 需 Domain 兄弟单元由 resolver 提供以恢复 next 的真实泛型返回 (单类反编译无 SiblingClassSig, 治本不触发)。
final class TypeVarLocalDeclSeed<C extends Comparable> {
    C endpoint;

    static <C extends Comparable> TypeVarLocalDeclBox<C> of(C value) {
        return new TypeVarLocalDeclBox<C>(value);
    }

    static <C extends Comparable> TypeVarLocalDeclBox<C> empty() {
        return new TypeVarLocalDeclBox<C>(null);
    }

    TypeVarLocalDeclBox<C> make(TypeVarLocalDeclDomain<C> domain) {
        C next = domain.next(this.endpoint);
        return (next == null) ? empty() : of(next);
    }
}
