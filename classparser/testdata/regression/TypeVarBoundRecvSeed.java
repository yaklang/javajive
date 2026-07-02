// Seed for receiver type-variable bound resolution (JDEC_TYPEVAR_BOUND_RECV_OFF).
//
// The receiver `var1` has static type C, a bare METHOD-scope type variable whose bound is the
// parameterized container `Collection<? super E>`. In bytecode C erases to its bound's erasure (raw
// Collection) and `it.next()` erases to Object, so the decompiler renders `var1` typed C and the arg as
// Object; javac -- re-resolving add against C's bound Collection<? super E> -- rejects the Object arg
// ("Object cannot be converted to CAP#1"). Mirrors guava FluentIterable.copyInto `var1.add(var3.next())`.
// receiverParamTypeArgs now recovers C's parameterized bound (Collection<? super E>) from the method's
// formal type-parameter section, which lets the wildcard `Collection` receiver fix re-emit a raw
// `((Collection)(var1))` cast (an unchecked, behaviour-preserving add(Object)). Single-class reproducible
// because the bound is a JDK type resolvable without sibling-jar context.
import java.util.Collection;
import java.util.Iterator;

public class TypeVarBoundRecvSeed<E> {
    <C extends Collection<? super E>> C copyInto(Iterator<? extends E> it, C dest) {
        while (it.hasNext()) {
            dest.add(it.next());
        }
        return dest;
    }
}
