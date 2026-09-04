package bench;

import java.util.HashMap;
import java.util.Map;

// Hand-written computeIfAbsent: V x = map.get(k); if (x == null) { x = ...; map.put(k, x); }
// use x. This is the T4b definite-assignment shape (first store folded into the null check).
public class ComputeIfAbsent {
    public static int tally(int[] keys) {
        Map<Integer, Integer> m = new HashMap<Integer, Integer>();
        int sum = 0;
        for (int i = 0; i < keys.length; i++) {
            int k = keys[i];
            Integer v = m.get(Integer.valueOf(k));
            if (v == null) {
                v = Integer.valueOf(k * 3 + m.size());
                m.put(Integer.valueOf(k), v);
            }
            sum += v.intValue();
        }
        return sum + 17 * m.size();
    }

    public static void main(String[] args) {
        System.out.println("a=" + tally(new int[]{}));
        System.out.println("b=" + tally(new int[]{1, 2, 3}));
        System.out.println("c=" + tally(new int[]{1, 1, 2, 1, 3, 2}));
        System.out.println("d=" + tally(new int[]{7, 7, 7, 8, 0, -3, 8}));
    }
}
