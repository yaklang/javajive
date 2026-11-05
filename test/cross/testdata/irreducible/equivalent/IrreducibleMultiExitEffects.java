// Reviewed semantic equivalent of the generated major-49 classfile.
public final class IrreducibleMultiExitEffects {
    public static int effects;

    public static int run(int entry, int budget) {
        effects = 0;
        int acc = entry;
        boolean atA = entry == 0;
        while (budget > 0) {
            if (atA) {
                effects += 1;
                acc += 2;
                budget--;
                if (budget <= 0) {
                    return acc + effects;
                }
                if ((acc & 7) == 0) {
                    return acc + 1000 + effects;
                }
            } else {
                effects += 10;
                acc += 3;
                budget--;
                if (budget <= 0) {
                    return acc + effects;
                }
                if ((acc & 3) == 0) {
                    return acc + 2000 + effects;
                }
            }
            atA = !atA;
        }
        return acc + effects;
    }

    public static void main(String[] args) {
        int[] entries = {0, 0, 1, 2, 3, -1};
        int[] budgets = {0, 1, 2, 3, 5, 8};
        for (int i = 0; i < entries.length; i++) {
            System.out.println(run(entries[i], budgets[i]));
        }
    }
}
