package bench;

// Self-hosted integer state machine. Exercises switch with fall-through, default, nested loops
// and array indexing so the round-trip test can compare control-flow-heavy output byte for byte.
public class SwitchFSM {
    public static int run(byte[] input) {
        int state = 0;
        int acc = 0;
        for (int i = 0; i < input.length; i++) {
            int b = input[i] & 0xff;
            switch (state) {
                case 0:
                    if (b == 'a') {
                        state = 1;
                        break;
                    }
                    acc += b;
                    state = 0;
                    break;
                case 1:
                    if (b == 'b') {
                        state = 2;
                        break;
                    }
                    acc += 3 * b;
                    state = 0;
                    break;
                case 2:
                    switch (b) {
                        case 'c':
                            acc += 100;
                            state = 3;
                            break;
                        case 'x':
                        case 'y':
                            acc += 7;
                            state = 0;
                            break;
                        default:
                            acc += b;
                            state = 0;
                            break;
                    }
                    break;
                case 3:
                    acc ^= b;
                    state = (b & 1) == 0 ? 0 : 1;
                    break;
                default:
                    state = 0;
                    break;
            }
        }
        return (acc << 4) ^ state;
    }

    public static void main(String[] args) {
        String[] in = {"", "a", "ab", "abc", "abx", "aby", "abcabc", "The quick abcabx brown fox"};
        for (int i = 0; i < in.length; i++) {
            System.out.println(in[i].length() + ":" + run(in[i].getBytes()));
        }
    }
}
