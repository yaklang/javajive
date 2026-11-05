import java.io.*;
import java.util.*;
import java.util.function.*;

public class ControlFlow {
    static final StringBuilder TRACE = new StringBuilder();

    static class Boom extends RuntimeException {
        final int code;
        Boom(int code) { super("boom" + code); this.code = code; }
    }

    static class Resource implements AutoCloseable {
        final String name;
        Resource(String name) { this.name = name; TRACE.append("open").append(name); }
        public void close() { TRACE.append("close").append(name); }
        int use(int x) { if (x < 0) throw new Boom(x); return x * 2; }
    }

    static int nested(int n) {
        int acc = 0;
        outer:
        for (int i = 0; i < n; i++) {
            for (int j = 0; j < n; j++) {
                if ((i + j) % 7 == 0) continue outer;
                if (i * j > 9) break outer;
                if ((i ^ j) == 3) {
                    acc += 100;
                    continue;
                }
                acc += i * 3 + j;
            }
            acc--;
        }
        return acc;
    }

    static int switchInLoop(int n) {
        int acc = 0;
        for (int i = 0; i < n; i++) {
            switch (i % 7) {
                case 0:
                    acc += 1;
                    break;
                case 1:
                case 2:
                    acc += 10;
                    break;
                case 3: {
                    if (i > 5) break;
                    acc += 100;
                    break;
                }
                case 4:
                    continue;
                default:
                    acc -= 2;
            }
            acc += 1000;
        }
        return acc;
    }

    static String tryFinally(int x) {
        TRACE.setLength(0);
        int out;
        try {
            if (x == 0) return "zero";
            if (x < 0) throw new Boom(x);
            out = 10 / x;
            TRACE.append("body");
        } catch (Boom b) {
            TRACE.append("boom").append(b.code);
            out = -1;
            return "caught";
        } catch (ArithmeticException e) {
            TRACE.append("arith");
            out = -2;
        } finally {
            TRACE.append("finally");
        }
        return TRACE + ":" + out;
    }

    static String resources(int x) {
        TRACE.setLength(0);
        try (Resource a = new Resource("A"); Resource b = new Resource("B")) {
            TRACE.append("use").append(a.use(x)).append(b.use(x + 1));
            if (x == 3) throw new Boom(3);
        } catch (Boom e) {
            TRACE.append("caught").append(e.code);
        } finally {
            TRACE.append("done");
        }
        return TRACE.toString();
    }

    static int denseSwitch(int x) {
        switch (x) {
            case -2: return 20;
            case -1: return 10;
            case 0: return 0;
            case 1: return 11;
            case 2: return 22;
            case 3: return 33;
            case 4: return 44;
            case 5: return 55;
            default: return -1;
        }
    }

    static int sparseSwitch(int x) {
        switch (x) {
            case 1: return 1;
            case 100: return 2;
            case 10000: return 3;
            case 1000000: return 4;
            default: return 0;
        }
    }

    static int fallthrough(int x) {
        int acc = 0;
        switch (x) {
            case 0:
                acc += 1;
            case 1:
                acc += 2;
            case 2:
                acc += 4;
                break;
            case 3:
                acc += 8;
            default:
                acc += 16;
        }
        return acc;
    }

    static int strings(String s) {
        switch (s) {
            case "alpha": return 1;
            case "beta": return 2;
            case "gamma": return 3;
            default: return 0;
        }
    }

    static int ternaryChain(int a, int b, int c) {
        return a > 0 ? (b > 0 ? (c > 0 ? 7 : 6) : (c > 0 ? 5 : 4)) : (b > 0 ? (c > 0 ? 3 : 2) : (c > 0 ? 1 : 0));
    }

    static int shortCircuit(int a, int b) {
        int acc = 0;
        if (a > 0 && b > 0 && a + b < 10) acc++;
        if (a < 0 || b < 0 || a * b > 100) acc += 2;
        if (!(a == b) && (a != 0 || b != 0)) acc += 4;
        return acc;
    }

    static int exceptionLoop(int n) {
        int acc = 0;
        for (int i = 0; i < n; i++) {
            try {
                if (i % 3 == 0) throw new Boom(i);
                acc += i;
            } catch (Boom e) {
                if (e.code == 6) continue;
                acc -= e.code;
            } finally {
                acc++;
            }
        }
        return acc;
    }

    static int shape(int kind, int x) {
        switch (kind) {
            case 0:
                return switch (x) {
                    case 0 -> 100;
                    case 1 -> { int y = 2; yield y * 5; }
                    default -> -x;
                };
            case 1:
                return x > 0 ? x : -x;
            default:
                return 0;
        }
    }

    interface Formula {
        int apply(int x);
        default int twice(int x) { return apply(apply(x)); }
        private int helper(int x) { return x + 1; }
        static int id(int x) { return x; }
        default int viaHelper(int x) { return helper(x); }
    }

