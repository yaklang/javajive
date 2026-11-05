// Reviewed semantic equivalent of the generated major-49 classfile.
// The original bytecode also contains an unreachable D -> B edge.
public final class DeadSecondaryEntry {
    public static int run(int entry, int budget) {
        int acc = entry;
        boolean atA = true;
        while (budget > 0) {
            acc += atA ? 2 : 3;
            budget--;
            atA = !atA;
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
