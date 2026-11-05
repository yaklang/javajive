import java.util.*;
import java.util.function.*;

public class Legacy {
    enum Op {
        PLUS { int apply(int a, int b) { return a + b; } },
        TIMES { int apply(int a, int b) { return a * b; } };
        abstract int apply(int a, int b);
    }

    interface Formula {
        int apply(int x);
        default int twice(int x) { return apply(apply(x)); }
        static int id(int x) { return x; }
    }

    static class Impl implements Formula {
        public int apply(int x) { return x + 1; }
    }

    static class Resource implements AutoCloseable {
        final StringBuilder trace;
        Resource(StringBuilder trace) { this.trace = trace; trace.append("open"); }
        public void close() { trace.append("close"); }
        int use(int x) { if (x < 0) throw new IllegalStateException("neg"); return x * 2; }
    }

    static int switchInLoop(int n) {
        int acc = 0;
        for (int i = 0; i < n; i++) {
            switch (i % 5) {
                case 0: acc += 1; break;
                case 1:
                case 2: acc += 10; break;
                case 3: acc += 100; break;
                default: acc -= 2;
            }
            acc += 1000;
        }
        return acc;
    }

    static int stringSwitch(String s) {
        switch (s) {
            case "alpha": return 1;
            case "beta": return 2;
            default: return 0;
        }
    }

    static String twr(int x) {
        StringBuilder sb = new StringBuilder();
        try (Resource r = new Resource(sb)) {
            sb.append("use").append(r.use(x));
        } catch (IllegalStateException e) {
            sb.append("caught");
        } finally {
            sb.append("done");
        }
        return sb.toString();
    }

    static int lambda(int base) {
        Function<Integer, Integer> f = x -> x + base;
        Supplier<String> s = () -> "s" + base;
        return f.apply(2) + s.get().length();
    }

    static int anon(final int x) {
        Formula f = new Formula() {
            public int apply(int y) { return y * x; }
        };
        Runnable r = new Runnable() { public void run() { } };
        r.run();
        return f.apply(3);
    }

    static int generics(List<? extends Number> xs) {
        int acc = 0;
        for (Number n : xs) acc += n.intValue();
        return acc;
    }

    static int enumSwitch(Op op, int a, int b) { return op.apply(a, b); }

    static int nested(int n) {
        int acc = 0;
        outer:
        for (int i = 0; i < n; i++) {
            for (int j = 0; j < n; j++) {
                if ((i + j) % 7 == 0) continue outer;
                if (i * j > 9) break outer;
                acc += i * 3 + j;
            }
            acc--;
        }
        return acc;
    }

    public static void main(String[] args) {
        System.out.println("switch=" + switchInLoop(12));
        System.out.println("string=" + stringSwitch("beta") + stringSwitch("x"));
        System.out.println("twr=" + twr(3) + ":" + twr(-1));
        System.out.println("lambda=" + lambda(5));
        System.out.println("anon=" + anon(4));
        System.out.println("gen=" + generics(Arrays.asList(1, 2, 3)));
        System.out.println("enum=" + enumSwitch(Op.TIMES, 3, 4));
        System.out.println("nested=" + nested(6));
        System.out.println("iface=" + new Impl().twice(3) + ":" + Formula.id(9));
        try {
            Op missing = Op.valueOf("NOPE");
            System.out.println(missing);
        } catch (IllegalArgumentException e) {
            System.out.println("valueof");
        }
    }
}
