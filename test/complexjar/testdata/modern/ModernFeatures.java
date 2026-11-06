import java.lang.annotation.*;
import java.util.*;
import java.util.function.*;

public class ModernFeatures {
    @Retention(RetentionPolicy.RUNTIME)
    @Target({ElementType.TYPE_USE, ElementType.METHOD, ElementType.FIELD})
    public @interface Mark { String value() default "m"; }

    public sealed interface Shape permits Circle, Square, Rect {}
    public record Circle(double radius) implements Shape {
        public Circle { if (radius < 0) throw new IllegalArgumentException("r"); }
    }
    public record Square(double side) implements Shape {}
    public static final class Rect implements Shape {
        final double w, h;
        Rect(double w, double h) { this.w = w; this.h = h; }
    }

    public record Pair<A extends Comparable<A>, B>(A first, B second) {
        public String describe() { return first + ":" + second; }
    }

    public enum Op {
        PLUS { int apply(int a, int b) { return a + b; } },
        TIMES { int apply(int a, int b) { return a * b; } };
        abstract int apply(int a, int b);
    }

    public interface WithPrivate {
        default int twice(int x) { return add(x, x); }
        private int add(int a, int b) { return a + b; }
        static int identity(int x) { return x; }
    }

    static final class Impl implements WithPrivate {}

    public static int patternShape(Shape s) {
        return switch (s) {
            case Circle c when c.radius() > 0 -> (int) (c.radius() * 2);
            case Circle c -> -1;
            case Square q -> (int) (q.side() * 4);
            case Rect r -> (int) (r.w + r.h);
        };
    }

    public static String switchExpr(int x) {
        return switch (x & 3) {
            case 0 -> "zero";
            case 1 -> { yield "one"; }
            case 2 -> "two";
            default -> "three";
        };
    }

    public static String stringSwitch(String s) {
        return switch (s) { case "a" -> "A"; case "b" -> "B"; default -> "?"; };
    }

    public static int enumSwitch(Op op, int a, int b) { return op.apply(a, b); }

    public static String textBlock(int x) {
        String t = """
            value=%d
            line2
            """;
        return t.formatted(x).replace("\n", "|");
    }

    public static int instancePattern(Object o) {
        if (o instanceof String s && s.length() > 2) return s.length();
        if (o instanceof Integer i) return i;
        return -1;
    }

    public static String lambdas(int base) {
        Function<Integer, Integer> f = x -> x + base;
        Function<Integer, Integer> g = ModernFeatures::square;
        Supplier<String> s = () -> "s" + base;
        return f.apply(2) + "," + g.apply(3) + "," + s.get();
    }

    static int square(int x) { return x * x; }

    public static String resources(int x) throws Exception {
        final StringBuilder sb = new StringBuilder();
        try (AutoCloseable c = () -> sb.append("close"); AutoCloseable d = () -> sb.append("d")) {
            if (x < 0) throw new IllegalStateException("neg");
            sb.append("body").append(x);
        } catch (IllegalArgumentException | IllegalStateException e) {
            sb.append("catch").append(e.getMessage());
        } finally {
            sb.append("finally");
        }
        return sb.toString();
    }

    public static int labeled(int n) {
        int acc = 0;
        outer:
        for (int i = 0; i < n; i++) {
            for (int j = 0; j < n; j++) {
                if ((i + j) % 5 == 0) continue outer;
                if (i * j > 4) break outer;
                acc += i + j;
            }
            acc++;
        }
        return acc;
    }

    public static String inner(int x) {
        class Local { int v; Local(int v) { this.v = v; } int get() { return v; } }
        Runnable r = new Runnable() { public void run() { } };
        r.run();
        return new Local(x).get() + ":" + new Object() { public String toString() { return "anon"; } };
    }

    public static synchronized int sync(int x) { return x + 1; }

    public static <T extends Comparable<T>> T max(List<T> xs) {
        T best = null;
        for (T x : xs) if (best == null || x.compareTo(best) > 0) best = x;
        return best;
    }

    static int sum(int... xs) { int s = 0; for (int x : xs) s += x; return s; }

    public static int arrays(int x) {
        int[][] a = new int[3][2];
        for (int i = 0; i < 3; i++) for (int j = 0; j < 2; j++) a[i][j] = i + j + x;
        return a[2][1] + a.length;
    }

    public static int methodHandle(int x) throws Throwable {
        java.lang.invoke.MethodHandle mh = java.lang.invoke.MethodHandles.lookup().findStatic(
            ModernFeatures.class, "square", java.lang.invoke.MethodType.methodType(int.class, int.class));
        return (int) mh.invokeExact(x);
    }

    public static void main(String[] args) throws Throwable {
        System.out.println(patternShape(new Circle(2)));
        System.out.println(patternShape(new Square(3)));
        System.out.println(patternShape(new Rect(4, 5)));
        System.out.println(switchExpr(1));
        System.out.println(stringSwitch("b"));
        System.out.println(enumSwitch(Op.TIMES, 3, 4));
        System.out.println(textBlock(7));
        System.out.println(instancePattern("abcd"));
        System.out.println(lambdas(5));
        System.out.println(resources(2));
        System.out.println(labeled(5));
        System.out.println(inner(9));
        System.out.println(sync(1));
        System.out.println(max(List.of(3, 7, 2)));
        System.out.println(sum(1, 2, 3, 4));
        System.out.println(arrays(1));
        System.out.println(methodHandle(5));
        System.out.println(new Pair<>("a", 2).describe());
        System.out.println(new Pair<>("a", 2).equals(new Pair<>("a", 2)));
        System.out.println(new Impl().twice(4));
    }
}
