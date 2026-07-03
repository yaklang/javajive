public class TernaryClassLitSeed {
    private Class superclass;

    // The ternary `cond ? Object.class : classField` is a java.lang.Class value. A class-literal arm's
    // Type() reports the REFERENCED class (Object) rather than java.lang.Class, so the naive arm-merge
    // collapses to Object and the capturing local would be declared `Object c`, making the later
    // c.getModifiers()/c.getName() fail to recompile. The fix (JDEC_NO_CLASSLIT_SLOT_TYPE) keeps the
    // class-literal arm typed as java.lang.Class so the local is declared `Class c`.
    public String probe() {
        Class c = (this.superclass == null) ? Object.class : this.superclass;
        return c.getName() + c.getModifiers();
    }
}
