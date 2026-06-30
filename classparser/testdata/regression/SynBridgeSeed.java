// Seed for the synthetic access-bridge constructor `this()` delegation fix (guava
// AbstractFuture$UnsafeAtomicHelper / $SynchronizedHelper / AggregateFutureState$SynchronizedAtomicHelper).
//
// `Base` has a PRIVATE no-arg ctor; `Sub` extends it with its own PRIVATE no-arg ctor; `make()` in the
// enclosing class does `new Sub()`. Compiled with `--release 8` (pre-nestmates) javac synthesizes a
// marker anonymous class `SynBridgeSeed$1` and package-private access-bridge ctors `Base(SynBridgeSeed$1)`
// / `Sub(SynBridgeSeed$1)` whose bytecode is just `aload_0; invokespecial <init>:()V` (= `this()` to the
// private no-arg). When the decompiler strips that `this()` and the nested classes are flattened to
// top-level units, the empty-body bridge implicitly calls `super()` -> the PRIVATE `Base()` -> javac
// "constructor Base in class Base cannot be applied ... Base() has private access". The fix re-emits the
// faithful `this()` delegation.
public class SynBridgeSeed {
    private abstract static class Base {
        private Base() {
        }

        abstract void f();
    }

    private static final class Sub extends Base {
        private Sub() {
        }

        void f() {
        }
    }

    static Base make() {
        return new Sub();
    }
}
