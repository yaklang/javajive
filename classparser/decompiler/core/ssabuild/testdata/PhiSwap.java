public class PhiSwap {
  static int f(int n) { int a=1,b=2; for(int i=0;i<n;i++){int t=a;a=b;b=t;} return a*10+b; }
  static long wide(int n){long a=0x123456789L,b=-3; for(int i=0;i<n;i++){long t=a;a=b;b=t;} return a;}
  public static void main(String[] a){for(int n=0;n<6;n++){System.out.println(f(n));System.out.println(wide(n));}}
}
