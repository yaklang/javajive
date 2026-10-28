package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

func TestAdversarialOffsetCopiesRetainZeroTripDefinition(t *testing.T) {
	t.Setenv("JDEC_INIT_PROX_SPLIT_OFF", "1")
	const source = `
public class OffsetLedger {
 public static String scan(int[] kinds,int[] widths,int seed){int[] copy=new int[kinds.length];for(int i=0;i<copy.length;i++)copy[i]=kinds[i];int first=INITIAL;int second=INITIAL;int third=INITIAL;int fourth=INITIAL;int position=seed;
 for(int count=kinds.length;count>0;count--){int kind=copy[kinds.length-count];int width=widths[kinds.length-count];position+=6;
 int selected=position;int retained=selected;if(kind==0)first=retained;else if(kind==1)second=retained;else if(kind==2)third=retained;else if(kind==3)fourth=retained;position+=width;}
 StringBuilder out=new StringBuilder();if(first!=0)out.append('A').append(first).append(':').append(first+2);if(second!=0)out.append('B').append(second).append(':').append(second+2);if(third!=0)out.append('C').append(third).append(':').append(third+2);if(fourth!=0)out.append('D').append(fourth).append(':').append(fourth+2);return out.toString();}
}
class OffsetLedgerDriver {public static void main(String[]args){int rows=0;for(int size=0;size<18;size++)for(int pattern=0;pattern<7;pattern++)for(int seed:new int[]{0,1,-1,17,Integer.MIN_VALUE,Integer.MAX_VALUE}){
int[] kinds=new int[size],widths=new int[size],offsets=new int[4];java.util.Arrays.fill(offsets,INITIAL);int position=seed;for(int i=0;i<size;i++){kinds[i]=(i*3+pattern)%7;widths[i]=(i%3==0?Integer.MAX_VALUE: i%3==1?-i*13-pattern:i*13+pattern);position+=6;if(kinds[i]<4)offsets[kinds[i]]=position;position+=widths[i];}StringBuilder expected=new StringBuilder();for(int j=0;j<4;j++)if(offsets[j]!=0)expected.append((char)('A'+j)).append(offsets[j]).append(':').append(offsets[j]+2);String got=OffsetLedger.scan(kinds,widths,seed);if(!got.equals(expected.toString()))throw new AssertionError("size="+size+" seed="+seed+" actual="+got+" expected="+expected);rows++;}System.out.println(rows+":offsets:copies:zero-trip");}}
`
	for _, name := range []string{"OffsetLedger", "PositionCatalogue"} {
		for _, initial := range []int{0, 1, 17, -1} {
			t.Run(fmt.Sprintf("%s/initial%d", name, initial), func(t *testing.T) {
				text := strings.ReplaceAll(strings.ReplaceAll(source, "OffsetLedger", name), "INITIAL", fmt.Sprint(initial))
				testAuthoredOwnerAcrossSourceModes(t, text, name, name+"Driver", "756:offsets:copies:zero-trip\n")
			})
		}
	}
}
