public class SlotReuse {
  public static void main(String[] args) {
    java.util.function.IntUnaryOperator f;
    {
      int captured = args.length + 2;
      f = x -> x * captured;
    }
    System.out.println(f.applyAsInt(3));
    String later = "reused-slot";
    System.out.println(later);
  }
}
