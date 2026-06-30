package bench;

// Self-hosted quicksort + binary search over a deterministic LCG-generated array. Exercises recursion,
// array index arithmetic, in-place swaps and comparison branches. main() prints the sorted array and a
// few search results so the round-trip test can compare control-flow-heavy output byte for byte.
public class QuickSort {
    public static void sort(int[] a, int lo, int hi) {
        if (lo >= hi) {
            return;
        }
        int pivot = a[(lo + hi) >>> 1];
        int i = lo, j = hi;
        while (i <= j) {
            while (a[i] < pivot) {
                i++;
            }
            while (a[j] > pivot) {
                j--;
            }
            if (i <= j) {
                int t = a[i];
                a[i] = a[j];
                a[j] = t;
                i++;
                j--;
            }
        }
        sort(a, lo, j);
        sort(a, i, hi);
    }

    public static int binarySearch(int[] a, int key) {
        int lo = 0, hi = a.length - 1;
        while (lo <= hi) {
            int mid = (lo + hi) >>> 1;
            if (a[mid] == key) {
                return mid;
            } else if (a[mid] < key) {
                lo = mid + 1;
            } else {
                hi = mid - 1;
            }
        }
        return -1;
    }

    public static void main(String[] args) {
        int n = 200;
        int[] a = new int[n];
        long seed = 0x5DEECE66DL;
        for (int i = 0; i < n; i++) {
            seed = (seed * 0x5DEECE66DL + 0xBL) & ((1L << 48) - 1);
            a[i] = (int) (seed >>> 16) % 1000;
        }
        sort(a, 0, n - 1);
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < n; i++) {
            sb.append(a[i]);
            sb.append(i + 1 < n ? "," : "");
        }
        System.out.println(sb.toString());
        int[] keys = {a[0], a[n / 2], a[n - 1], -12345};
        for (int k : keys) {
            System.out.println(k + " -> " + binarySearch(a, k));
        }
    }
}
