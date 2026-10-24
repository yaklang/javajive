package javaclassparser

import (
	"strconv"
	"strings"
	"testing"
)

// A source view can pin an overload or retain a runtime check of an erased
// field. Neither the field spelling nor a runtime array's component type can
// replace the original descriptor and CHECKCAST evidence.
type arrayFieldViewOperation struct {
	name, parameters, arguments, declaration, count string
}

// These names reproduce the retired source rules; the production fix must not
// recognize any of them. Each family has an otherwise applicable narrower rival.
func arrayFieldViewOperations() []arrayFieldViewOperation {
	result := []arrayFieldViewOperation{}
	for _, name := range []string{"assertAre", "assertAreNot", "assertHave", "assertDoNotHave"} {
		result = append(result, arrayFieldViewOperation{name, "Condition var1", "var1", "Condition criterion", "0"})
	}
	for _, name := range []string{"assertAreAtLeast", "assertAreAtMost", "assertAreExactly", "assertHaveAtLeast", "assertHaveAtMost", "assertHaveExactly"} {
		result = append(result, arrayFieldViewOperation{name, "int var1,Condition var2", "var1,var2", "int count,Condition criterion", "count"})
	}
	return append(result, arrayFieldViewOperation{"assertIsSortedAccordingToComparator", "java.util.Comparator var1", "var1", "java.util.Comparator criterion", "0"})
}

