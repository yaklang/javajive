// 种子: 同类字段声明为 X<C> (以类型变量 C 参数化), 与同 raw 擦除的泛型静态工厂调用做 == / != 比较。源码原带
// 显式类型见证 CmpBound.<C>top(), 字节码擦除见证后只剩 invokestatic top(); 反编译还原为 `field != CmpBound.top()`。
// 裸 == / != 无目标类型, javac 独立推断 top() 的自由返回类型变量到其上界 (Comparable), 得 CmpBound<Comparable>,
// 与字段 CmpBound<C> 不可比 ——「incomparable types: CmpBound<C> and CmpBound<Comparable>」。治法
// (JDEC_CMP_GENERIC_FIELD_RAW_CAST_OFF): 对该调用侧补一处 raw (CmpBound) 造型, 同擦除下 raw 与参数化互相可比,
// 永远合法。镜像 guava TreeRangeSet$ComplementRangesByLowerBound$1/$2 的 `nextComplementRangeLowerBound != Cut.aboveAll()`。
public class CmpGenericFieldRawCastSeed<C extends Comparable<?>> {
    CmpBound<C> current;
    int step;

    void advance() {
        if (this.current != CmpBound.<C>top()) {
            this.step++;
        }
    }
}

class CmpBound<C extends Comparable<?>> {
    static <C extends Comparable<?>> CmpBound<C> top() {
        return new CmpBound<C>();
    }
}
