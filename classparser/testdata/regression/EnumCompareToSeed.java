// Seed for jdkCalleeParamIsErasedTypeVar (guava AbstractService/ServiceManager
// `state().compareTo(State.RUNNING)`): a call to java.lang.Enum<E>.compareTo(E). The method descriptor
// erases E to its bound `java.lang.Enum`, so the arg-cast logic upcasts the concrete enum constant to
// raw `Enum` -- `compareTo((Enum) State.RUNNING)` -- which breaks compareTo's real signature
// compareTo(E) ("Enum cannot be converted to <ConcreteEnum>"). The fix drops that no-op upcast.
public class EnumCompareToSeed {
    enum State {
        NEW,
        RUNNING,
        DONE
    }

    static boolean reachedRunning(State s) {
        return s.compareTo(State.RUNNING) >= 0;
    }

    private EnumCompareToSeed() {
    }
}
