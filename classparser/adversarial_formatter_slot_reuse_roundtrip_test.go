package javaclassparser

import "testing"

// The formatter and text are reaching definitions of different JVM reference
// slot lifetimes. Rebuilding them as one hoisted local changes overload binding
// or feeds the formatter value into the parse path. Compare actual JVM output
// through both parse APIs, numeric input, null, and parser failure.
func TestAdversarialFormatterStringSlotReuseRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "FormatterSlotReuse", `
import java.time.*;
import java.time.format.*;
public class FormatterSlotReuse {
 static String trace="";
 static LocalTime read(String text,boolean formatted,boolean numeric){
  if(numeric){long value=Long.parseLong(text);trace+="N";return LocalTime.ofSecondOfDay(value);}
  if(formatted){DateTimeFormatter formatter=DateTimeFormatter.ofPattern("yyyy/MM/dd HH:mm:ss");trace+="F";return LocalDateTime.parse(text,formatter).toLocalTime();}
  String value=text;trace+="S";return LocalTime.parse(value);
 }
 public static void main(String[] args){for(String text:new String[]{"2020/02/29 12:34:56","12:34:56","45296","bad",null})for(boolean formatted:new boolean[]{false,true})for(boolean numeric:new boolean[]{false,true}){trace="";try{System.out.println(read(text,formatted,numeric)+":"+trace);}catch(Throwable e){System.out.println(e.getClass().getName()+":"+trace);}}}
}`, Precision, Compatibility, "legacy")
}
