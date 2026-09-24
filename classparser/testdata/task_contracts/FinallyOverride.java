public class FinallyOverride {
  static String log="";
  static int f(boolean override){try{log+="T";return 1;}finally{log+="F";if(override)return 2;}}
  static int g(){try{throw new IllegalStateException("original");}finally{log+="G";}}
  public static void main(String[] a){System.out.println(f(false));System.out.println(f(true));try{g();}catch(IllegalStateException e){System.out.println(e.getMessage());}System.out.println(log);}
}
