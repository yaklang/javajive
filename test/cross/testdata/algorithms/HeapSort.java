package bench;

// Self-hosted heapsort over a deterministic LCG-generated array. Exercises nested loops, index
// arithmetic, in-place swaps and the sift-down invariant. main() prints the sorted array and a
// checksum so the round-trip test can compare control-flow-heavy output byte for byte.
public class HeapSort {
    public static void sort(int[] a) {
        int n = a.length;
        for (int i = (n >>> 1) - 1; i >= 0; i--) {
            sift(a, n, i);
        }
        for (int end = n - 1; end > 0; end--) {
            int t = a[0];
            a[0] = a[end];
            a[end] = t;
            sift(a, end, 0);
        }
    }

    private static void sift(int[] a, int n, int root) {
        while (true) {
            int left = (root << 1) + 1;
            if (left >= n) {
                return;
            }
            int largest = root;
            if (a[left] > a[largest]) {
                largest = left;
            }
            int right = left + 1;
            if (right < n && a[right] > a[largest]) {
                largest = right;
            }
            if (largest == root) {
                return;
            }
            int t = a[root];
            a[root] = a[largest];
            a[largest] = t;
            root = largest;
        }
    }

    public static void main(String[] args) {
        int n = 200;
        int[] a = new int[n];
        long seed = 0x5DEECE66DL;
        for (int i = 0; i < n; i++) {
            seed = (seed * 0x5DEECE66DL + 0xBL) & ((1L << 48) - 1);
            a[i] = (int) (seed >>> 16);
        }
        sort(a);
        int sum = 0;
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < n; i++) {
            if (i > 0) {
                sb.append(',');
            }
            sb.append(a[i]);
            sum += a[i];
            if (i > 0 && a[i] < a[i - 1]) {
                throw new AssertionError("not sorted at " + i);
            }
        }
        System.out.println(sum);
        System.out.println(sb);
    }
}
