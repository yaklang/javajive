package javaclassparser

import "testing"

// An interface CHECKCAST may precede argument evaluation or fix a wider
// overload independently of a narrower checked operand. Method/type spelling
// cannot erase that check or its dispatch view. A same-spelled user declaration
// must not borrow a library's interpretation either.
const interfaceCastRecoveryFixture = `import java.time.*;import java.time.temporal.*;
class InterfaceViewState{static String trace="";static boolean fail;static final RuntimeException fault=new IllegalStateException("same");}
class TemporalOffset{}
class SpecificOffset extends TemporalOffset{}
public class InterfaceViews{
 public static String select(Object value){return choose((Temporal)(LocalDate)value);}
 public static String choose(Temporal value){InterfaceViewState.trace+="T";return "temporal";}
 public static String choose(LocalDate value){InterfaceViewState.trace+="D";return "date";}
 public static String isCloseTo(Object value){return offset((TemporalOffset)(SpecificOffset)value);}
 public static String offset(TemporalOffset value){InterfaceViewState.trace+="B";return "base";}
 public static String offset(SpecificOffset value){InterfaceViewState.trace+="S";return "specific";}
 public static long read(Object value){return ((Temporal)value).getLong(field());}
 private static TemporalField field(){InterfaceViewState.trace+="F";if(InterfaceViewState.fail)throw InterfaceViewState.fault;return ChronoField.EPOCH_DAY;}
 public static String data(){return "assertj isCloseTo (Temporal)(payload) (TemporalOffset)(payload)";}
}
class InterfaceViewDriver{
 public static void main(String[]args){int rows=0;
  for(Object value:new Object[]{null,LocalDate.of(2000,2,29),Instant.EPOCH,new Object()}){
   InterfaceViewState.trace="";String got=null;Throwable actual=null;
   try{got=InterfaceViews.select(value);}catch(Throwable e){actual=e;}
   boolean valid=value==null||value instanceof LocalDate;
   if(valid?actual!=null||!"temporal".equals(got):!(actual instanceof ClassCastException))throw new AssertionError("original LocalDate check and selected Temporal overload",actual);
   if(!InterfaceViewState.trace.equals(valid?"T":""))throw new AssertionError("checked interface dispatch");rows++;
  }
  for(Object value:new Object[]{null,new SpecificOffset(),new TemporalOffset(),new Object()}){
   InterfaceViewState.trace="";String got=null;Throwable actual=null;
   try{got=InterfaceViews.isCloseTo(value);}catch(Throwable e){actual=e;}
   boolean valid=value==null||value instanceof SpecificOffset;
   if(valid?actual!=null||!"base".equals(got):!(actual instanceof ClassCastException))throw new AssertionError("original subtype check and selected user declaration",actual);
   if(!InterfaceViewState.trace.equals(valid?"B":""))throw new AssertionError("user declaration dispatch");rows++;
  }
  for(Object value:new Object[]{null,LocalDate.of(1970,1,1),LocalDate.of(1969,12,31),LocalDate.MAX,Instant.EPOCH,new Object()})for(boolean fail:new boolean[]{false,true}){
   boolean wrong=value!=null&&!(value instanceof Temporal);Throwable expected=null;long number=0;
   if(wrong)expected=new ClassCastException();
   else if(fail)expected=InterfaceViewState.fault;
   else if(value==null)expected=new NullPointerException();
   else try{number=((Temporal)value).getLong(ChronoField.EPOCH_DAY);}catch(Throwable e){expected=e;}
   InterfaceViewState.trace="";InterfaceViewState.fail=fail;Throwable actual=null;long got=0;
   try{got=InterfaceViews.read(value);}catch(Throwable e){actual=e;}
   if(expected==null?actual!=null||got!=number:expected==InterfaceViewState.fault?actual!=expected:actual==null||actual.getClass()!=expected.getClass())throw new AssertionError("interface receiver check / argument failure priority",actual);
   if(!InterfaceViewState.trace.equals(wrong?"":"F"))throw new AssertionError("argument evaluated before original receiver check or more than once");rows++;
  }
  if(!InterfaceViews.data().equals("assertj isCloseTo (Temporal)(payload) (TemporalOffset)(payload)"))throw new AssertionError("application data changed");rows++;
  System.out.println(rows+":interface:cast:binding:priority:literal");
 }
}`

func TestAdversarialInterfaceViewsKeepOriginalChecksAndSelectedBinding(t *testing.T) {
	runCheckedTypeViewsRoundTrip(t, interfaceCastRecoveryFixture, []string{"InterfaceViews", "RenamedInterfaceViews"}, "InterfaceViewDriver", "21:interface:cast:binding:priority:literal\n", "interface")
}

func TestAdversarialInterfaceSourceWordsCannotEraseChecks(t *testing.T) {
	t.Setenv("JDEC_ASSERTJ_REMAINING_OFF", "")
	for _, source := range []string{
		`class C{String marker="assertj";Object isCloseTo(Object x){return selected((Temporal)(Narrow)x);}}`,
		`class C{String marker="assertj";Object isCloseTo(Object x){return selected((TemporalOffset)(Specific)x);}}`,
		`class C{String data="assertj isCloseTo (Temporal)(payload) (TemporalOffset)(payload)";}`,
	} {
		if got := fixAssertjRemainingReconstructs(source); got != source {
			t.Fatalf("source spelling cannot erase an original interface check, overload view or application data:\nbefore %s\nafter %s", source, got)
		}
	}
}