func TestAdversarialArrayFieldViewsKeepOriginalSelectedOverload(t *testing.T) {
	var operations, declarations, dispatch strings.Builder
	var words []string
	for i, op := range arrayFieldViewOperations() {
		call := "this.arrays." + op.name + "((AssertionInfo)(this.info),((Object[])(this.actual))," + op.arguments + ")"
		operations.WriteString("public int " + op.name + "(" + op.parameters + "){return " + call + ";}\n")
		index := strconv.Itoa(i)
		declarations.WriteString("public int " + op.name + "(AssertionInfo info,Object[] values," + op.declaration + "){return broad(info,values,criterion," + index + "," + op.count + ");}\n")
		declarations.WriteString("public int " + op.name + "(AssertionInfo info,String[] values," + op.declaration + "){FieldArrayState.trace+=\"N\";return -999;}\n")
		arguments := "condition"
		if op.count == "count" {
			arguments = "count,condition"
		}
		if strings.Contains(op.parameters, "Comparator") {
			arguments = "comparator"
		}
		dispatch.WriteString("case " + index + ":result=target." + op.name + "(" + arguments + ");break;\n")
		words = append(words, call)
	}
	payload := "assertj " + strings.Join(words, ";")
	fixture := `public class ArrayFieldViews{
 public final FieldArrays arrays;public final Object info;public final FIELD_TYPE actual;
 public ArrayFieldViews(FieldArrays arrays,Object info,FIELD_TYPE actual){this.arrays=arrays;this.info=info;this.actual=actual;}
 ` + operations.String() + `
 public static String data(){return ` + strconv.Quote(payload) + `;}
}
class FieldArrayState{static int mode;static Object criterion;static String trace="";static final RuntimeException fault=new IllegalStateException("same");}
class ArrayFieldDriver{
 public static void main(String[]args){int rows=0;
  int[] counts={Integer.MIN_VALUE,-1,0,Integer.MAX_VALUE};
  for(int method=0;method<11;method++)for(int countIndex=0;countIndex<(method>=4&&method<10?counts.length:1);countIndex++)for(int mode=0;mode<3;mode++)for(int infoIndex=0;infoIndex<3;infoIndex++)for(int arrayIndex=0;arrayIndex<ARRAY_COUNT;arrayIndex++)for(boolean present:new boolean[]{false,true}){
   int count=method>=4&&method<10?counts[countIndex]:0;
   Object info=infoIndex==0?new AssertionInfo(3):infoIndex==1?null:new Object();
   Object values;
   switch(arrayIndex){case 0:values=null;break;case 1:values=new String[0];break;case 2:values=new String[]{"seed",null};break;case 3:values=new Object();break;case 4:values=new int[2];break;case 5:values=new Object[]{"seed",null};break;case 6:values=new Integer[]{1};break;default:values=new int[][]{{3}};}
   Object[] array=values instanceof Object[]?(Object[])values:null;
   Object originalFirst=array!=null&&array.length>0?array[0]:null;
   Condition condition=present?new Condition(11):null;java.util.Comparator comparator=present?new FieldComparator():null;
   FieldArrayState.mode=mode;FieldArrayState.trace="";FieldArrayState.criterion=method==10?comparator:condition;
   ArrayFieldViews target=new ArrayFieldViews(mode==2?null:new FieldArrays(),info,FIELD_VALUE);Throwable actual=null;int result=0;
   try{switch(method){` + dispatch.String() + `default:throw new AssertionError("missing original operation");}}
   catch(Throwable failure){actual=failure;}
   boolean badArray=values!=null&&!(values instanceof Object[]),badStore=values instanceof Integer[]||values instanceof int[][];
   boolean success=infoIndex!=2&&!badArray&&mode==0&&!badStore;
   if(infoIndex==2||badArray?!(actual instanceof ClassCastException):mode==2?!(actual instanceof NullPointerException):mode==1?actual!=FieldArrayState.fault:badStore?!(actual instanceof ArrayStoreException):actual!=null)throw new AssertionError("original argument checks / receiver failure / helper exception identity",actual);
   int expected=((array==null?-100:array.length*7)+(infoIndex==0?3:0)+(present?11:0)+method*100)^count;
   if(success&&result!=expected)throw new AssertionError("descriptor-selected Object[] overload");
   String trace=infoIndex!=2&&!badArray&&mode!=2?"B"+method:"";
   if(!FieldArrayState.trace.equals(trace))throw new AssertionError("selected helper or evaluation priority: "+FieldArrayState.trace);
   if(target.actual!=values||target.info!=info)throw new AssertionError("field/caller identity changed");
   if(array!=null&&array.length>0&&(success?!array[0].equals("changed"+method):array[0]!=originalFirst))throw new AssertionError("original array mutation or failure boundary");
   rows++;
  }
  if(!ArrayFieldViews.data().equals(` + strconv.Quote(payload) + `))throw new AssertionError("application data changed");rows++;
  System.out.println(rows+":array-field:binding:priority:identity:literal");
 }
}`
	declarationsMap := map[string]string{
		"AssertionInfo.java":   `public final class AssertionInfo{public final int number;public AssertionInfo(int number){this.number=number;}}`,
		"Condition.java":       `public final class Condition{public final int number;public Condition(int number){this.number=number;}}`,
		"FieldComparator.java": `public final class FieldComparator implements java.util.Comparator<Object>{public int compare(Object a,Object b){throw new AssertionError("comparator must only be passed, never evaluated");}}`,
		"FieldArrays.java": `public class FieldArrays{
 private static int broad(AssertionInfo info,Object[] values,Object criterion,int method,int count){
  FieldArrayState.trace+="B"+method;if(criterion!=FieldArrayState.criterion)throw new AssertionError("criterion identity changed");
  if(FieldArrayState.mode==1)throw FieldArrayState.fault;
  if(values!=null&&values.length>0)values[0]="changed"+method;
  return((values==null?-100:values.length*7)+(info==null?0:info.number)+(criterion==null?0:11)+method*100)^count;
 }
 ` + declarations.String() + `}`,
	}
	for _, variant := range []struct{ name, field, argument, count, rows string }{
		{"narrow-field", "String[]", "(String[])values", "3", "1567"},
		{"erased-field", "Object", "values", "8", "4177"},
	} {
		t.Run(variant.name, func(t *testing.T) {
			source := strings.NewReplacer("FIELD_TYPE", variant.field, "FIELD_VALUE", variant.argument, "ARRAY_COUNT", variant.count).Replace(fixture)
			runCheckedTypeViewsRoundTrip(t, source, []string{"ArrayFieldViews", "RenamedArrayFields"}, "ArrayFieldDriver", variant.rows+":array-field:binding:priority:identity:literal\n", "array-field", declarationsMap)
		})
	}
}

func TestAdversarialArrayFieldSourceWordsCannotChangeArgumentViews(t *testing.T) {
	t.Setenv("JDEC_ASSERTJ_REMAINING_OFF", "")
	for _, op := range arrayFieldViewOperations() {
		call := "this.arrays." + op.name + "((AssertionInfo)(this.info),((Object[])(this.actual))," + op.arguments + ")"
		for _, source := range []string{"class C{String marker=\"assertj\";int read(" + op.parameters + "){return " + call + ";}}", "class C{String data=" + strconv.Quote("assertj "+call) + ";}"} {
			if got := fixAssertjRemainingReconstructs(source); got != source {
				t.Fatalf("source words cannot erase a selected view or change application data:\nbefore %s\nafter %s", source, got)
			}
		}
	}
}
