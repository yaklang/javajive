public class NestedAlpha {
  public static boolean alphaMethod() {
    java.util.function.Supplier<String> s = () -> {
      java.util.function.Supplier<Integer> inner = () -> 1;
      return "alphaVar" + inner.get();
    };
    return s.get().equals("alphaVar1");
  }
  public static void main(String[] a) {
    System.out.println(alphaMethod());
  }
}
