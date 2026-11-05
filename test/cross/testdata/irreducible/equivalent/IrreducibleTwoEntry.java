// Reviewed semantic equivalent of the generated major-49 classfile.
// The source uses a state variable because Java cannot directly spell the
// original bytecode's two-entry A <-> B strongly connected component.
public final class IrreducibleTwoEntry {
    public static int run(int entry, int budget) {
        int acc = entry;
        boolean atA = entry == 0;
        while (budget > 0) {
            acc += atA ? 3 : 5;
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
