public class ConcatSide {
  public static void main(String[] a) {
    try {
      System.out.println("[" + new Counter() + new Counter() + new Boom() + "]");
    } catch (RuntimeException e) {
      System.out.println("ex:" + e.getMessage());
      System.out.println("count:" + Counter.n);
    }
  }
}
