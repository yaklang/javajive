package javaclassparser

import "testing"

func TestAdversarialSwitchTryKeepsLoopOwnerRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "SwitchTryLoop", `public class SwitchTryLoop {
  final String[] names,values;
  int index;
  SwitchTryLoop(String[] names,String[] values) { this.names=names;this.values=values; }
  String next() { return index==names.length ? null : names[index++]; }
  int scan() {
    int result=0;
    String name;
    while((name=next())!=null) {
      String value=values[index-1];
      switch(name) {
        case "line":
          if(value.length()==1 && value.charAt(0)>='0' && value.charAt(0)<='9') {
            result=value.charAt(0)-'0';
          } else {
            try { result=Integer.parseInt(value); break; }
            catch(NumberFormatException e) { throw new IllegalArgumentException("invalid:"+value,e); }
          }
          break;
        case "add": result+=10; break;
        default: result--;
      }
    }
    return result;
  }
  public static void main(String[] args) {
    for(String value:new String[]{"3","123","-5","bad"}) {
      SwitchTryLoop s=new SwitchTryLoop(new String[]{"line","add","other"},new String[]{value,"",""});
      try { System.out.print(s.scan()+":"+s.index+";"); }
      catch(IllegalArgumentException e) { System.out.print(e.getMessage()+":"+s.index+";"); }
    }
  }
}`, Precision, Compatibility, "legacy")
}
