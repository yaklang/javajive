public class BoolIntOperandCmpSeed {
    boolean flag;
    boolean check(Object p) {
        int n = (p == null) ? 1 : 0;
        return n != this.flag;
    }
}
