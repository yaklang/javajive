public class Main {
 static int run(int n){int a=1,b=2;for(int i=0;i<n;i++){int old=a;a=b;b=old;}return a*10+b;}
 public static void main(String[] args){for(int i=0;i<6;i++)System.out.println(run(i));}
}
