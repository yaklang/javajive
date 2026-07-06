// Regression seed for the method-parameter free-type-var injection (dumper.go). An anonymous class created
// inside a static GENERIC method captures the method's type variables. javac emits the anonymous class's
// Signature WITHOUT a formal `<...>` section (it lists only the free-var refs), so a var referenced only in
// a private method PARAMETER (`I` in applyTransformation(I)) appears in no supertype/field and, before the
// fix, rendered undeclared ("cannot find symbol: class I"). Mirrors guava Futures$2 (Futures.transform).
//
// Recompile: javac -d . MethodParamTypeVarSeed.java
import java.util.concurrent.Future;
import java.util.concurrent.TimeUnit;

public class MethodParamTypeVarSeed {
    interface Fn<A, B> { B apply(A a); }

    static <I, O> Future<O> transform(final Future<I> input, final Fn<? super I, ? extends O> function) {
        return new Future<O>() {
            public boolean cancel(boolean b) { return input.cancel(b); }
            public boolean isCancelled() { return input.isCancelled(); }
            public boolean isDone() { return input.isDone(); }
            public O get() throws InterruptedException, java.util.concurrent.ExecutionException {
                return applyTransformation(input.get());
            }
            public O get(long t, TimeUnit u) throws InterruptedException, java.util.concurrent.ExecutionException, java.util.concurrent.TimeoutException {
                return applyTransformation(input.get(t, u));
            }
            private O applyTransformation(I in) {
                return function.apply(in);
            }
        };
    }
}
