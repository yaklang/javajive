// Reviewed semantic equivalent of the generated major-49 classfile.
public final class NaturalSingleEntry {
    public static int run(int entry, int budget) {
        int acc = entry;
        while (budget > 0) {
            acc += budget;
            budget--;
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
