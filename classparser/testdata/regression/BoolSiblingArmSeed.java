// Seed for reachingBoolSiblingArmMerge (gson SqlTypesSupport.<clinit> shape): a boolean local assigned
// `true` in a try body and `false` in the catch handler, then read after the try. javac compiles both
// assignments as iconst_1/iconst_0 + istore (int category) on DISJOINT arms, so the decompiler's DFS
// mints two int versions of the slot; the post-try read binds one, a later boolean USE re-types it, and
// the other arm splits off an int variable -> the read renders out of scope / `Object x = null`.
public class BoolSiblingArmSeed {
    public static final boolean SUPPORTED;

    static {
        boolean b;
        try {
            Class.forName("java.sql.Date");
            b = true;
        } catch (ClassNotFoundException e) {
            b = false;
        }
        SUPPORTED = b;
    }

    private BoolSiblingArmSeed() {
    }
}
