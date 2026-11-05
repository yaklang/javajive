package accept.gate;

import accept.ArrayClone;

/** Reviewed behavioral oracle for the accepted array-clone fixture. */
public final class ArrayCloneDriver {
  public static void main(String[] args) {
    int[] original = new int[]{3, 5, 8};
    int[] copy = (int[]) ArrayClone.cloneArray(original);
    copy[0] = 13;

    boolean nullThrows = false;
    try {
      ArrayClone.cloneArray(null);
    } catch (NullPointerException expected) {
      nullThrows = true;
    }

    System.out.println("array-clone:" + (copy != original)
        + ":" + original[0] + ":" + copy[0]
        + ":" + copy[1] + ":" + copy[2] + ":" + nullThrows);
  }
}
