// Regression seed for sameClassMethodParamType Class<L> parameterized formal:
// A same-class instance method has a generic Signature formal `Class<L>` (L the class type
// variable). When called from another instance method with a raw `Class` argument (from
// getComponentType's erased descriptor return), the source's unchecked `(Class<L>)` cast is
// erased to a no-op checkcast. Mirrors commons-lang3 EventListenerSupport.readObject ->
// initializeTransientFields(Class<L>, ClassLoader).
//
// Recompile: javac -d . ClassTypeVarParamSeed.java
public class ClassTypeVarParamSeed<L> {
    private Class<L> type;

    private void setType(Class<L> type) {
        this.type = type;
    }

    @SuppressWarnings("unchecked")
    public void init(Object[] array) {
        this.setType((Class<L>) array.getClass().getComponentType());
    }
}