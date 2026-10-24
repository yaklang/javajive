package javaclassparser

import (
	"strings"
	"testing"
)

// Cases SKIP and default have distinct bytecode entries but reach the same
// loop backedge after GOTO removal. The independent caller checks every read
// and progress index, sparse signed labels, default-before-case layout, an
// explicit continue, and a real fall-through which must remain fall-through.
const switchSharedTransferFixture = `class SwitchEntryProbe {
 static int reads;
 static int read(int[] values,int index){reads++;return values[index];}
 static int dense(int[] values){int index=0;for(;;){int action=read(values,index++);if(action==Integer.MIN_VALUE)return -2;switch(action){case 1:return index;case 2:break;case 3:return -1;default:break;}}}
 static int sparse(int[] values){int index=0;for(;;){int action=read(values,index++);if(action==Integer.MIN_VALUE)return -2;switch(action){case -1:return index;case -271:break;case Integer.MAX_VALUE:return -1;default:break;}}}
 static int defaultFirst(int[] values){int index=0;for(;;){int action=read(values,index++);if(action==Integer.MIN_VALUE)return -2;switch(action){default:continue;case 7:return index;case 8:continue;case 9:return -1;}}}
 static int explicitContinue(int[] values){int index=0;for(;;){int action=read(values,index++);if(action==Integer.MIN_VALUE)return -2;switch(action){case 9:return index;case 10:continue;case 11:return -1;default:continue;}}}
 static int fallThrough(int action){int result=0;switch(action){case 2:result+=10;case 3:result+=1;break;default:result=7;}return result;}
}
class SwitchEntryDriver {public static void main(String[]args){int rows=0;int[] accept={1,-1,7,9},skip={2,-271,8,10},stop={3,Integer.MAX_VALUE,9,11};
 for(int which=0;which<4;which++)for(int prefix=0;prefix<=12;prefix++)for(int ending=0;ending<3;ending++){
  int[] input=new int[prefix+1];for(int i=0;i<prefix;i++)input[i]=i%3==0?skip[which]:(i%3==1?0:99);input[prefix]=ending==0?accept[which]:(ending==1?stop[which]:Integer.MIN_VALUE);
  SwitchEntryProbe.reads=0;int got=which==0?SwitchEntryProbe.dense(input):(which==1?SwitchEntryProbe.sparse(input):(which==2?SwitchEntryProbe.defaultFirst(input):SwitchEntryProbe.explicitContinue(input)));
  int expected=ending==0?prefix+1:(ending==1?-1:-2);if(got!=expected||SwitchEntryProbe.reads!=prefix+1)throw new AssertionError(which+":"+prefix+":"+ending+":"+got+":"+SwitchEntryProbe.reads);rows++;
 }
 for(int action:new int[]{2,3,0}){int expected=action==2?11:(action==3?1:7);if(SwitchEntryProbe.fallThrough(action)!=expected)throw new AssertionError("real fall-through changed");rows++;}
 System.out.println(rows+":switch:entry:transfer:effects:fallthrough");}}
`

func TestAdversarialSwitchSharedLoopTransferPreservesEntryEffects(t *testing.T) {
	for _, owner := range []string{"SwitchEntryProbe", "RenamedSwitchEntry"} {
		t.Run(owner, func(t *testing.T) {
			testNativePrivateSetterFixture(t, strings.ReplaceAll(switchSharedTransferFixture, "SwitchEntryProbe", owner), owner, "SwitchEntryDriver", "159:switch:entry:transfer:effects:fallthrough\n")
		})
	}
}
