// Reviewed semantic equivalent of the generated major-49 classfile.
// "value" is the local merged at the original mid-body entry M.
public final class IrreducibleMidBodyPhi {
    public static int run(int entry, int budget) {
        int acc = entry;
        int value = entry;
        boolean enterAtHeader = entry == 0;
        while (budget > 0) {
            if (enterAtHeader) {
                value = acc + 2;
            }
            acc += value;
            budget--;
            enterAtHeader = true;
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
