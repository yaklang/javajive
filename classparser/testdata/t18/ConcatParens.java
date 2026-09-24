public class ConcatParens {
  public static void main(String[] a) {
    int n = 3;
    boolean c = true;
    System.out.println("a" + (n & 0xff) + "b" + (n << 2) + "c" + (n >> 1) + "d" + (n >>> 1) + "e" + (n | 1) + "f" + (n ^ 1) + "g" + (c ? n : 0));
  }
}
