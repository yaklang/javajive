package accept.gate;

import accept.Arithmetic;

/** Reviewed behavioral oracle for the accepted arithmetic fixture. */
public final class ArithmeticDriver {
  public static void main(String[] args) {
    Arithmetic.main(new String[0]);
    System.out.println("arithmetic:" + Arithmetic.choose(-3) + ":" + Arithmetic.choose(-2)
        + ":" + Arithmetic.choose(0) + ":" + Arithmetic.choose(4)
        + ":" + new Arithmetic(9).value() + ":" + Arithmetic.run());
  }
}
