public class PatternGuardTrace {
  static String log="";static int selectors=0;
  static Object select(Object x){selectors++;return x;}
  static boolean guard(String s){log+="G";return s.length()>2;}
  static String f(Object x){return switch(select(x)){case null -> "NULL";case String s when guard(s) -> "LONG";case String s -> "SHORT";case Integer i -> "INT:"+i;default -> "OTHER";};}
  public static void main(String[] a){System.out.println(f(null));System.out.println(f("a"));System.out.println(f("abcd"));System.out.println(f(2));System.out.println(f(2L));System.out.println(log);System.out.println(selectors);}
}
