package bench;

// for-loop whose continue must still run the increment. Exercises the T5 shape
// (continue-to-latch) so a do-while reconstruction that drops the increment
// would change the fingerprint.
public class ForContinue {
    public static int scan(String s) {
        int n = 0;
        int escaped = 0;
        for (int i = 0; i < s.length(); i++) {
            char c = s.charAt(i);
            if (c == '\\') {
                if (i + 1 < s.length()) {
                    escaped += s.charAt(i + 1);
                    i++;
                }
                continue;
            }
            if (c == '"') {
                n++;
                continue;
            }
            if (c == '\n') {
                continue;
            }
            n += c;
        }
        return n + 31 * escaped;
    }

    public static void main(String[] args) {
        String[] in = {"", "abc", "a\"b", "a\\nb", "\\\"x\ny", "\\\\", "end\\"};
        for (int i = 0; i < in.length; i++) {
            System.out.println(in[i] + "=" + scan(in[i]));
        }
    }
}
