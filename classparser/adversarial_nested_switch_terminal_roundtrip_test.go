package javaclassparser

import (
	"strings"
	"testing"
)

const nestedSwitchTerminalFixture = `class NestedSwitchTerminalOwner {
 static int choose(boolean direct,int x){switch(x){case 0:if(direct){return 17;}else{switch(x+1){case 1:while(true){if(x==0)break;x--;}return 23;default:return 29;}}default:return 31;}}
 static int nested(int x){switch(x){case 0:switch(x+1){case 1:switch(x+2){case 2:break;default:break;}return 37;default:return 41;}default:return 43;}}
}
class NestedSwitchTerminalDriver{public static void main(String[]args){int rows=0;for(int x=-2;x<3;x++)for(boolean direct:new boolean[]{false,true}){if(NestedSwitchTerminalOwner.choose(direct,x)!=(x==0?(direct?17:23):31))throw new AssertionError("conditional nested loop exit/return");if(NestedSwitchTerminalOwner.nested(x)!=(x==0?37:43))throw new AssertionError("nested switch exit/return");rows++;}System.out.println(rows+":nested:inner-break:outer-terminal");}}
`

func TestAdversarialNestedSwitchInnerBreakDoesNotCompleteOuterRoundTrip(t *testing.T) {
	for _, owner := range []string{"NestedSwitchTerminalOwner", "RenamedTerminalOwner"} {
		t.Run(owner, func(t *testing.T) {
			testNativePrivateSetterFixture(t, strings.ReplaceAll(nestedSwitchTerminalFixture, "NestedSwitchTerminalOwner", owner), owner, "NestedSwitchTerminalDriver", "10:nested:inner-break:outer-terminal\n")
		})
	}
}
