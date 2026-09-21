public class OverloadWitness {
  static String pick(Object x) { return "Object"; }
  static String pick(char[] x) { return "chars"; }
  static String pick(String x) { return "String"; }
  static String number(int x) { return "int"; }
  static String number(Integer x) { return "Integer"; }
  public static void main(String[] a) {
    Object o=null; String s=null; char[] c=null; Integer boxed=3;
    System.out.println(pick(o)); System.out.println(pick(s)); System.out.println(pick(c));
    System.out.println(number(boxed)); System.out.println(number(boxed.intValue()));
    System.out.println(String.valueOf(o));
  }
}
