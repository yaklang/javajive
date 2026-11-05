// Reviewed semantic equivalent of the generated major-49 classfile.
// State 0 is the natural outer header; states 1 and 2 are the two-entry A/B
// region nested inside it.
public final class RegionalIrreducibility {
    public static int run(int entry, int budget) {
        int acc = entry;
        int state = 0;
        while (budget > 0) {
            if (state == 0) {
                state = (acc & 1) == 0 ? 1 : 2;
            } else if (state == 1) {
                acc += 2;
                budget--;
                state = (budget & 1) == 0 ? 0 : 2;
            } else {
                acc += 5;
                budget--;
                state = (budget & 1) == 0 ? 1 : 0;
            }
        }
        return acc;
    }

    public static void main(String[] args) {
        int[] entries = {0, 0, 1, 2, 3, -1};
        int[] budgets = {0, 1, 2, 3, 5, 8};
        for (int i = 0; i < entries.length; i++) {
            System.out.println(run(entries[i], budgets[i]));
        }
    }
}
