public class HierarchyJoin {
  static java.util.List<String> f(boolean c){java.util.List<String> x;if(c)x=new java.util.ArrayList<String>();else x=new java.util.LinkedList<String>();x.add("ok");return x;}
  static Number g(boolean c){Number n;if(c)n=Integer.valueOf(3);else n=Long.valueOf(4);return n;}
  public static void main(String[] a){System.out.println(f(true).get(0));System.out.println(f(false).get(0));System.out.println(g(true).longValue());System.out.println(g(false).longValue());}
}
