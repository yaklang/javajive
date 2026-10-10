public class NestedBeta {
  public static boolean betaMethod() {
    java.util.function.Supplier<String> s = () -> {
      java.util.function.Supplier<Integer> inner = () -> 2;
      return "betaVar" + inner.get();
    };
    return s.get().equals("betaVar2");
  }
  public static void main(String[] a) {
    System.out.println(betaMethod());
  }
}
