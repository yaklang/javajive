public class Counter {
  static int n;
  public String toString() {
    n++;
    System.out.println("c" + n);
    return "C" + n;
  }
}
