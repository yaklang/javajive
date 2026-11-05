// Reviewed semantic equivalent of the generated major-49 classfile.
// The original tableswitch enters A, B, or C directly; state preserves that
// behavior in structured Java.
public final class IrreducibleSwitchThreeEntry {
    public static int run(int entry, int budget) {
        int acc = entry;
        int state = entry == 1 ? 1 : entry == 2 ? 2 : 0;
        while (budget > 0) {
            switch (state) {
                case 0:
                    acc += 1;
                    break;
                case 1:
                    acc += 10;
                    break;
                default:
                    acc += 100;
                    break;
            }
            budget--;
            state = (state + 1) % 3;
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
