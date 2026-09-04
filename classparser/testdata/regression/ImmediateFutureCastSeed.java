// Regression seed for enclosingTypeVarArgCast on Futures.immediateFuture
// (JDEC_ENCLOSING_TYPEVAR_ARG_CAST_OFF). A ternary
// `set((V) obj) ? futureValue : Futures.immediateFuture(obj)` infers the false
// arm as Future<Object> against Future<V> ("bad type in conditional expression").
// The source's `(V)` on immediateFuture erases; re-emit it. Real hit: guava
// LocalCache$LoadingValueReference.loadFuture.
// Recompile: javac --release 8 -d . ImmediateFutureCastSeed.java
import java.util.concurrent.Future;

public class ImmediateFutureCastSeed<V> {
    static class Futures {
        static <V> Future<V> immediateFuture(V value) {
            return null;
        }
    }

    Future<V> futureValue;

    boolean set(V v) {
        return false;
    }

    Future<V> load() {
        Object obj = null;
        return this.set((V) obj) ? this.futureValue : Futures.immediateFuture((V) obj);
    }
}
