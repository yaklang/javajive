// Seed for the List.set(int, E) / add(int, E) element-arg cast (JDEC_LIST_SET_PARAM_OFF).
//
// `List<T>.get(int)` erases its return to Object, so `Object v = list.get(i)` declares v as Object.
// Passing v to `List<T>.set(int, E)` (element param erased to Object in the descriptor) drops the
// source's unchecked `(T)` cast; javac -- re-resolving against set(int, T) -- rejects it ("Object
// cannot be converted to T"). Mirrors guava Iterables.removeIfFromRandomAccessList `var0.set(var3,
// var4)`. jdkMethodParamTypeArgIndex now maps List.set/add(int, E)'s second parameter to the receiver's
// element type arg so the `(T)` cast is re-emitted. The intervening sink() keeps v from being inlined.
import java.util.List;

public class ListSetElemSeed<T> {
    void shift(List<T> list, int i, int j) {
        Object v = list.get(i);
        sink();
        list.set(j, (T) v);
    }

    static void sink() {
    }
}
