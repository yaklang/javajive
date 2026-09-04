package bench;

// Self-hosted Knuth–Morris–Pratt matcher. Exercises prefix-table construction, nested loops
// and failure-function backtracking so the round-trip test can compare search output byte for byte.
public class KMP {
    public static int[] prefix(String pat) {
        int n = pat.length();
        int[] pi = new int[n];
        int k = 0;
        for (int i = 1; i < n; i++) {
            while (k > 0 && pat.charAt(k) != pat.charAt(i)) {
                k = pi[k - 1];
            }
            if (pat.charAt(k) == pat.charAt(i)) {
                k++;
            }
            pi[i] = k;
        }
        return pi;
    }

    public static int count(String text, String pat) {
        if (pat.length() == 0) {
            return text.length() + 1;
        }
        int[] pi = prefix(pat);
        int q = 0;
        int hits = 0;
        for (int i = 0; i < text.length(); i++) {
            while (q > 0 && pat.charAt(q) != text.charAt(i)) {
                q = pi[q - 1];
            }
            if (pat.charAt(q) == text.charAt(i)) {
                q++;
            }
            if (q == pat.length()) {
                hits++;
                q = pi[q - 1];
            }
        }
        return hits;
    }

    public static void main(String[] args) {
        String text = "ababcabcabababdababcabcabababd";
        String[] pats = {"ab", "abc", "abab", "ababd", "xyz", "a", text};
        int sum = 0;
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < pats.length; i++) {
            int c = count(text, pats[i]);
            sum += c;
            if (i > 0) {
                sb.append(',');
            }
            sb.append(c);
        }
        System.out.println(sum);
        System.out.println(sb);
    }
}
