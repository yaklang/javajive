package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialSwitchBreakReachesSharedLoopTail(t *testing.T) {
	const fixture = `
public class SwitchTail{
 static final RuntimeException FAILURE=new IllegalStateException("identity");StringBuilder trace=new StringBuilder();int calls,fail;
 void event(String text){trace.append(text);if(++calls==fail)throw FAILURE;}
 boolean offer(int[] codes,int mask){int position=0;while(true){int index=position++;event("S;");int code=codes[index];if(code>=0){switch(code){case 0:break;case 1:continue;case 2:return false;case 3:event("E;");return true;}}event("C;");if((mask&(1<<index))!=0)return true;}}
}
class TailDriver{public static void main(String[]args){int rows=0;
 for(int first:new int[]{-1,0,1,4})for(int second:new int[]{-1,0,1,4})for(int terminal:new int[]{2,3})for(int mask=0;mask<8;mask++)for(int fail=0;fail<8;fail++){
 int[] codes={first,second,terminal};StringBuilder whole=new StringBuilder();boolean expectedValue=false;
 for(int i=0;i<codes.length;i++){whole.append("S;");int code=codes[i];if(code==1)continue;if(code==2)break;if(code==3){whole.append("E;");expectedValue=true;break;}whole.append("C;");if((mask&(1<<i))!=0){expectedValue=true;break;}}
 int events=whole.length()/2;boolean thrown=fail>0&&fail<=events;String expected=thrown?whole.substring(0,2*fail):whole.toString();
 SwitchTail target=new SwitchTail();target.fail=fail;boolean actual=false;Throwable error=null;try{actual=target.offer(codes,mask);}catch(Throwable ex){error=ex;}
 if(thrown?error!=SwitchTail.FAILURE:error!=null||actual!=expectedValue)throw new AssertionError("route "+first+","+second+","+terminal+" mask="+mask+" fail="+fail,error);
 if(!expected.equals(target.trace.toString()))throw new AssertionError("trace "+target.trace+" != "+expected);rows++;
 }System.out.println(rows+":switch:break:shared-tail:effects:failure");}}
`
	for _, spelling := range []string{"original", "renamed"} {
		t.Run(spelling, func(t *testing.T) {
			text, owner := fixture, "SwitchTail"
			if spelling == "renamed" {
				text = strings.NewReplacer("SwitchTail", "QueuePath", "offer(", "publish(", "event(", "record(", "position", "cursor").Replace(text)
				owner = "QueuePath"
			}
			testAuthoredOwnerAcrossSourceModes(t, text, owner, "TailDriver", "2048:switch:break:shared-tail:effects:failure\n")
		})
	}
}
