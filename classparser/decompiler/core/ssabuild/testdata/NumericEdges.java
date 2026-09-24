public class NumericEdges {
  static int cmp(double x,double y){return x<y?-1:(x==y?0:1);}
  static int shift(int x,int s){return x>>>s;}
  public static void main(String[] a){double[] v={Double.NaN,-0.0,0.0,Double.POSITIVE_INFINITY};for(double x:v){System.out.println(cmp(x,0.0));System.out.println(Double.doubleToRawLongBits(x));}System.out.println(shift(-1,33));int x=Integer.MAX_VALUE;System.out.println(x+1);System.out.println((byte)255);}
}
