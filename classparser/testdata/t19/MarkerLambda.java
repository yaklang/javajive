public class MarkerLambda {
  public static void main(String[] a) {
    Runnable r = (Runnable & Marker) () -> System.out.print("m");
    r.run();
    System.out.println("ok");
  }
}
