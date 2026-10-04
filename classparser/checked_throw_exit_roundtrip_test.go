package javaclassparser

import "testing"

func TestAdversarialCheckedThrowExitRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "CheckedThrowExit", `import java.io.*;
public class CheckedThrowExit {
  static void visit(int kind,boolean wrap) throws IOException,ThrowExitChecked {
    try {ThrowExitOracle.body(kind);}
    catch(ThrowExitFlow e){throw e;}
    catch(ThrowExitChecked e){throw e;}
    catch(IOException e){throw e;}
    catch(Exception e){
      if(wrap)throw new IllegalArgumentException("wrapped",e);
      if(e instanceof RuntimeException)throw (RuntimeException)e;
      throw new IllegalStateException("other",e);
    } finally {ThrowExitOracle.mark("cleanup");}
  }
  static void direct(Throwable value,int kind) throws IOException {
    try {
      ThrowExitOracle.mark("cast");
      if(kind==0)throw (RuntimeException)value;
      if(kind==1)throw (IOException)value;
      throw (Error)value;
    } finally {ThrowExitOracle.mark("direct-cleanup");}
  }
  public static void main(String[] args){ThrowExitOracle.run();}
}
class ThrowExitChecked extends Exception {ThrowExitChecked(String s){super(s);}}
class ThrowExitFlow extends RuntimeException {ThrowExitFlow(String s){super(s);}}
class ThrowExitOracle {
  static StringBuilder trace;static int step,failAt;
  static void mark(String label){trace.append(label).append(';');if(++step==failAt)throw new IllegalStateException(label);}
  static void body(int kind) throws IOException,ThrowExitChecked {
    mark("body");
    if(kind==1)throw new IOException("io");
    if(kind==2)throw new ThrowExitChecked("checked");
    if(kind==3)throw new ThrowExitFlow("flow");
    if(kind==4)throw new IllegalArgumentException("argument");
    if(kind==5)throw new AssertionError("error");
  }
  static String result(Throwable e){return e.getClass().getSimpleName()+":"+(e instanceof NullPointerException ? "null" : e.getMessage())+":"+(e.getCause()==null ? "none" : e.getCause().getClass().getSimpleName());}
  static void run(){
    for(int kind=0;kind<6;kind++)for(int wrap=0;wrap<2;wrap++)for(failAt=0;failAt<5;failAt++) {
      trace=new StringBuilder();step=0;String result="ok";
      try{CheckedThrowExit.visit(kind,wrap!=0);}catch(Throwable e){result=result(e);}
      System.out.println("visit:"+kind+":"+wrap+":"+failAt+":"+result+":"+trace);
    }
    Throwable[] values={null,new IllegalStateException("runtime"),new IOException("io"),new AssertionError("error"),new ThrowExitChecked("checked")};
    for(int i=0;i<values.length;i++)for(int kind=0;kind<3;kind++)for(failAt=0;failAt<5;failAt++) {
      trace=new StringBuilder();step=0;String result="ok";
      try{CheckedThrowExit.direct(values[i],kind);}catch(Throwable e){result=result(e);}
      System.out.println("direct:"+i+":"+kind+":"+failAt+":"+result+":"+trace);
    }
  }
}`, Precision, Compatibility, DecompileMode("legacy"))
}
