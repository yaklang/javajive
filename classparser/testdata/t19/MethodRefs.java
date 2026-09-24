public class MethodRefs {
  int v;
  MethodRefs(int v) { this.v = v; }
  int inst(int x) { return this.v + x; }
  static int st(int x) { return x + 1; }
  public static void main(String[] a) {
    java.util.function.IntUnaryOperator s = MethodRefs::st;
    java.util.function.IntFunction<int[]> arr = int[]::new;
    MethodRefs o = new MethodRefs(10);
    java.util.function.IntUnaryOperator b = o::inst;
    java.util.function.ToIntFunction<String> len = String::length;
    java.util.function.IntFunction<MethodRefs> ctor = MethodRefs::new;
    System.out.println(s.applyAsInt(3));
    System.out.println(arr.apply(4).length);
    System.out.println(b.applyAsInt(3));
    System.out.println(len.applyAsInt("abcd"));
    System.out.println(ctor.apply(5).v);
  }
}
