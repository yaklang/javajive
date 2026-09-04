package bench;

// Self-hosted try/catch/finally with returns from every arm. Exercises exception edges, a
// shared accumulator mutated in finally, and nested try so the round-trip test can compare
// exception-control-flow output byte for byte.
public class TryFinally {
    public static int compute(int n) {
        int acc = 0;
        try {
            if (n < 0) {
                throw new IllegalArgumentException("neg");
            }
            try {
                acc += n;
                if ((n & 1) == 1) {
                    throw new ArithmeticException("odd");
                }
                return acc + 10;
            } catch (ArithmeticException e) {
                acc += 100;
                return acc;
            } finally {
                acc += 1;
            }
        } catch (IllegalArgumentException e) {
            return -n;
        } finally {
            acc += 1000;
        }
    }

    public static String classify(int n) {
        StringBuilder sb = new StringBuilder();
        try {
            int v = compute(n);
            sb.append("ok:").append(v);
            return sb.toString();
        } catch (RuntimeException e) {
            sb.append("ex:").append(e.getClass().getSimpleName());
            return sb.toString();
        } finally {
            sb.append("|done");
        }
    }

    public static void main(String[] args) {
        int[] in = {0, 1, 2, 3, -4, 8, 9};
        for (int i = 0; i < in.length; i++) {
            System.out.println(in[i] + "=" + compute(in[i]) + "/" + classify(in[i]));
        }
    }
}
