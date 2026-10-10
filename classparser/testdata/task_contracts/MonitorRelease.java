public class MonitorRelease {
  static final Object LOCK=new Object();
  static boolean held(){return Thread.holdsLock(LOCK);}
  static void f(){synchronized(LOCK){System.out.println(held());throw new IllegalStateException("lock");}}
  public static void main(String[] a){System.out.println(held());try{f();}catch(IllegalStateException e){System.out.println(e.getMessage());}System.out.println(held());synchronized(LOCK){System.out.println(held());}System.out.println(held());}
}
