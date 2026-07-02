// 兄弟单元: 泛型类 Domain<C extends Comparable> 声明 `C next(C value)`, 返回类型是类型变量 C。
// 供 TypeVarLocalDeclSeed.make 的接收者 (Domain<C> 形参) 使用; 反编译器需经 SiblingClassSig 恢复
// next 的真实实例化返回 C, 才能把 `C var = domain.next(...)` 的局部声明治本为类型变量而非擦除边界。
abstract class TypeVarLocalDeclDomain<C extends Comparable> {
    abstract C next(C value);
}
