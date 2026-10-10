public class Main {
 static Object left(){System.out.print("L");return new Value();}
 static int right(){System.out.print("R");return 4;}
 public static void main(String[] args){System.out.println("["+left()+":"+right()+"]");}
}
