public class LiteralEscapes {
  public static void main(String[] args) {
    String s = "quote=\" slash=\\ slashu=\\u0041\n\r\t\b\f" + "\uD800\uDC00\uDC00";
    for (int i=0;i<s.length();i++) System.out.println((int)s.charAt(i));
  }
}
