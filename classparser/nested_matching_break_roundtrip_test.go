package javaclassparser

import "testing"

func TestAdversarialNestedMatchingRuleBreakRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "NestedMatchingBreak", `import java.util.*;
public class NestedMatchingBreak {
 static String trace;
 static void emit(String branch,String replacement,boolean force){trace+="E:"+branch+":"+replacement+":"+force+";";}
 static String run(String text,boolean branching){trace="";List<String> branches=new ArrayList<String>(Arrays.asList("one","two"));char previous=0;
  for(int i=0;i<text.length();i++){
   char current=text.charAt(i);
   List<String> rules=Arrays.asList("ab","a","b","xy","x");
   List<String> next=branching?new ArrayList<String>():Collections.<String>emptyList();
   for(String rule:rules){if(text.startsWith(rule,i)){
    if(branching)next.clear();
    String[] replacements=new String[]{rule,"tail"};
    for(String branch:branches){for(String replacement:replacements){
     boolean force=(previous=='a' && current=='b')||(previous=='b' && current=='a');
     emit(branch,replacement,force);if(branching)next.add(branch);else break;
    }}
    if(branching){branches.clear();branches.addAll(next);}
    trace+="M:"+rule+";";i+=rule.length()-1;break;
   }}previous=current;
  }return trace;
 }
 public static void main(String[] args){for(String text:new String[]{"","ab","aba","xy","xya","baba","zzab","ax"})for(boolean b:new boolean[]{false,true})System.out.println(text+":"+b+":"+run(text,b));}
}`, Precision, Compatibility, "legacy")
}
