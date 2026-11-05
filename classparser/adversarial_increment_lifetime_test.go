package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialIncrementRetainsPrecedingLoopBinding(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class PrefixLedger {
 public static int visits;
 public static void visit(int limit){if(++visits>limit)throw new IllegalStateException("loop failed to advance");}
 public static int select(Object[][] entries,boolean names,boolean indexes,boolean pairs){
  if(names||indexes){boolean same=true;Object first=entries[0][0];
   for(int i=1;i<entries.length;i++){visit(entries.length);Object[] row=entries[i];if(!first.equals(row[0])){same=false;break;}}
   if(same){Object[] head=entries[0];
    if(names){String[] words=new String[entries.length];for(int i=0;i<entries.length;i++){Object[] row=entries[i];String word=(String)row[1];words[i]=word;}
     String prefix=(String)head[0];if(first instanceof String){String part=(String)first;return prefix.length()+part.length()+words.length;}}
    else{int[] numbers=new int[entries.length];for(int i=0;i<entries.length;i++){Object[] row=entries[i];Integer number=(Integer)row[1];numbers[i]=number.intValue();}
     String prefix=(String)head[0];if(first instanceof String){String part=(String)first;return prefix.length()+part.length()-numbers.length;}}
   }
  }else if(pairs){boolean same=true;Object first=entries[0][0];Object second=entries[0][1];
   for(int i=1;i<entries.length;i++){Object[] row=entries[i];if(!first.equals(row[0])||!second.equals(row[1])){same=false;break;}}
   if(same)return 100+entries.length;
  }
  if(entries.length>1){boolean same=true;boolean compatible=true;int lastIndex=entries[0].length-1;Object last=entries[0][lastIndex];
   for(int column=0;column<lastIndex;column++){Object value=entries[0][column];for(int row=1;row<entries.length;row++){if(!value.equals(entries[row][column])){same=false;break;}}if(!same)break;}
   if(same){for(int row=1;row<entries.length;row++){if(last.getClass()!=entries[row][lastIndex].getClass()){compatible=false;break;}}if(compatible)return 1000+lastIndex;}
  }
  return -71;
 }
}
class PrefixLedgerDriver {public static void main(String[]args){int rows=0;for(int n=1;n<=9;n++)for(int mismatch=-1;mismatch<n;mismatch++)for(boolean names:new boolean[]{false,true})for(boolean indexes:new boolean[]{false,true})for(boolean pairs:new boolean[]{false,true}){
 Object[][] input=new Object[n][2];for(int i=0;i<n;i++){input[i][0]=i==mismatch?"different":"root";input[i][1]=names?"value":Integer.valueOf(i);}
 boolean equal=true;for(int j=1;j<n;j++)if(!input[0][0].equals(input[j][0]))equal=false;
 int expected=-71;
 if((names||indexes)&&equal)expected=2*((String)input[0][0]).length()+(names?n:-n);
 else if(!(names||indexes)&&pairs&&equal&&n==1)expected=100+n;
 else if(n>1&&equal)expected=1001;
 int expectedVisits=0;if(names||indexes){expectedVisits=n-1;if(n>1&&mismatch==0)expectedVisits=1;else if(mismatch>0)expectedVisits=mismatch;}
 PrefixLedger.visits=0;int actual=PrefixLedger.select(input,names,indexes,pairs);if(actual!=expected||PrefixLedger.visits!=expectedVisits)throw new AssertionError("expected="+expected+" actual="+actual+" visits="+PrefixLedger.visits+" expectedVisits="+expectedVisits);rows++;
 }System.out.println(rows+":increment:disjoint-lifetimes");}}
`
	for _, name := range []string{"PrefixLedger", "ColumnAgreement"} {
		t.Run(name, func(t *testing.T) {
			testAuthoredOwnerAcrossSourceModes(t, strings.ReplaceAll(source, "PrefixLedger", name), name, name+"Driver", "432:increment:disjoint-lifetimes\n")
		})
	}
}
