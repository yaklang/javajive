public class Main {
 static int step(int i){if(i==1)throw new IllegalArgumentException("hit");return i;}
 static int run(boolean later){int x=7;try{step(later?0:1);x=11;step(1);return 0;}catch(IllegalArgumentException e){return x;}}
 public static void main(String[] args){System.out.println(run(false)+","+run(true));}
}
