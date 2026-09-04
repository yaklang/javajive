// Regression seed for addBreakToSwitchCases throwing-default force-break: independent
// `n = n + k` case bodies fall through into `default: throw`, making the post-switch
// statement unreachable. Mirrors spring ASM ClassReader opcode-size switch.
// Recompile: javac --release 8 -d . SwitchThrowingDefaultBreakSeed.java
public class SwitchThrowingDefaultBreakSeed {
    public static int size(int op) {
        int n = 0;
        int i = 0;
        while (i < 1) {
            switch (op) {
                case 16:
                case 18:
                    n = n + 2;
                    break;
                case 17:
                case 19:
                    n = n + 3;
                    break;
                case 185:
                    n = n + 5;
                    break;
                default:
                    throw new IllegalArgumentException();
            }
            i++;
        }
        return n;
    }
}
