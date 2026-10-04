public class SerializableLambda {
  public static void main(String[] a) {
    java.util.function.IntUnaryOperator ser =
        (java.util.function.IntUnaryOperator & java.io.Serializable) (x -> x + 1);
    System.out.println(ser.applyAsInt(2));
  }
}
