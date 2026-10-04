public class LambdaCapture {
  static int plus(int x){return x+5;}
  public static void main(String[] args){
    int a=2,b=7;java.util.function.IntUnaryOperator f=x->x*a+b;
    java.util.function.IntUnaryOperator g=LambdaCapture::plus;
    java.util.function.IntFunction<int[]> h=int[]::new;
    System.out.println(f.applyAsInt(3));System.out.println(g.applyAsInt(3));System.out.println(h.apply(4).length);
  }
}
