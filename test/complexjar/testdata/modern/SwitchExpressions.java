public class SwitchExpressions {
    public static String strings(int x) {
        return switch (x) {
            case 0 -> "zero";
            case 1 -> "one";
            case 2 -> "two";
            default -> "other";
        };
    }

    public static int ints(int x) {
        return switch (x) {
            case 0 -> 10;
            case 1 -> { yield 20; }
            default -> 30;
        };
    }

    public static int locals(int x) {
        int r;
        switch (x) {
            case 0: r = 10; break;
            case 1: r = 20; break;
            default: r = 30;
        }
        return r;
    }

    public static String nested(int x) {
        return switch (x) {
            case 0 -> switch (x + 1) { case 1 -> "one"; default -> "n"; };
            default -> "d";
        };
    }

    public static int block(int x) {
        return switch (x) {
            case 0 -> {
                int y = x + 1;
                y *= 3;
                yield y;
            }
            default -> -1;
        };
    }

    public static void main(String[] args) {
        for (int i = -1; i < 4; i++) {
            System.out.println(strings(i) + "," + ints(i) + "," + locals(i) + "," + nested(i) + "," + block(i));
        }
    }
}
