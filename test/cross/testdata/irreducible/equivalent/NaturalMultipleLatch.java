// Reviewed semantic equivalent of the generated major-49 classfile.
public final class NaturalMultipleLatch {
    public static int run(int entry, int budget) {
        int acc = entry;
        while (budget > 0) {
            if ((budget & 1) == 0) {
                acc += 2;
            } else {
                acc += 3;
            }
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
