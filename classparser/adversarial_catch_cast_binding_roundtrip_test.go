package javaclassparser

import "testing"

func TestAdversarialTypedCatchCauseBindingRoundTrip(t *testing.T) {
	roundTripGenericFlowUnits(t, "TypedCatchCauseBindingReview", `
import java.util.*;
class TypedCauseChecked extends Exception {}
class TypedCauseWrapper extends RuntimeException {TypedCauseWrapper(String message,Exception cause){super(message,cause);}}
class TypedCauseOps {static String trace="";static final TypedCauseChecked checked=new TypedCauseChecked();static final RuntimeException runtime=new IllegalArgumentException();static final AssertionError error=new AssertionError();static List<String> parse(int mode)throws TypedCauseChecked {trace+="parse;";if(mode==1)throw checked;if(mode==2)throw runtime;if(mode==3)throw error;return Arrays.asList("good");}static void report(Throwable caught){System.out.println(caught.getClass().getName()+":"+(caught==checked)+":"+(caught==runtime)+":"+(caught==error)+":"+(caught.getCause()==checked)+":"+trace);}}
public class TypedCatchCauseBindingReview {
 static List<String> run(int mode,boolean first){List<String> result=new ArrayList<String>();if(first){try{List<String> parsed=TypedCauseOps.parse(mode);result.addAll(parsed);}catch(TypedCauseChecked caught){throw new TypedCauseWrapper("first",caught);}}try{List<String> parsed=TypedCauseOps.parse(mode);if(parsed!=null)result.addAll(parsed);}catch(TypedCauseChecked caught){throw new TypedCauseWrapper("second",caught);}return result;}
 public static void main(String[]args){for(int mode=0;mode<4;mode++){TypedCauseOps.trace="";try{System.out.println(run(mode,false));}catch(Throwable caught){TypedCauseOps.report(caught);}TypedCauseOps.trace="";try{System.out.println(run(mode,true));}catch(Throwable caught){TypedCauseOps.report(caught);}}}
}`, nil, nil, Precision, Compatibility, "legacy")
}
