// 种子: 非静态内部类构造器的泛型 Signature 省略了合成的首个 this$0 形参, 导致 Signature 形参数 = 描述符形参数 - 1。
// 若按数目相等才覆盖, 内部类构造器的真实泛型形参会停留在被擦除的 Object。此处 Iter(T root) 的 root 一旦停在
// Object, `Collections.singletonList(root)` 就推成 List<Object>, 存入 List<List<T>> 字段会报
// 「List<Object> cannot be converted to List<T>」。治法 (JDEC_INNER_CTOR_SIG_ALIGN_OFF): 把 Signature 形参
// 按尾部对齐到描述符形参 (保留 this$0), 使 root 还原为 T。镜像 guava TreeTraverser$PreOrderIterator。
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

public class InnerGenericCtorSeed<E> {
    class Iter<T> {
        final List<List<T>> stack = new ArrayList<List<T>>();

        Iter(T root) {
            this.stack.add(Collections.singletonList(root));
        }
    }
}