    enum Kind implements Formula {
        DOUBLE { public int apply(int x) { return x * 2; } },
        SQUARE { public int apply(int x) { return x * x; } };
        public abstract int apply(int x);
    }

    static int iface(int x) {
        return Kind.DOUBLE.twice(x) + Kind.SQUARE.apply(x) + Formula.id(x) + Kind.DOUBLE.viaHelper(x);
    }

    sealed interface Node permits Leaf, Branch {}
    record Leaf(int value) implements Node {}
    record Branch(Node left, Node right) implements Node {}

    static int sum(Node n) {
        return switch (n) {
            case Leaf leaf -> leaf.value();
            case Branch b -> sum(b.left()) + sum(b.right());
        };
    }

    static int patternInstance(Object o) {
        if (o instanceof Leaf l && l.value() > 3) return l.value();
        if (o instanceof Branch b) return sum(b);
        if (o instanceof String s) return s.length();
        return -1;
    }

    static int anonymousAndLocal(int x) {
        class Local implements Formula {
            public int apply(int y) { return y + x; }
        }
        Formula f = new Formula() {
            public int apply(int y) { return y * 3; }
        };
        return new Local().apply(2) + f.apply(4);
    }

    static int generics(List<? extends Number> xs) {
        int acc = 0;
        for (Number n : xs) acc += n.intValue();
        return acc;
    }

    static int arrays(int[][] m) {
        int acc = 0;
        for (int[] row : m) for (int v : row) acc = acc * 3 + v;
        return acc;
    }

    static void side(String s) { TRACE.append(s); }

    static int evaluationOrder(int x) {
        TRACE.setLength(0);
        int a = x + 1;
        side("a");
        int b = a * 2;
        side("b");
        int[] arr = new int[3];
        arr[0] = a;
        arr[1] = b;
        arr[2] = arr[0] + arr[1];
        side("c");
        return arr[2] + TRACE.length();
    }

    static int synchronizedBlock(int x) {
        Object lock = new Object();
        int acc = 0;
        synchronized (lock) { acc += x; }
        synchronized (ControlFlow.class) { acc += 2; }
        return acc;
    }

    static int recursion(int n) {
        if (n <= 1) return 1;
        return n * recursion(n - 1) + recursion(n - 2);
    }

    static int stateMachine(int input) {
        int state = 0;
        int acc = 0;
        while (true) {
            switch (state) {
                case 0:
                    acc = input;
                    state = 1;
                    break;
                case 1:
                    if (acc > 100) state = 2;
                    else if (acc > 0) state = 3;
                    else state = 4;
                    break;
                case 2:
                    acc /= 3;
                    state = 5;
                    break;
                case 3:
                    acc += 17;
                    state = 5;
                    break;
                case 4:
                    return -1;
                case 5:
                    if ((acc & 1) == 0) return acc;
                    acc--;
                    state = 5;
                    break;
                default:
                    return -2;
            }
        }
    }

    public static void main(String[] args) throws Exception {
        System.out.println("nested=" + nested(6));
        System.out.println("switchInLoop=" + switchInLoop(12));
        for (int x : new int[]{-2, -1, 0, 1, 2, 3, 100}) {
            System.out.println("try=" + x + ":" + tryFinally(x));
            System.out.println("res=" + x + ":" + resources(x));
        }
        for (int x : new int[]{-3, -2, -1, 0, 1, 2, 3, 4, 5, 6, 100}) {
            System.out.println("dense=" + x + ":" + denseSwitch(x) + ":" + sparseSwitch(x) + ":" + fallthrough(x));
        }
        System.out.println("strings=" + strings("alpha") + strings("beta") + strings("gamma") + strings("x"));
        int t = 0;
        for (int a : new int[]{-1, 0, 1}) for (int b : new int[]{-1, 0, 1}) for (int c : new int[]{-1, 0, 1}) {
            t = t * 3 + ternaryChain(a, b, c);
            t += shortCircuit(a, b) + shape(a + 1, b * 2);
        }
        System.out.println("t=" + t);
        System.out.println("exc=" + exceptionLoop(10));
        System.out.println("iface=" + iface(3));
        Node tree = new Branch(new Leaf(1), new Branch(new Leaf(2), new Leaf(4)));
        System.out.println("sum=" + sum(tree) + ":" + patternInstance(tree) + ":" + patternInstance("hello"));
        System.out.println("anon=" + anonymousAndLocal(5));
        System.out.println("gen=" + generics(List.of(1, 2, 3, 4)) + ":" + arrays(new int[][]{{1, 2}, {3, 4}}));
        System.out.println("order=" + evaluationOrder(5));
        System.out.println("sync=" + synchronizedBlock(8));
        System.out.println("rec=" + recursion(8));
        for (int x : new int[]{-5, 0, 7, 99, 101, 1000}) System.out.println("sm=" + x + ":" + stateMachine(x));
    }
}
