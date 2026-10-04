public class ConcatOrder {
  static String log="";static int n(String s,int x){log+=s;return x;}
  public static void main(String[] a){Object o=null;String x="\u0001\u0002:"+n("A",1)+":"+n("B",2)+":"+o;
    for(int i=0;i<x.length();i++)System.out.println((int)x.charAt(i));System.out.println(log);
  }
}
