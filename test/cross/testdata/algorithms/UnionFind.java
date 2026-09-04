package bench;

// Self-hosted union-find with path compression and union-by-rank. Exercises nested classes,
// recursion, array mutation and a deterministic Kruskal-style clustering so the round-trip
// test can compare structure-heavy output byte for byte.
public class UnionFind {
    public static class DSU {
        private final int[] p;
        private final int[] r;
        private int parts;

        public DSU(int n) {
            p = new int[n];
            r = new int[n];
            parts = n;
            for (int i = 0; i < n; i++) {
                p[i] = i;
            }
        }

        public int find(int x) {
            if (p[x] != x) {
                p[x] = find(p[x]);
            }
            return p[x];
        }

        public boolean union(int a, int b) {
            int ra = find(a);
            int rb = find(b);
            if (ra == rb) {
                return false;
            }
            if (r[ra] < r[rb]) {
                p[ra] = rb;
            } else if (r[ra] > r[rb]) {
                p[rb] = ra;
            } else {
                p[rb] = ra;
                r[ra]++;
            }
            parts--;
            return true;
        }

        public int components() {
            return parts;
        }
    }

    public static void main(String[] args) {
        int n = 32;
        DSU d = new DSU(n);
        long seed = 0x5DEECE66DL;
        int unions = 0;
        for (int i = 0; i < 64; i++) {
            seed = (seed * 0x5DEECE66DL + 0xBL) & ((1L << 48) - 1);
            int a = (int) ((seed >>> 16) % n);
            seed = (seed * 0x5DEECE66DL + 0xBL) & ((1L << 48) - 1);
            int b = (int) ((seed >>> 16) % n);
            if (d.union(a, b)) {
                unions++;
            }
        }
        int xor = 0;
        for (int i = 0; i < n; i++) {
            xor ^= d.find(i) * (i + 1);
        }
        System.out.println(unions);
        System.out.println(d.components());
        System.out.println(xor);
    }
}
