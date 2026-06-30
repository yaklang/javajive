public class ArrCompoundSeed {
    int[] pathIndices = new int[32];
    int stackSize = 1;
    void bump() {
        this.pathIndices[this.stackSize - 1]++;
    }
    int read(int[] a, int i) {
        return a[i + 1]++;
    }
}
