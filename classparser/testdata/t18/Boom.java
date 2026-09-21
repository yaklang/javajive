public class Boom {
  public String toString() {
    System.out.println("boom");
    throw new RuntimeException("boom");
  }
}
