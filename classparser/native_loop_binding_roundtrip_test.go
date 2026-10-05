package javaclassparser

import (
	"strings"
	"testing"
)

const nativeLoopBindingFixture = `class LoopBindingOwner{static int count=777;private Object token;LoopBindingOwner(Object token){this.token=token;}static class Reader{Object read(LoopBindingOwner owner){return owner.token;}}static int scan(int stop,int skip){int count=0;OUTER:for(int i=0;i<8;i++){for(int j=0;j<5;j++){if(i==stop&&j==2)break OUTER;if(j==skip)continue OUTER;count+=i*5+j+1;}}return count;}static int choose(int input){int count;switch(input&7){case 0:count=input;break;case 1:count=input+1;break;default:count=~input;break;}return count;}}
class LoopBindingDriver{public static void main(String[]args){int rows=0;for(int stop:new int[]{-1,0,3,7,8,Integer.MAX_VALUE})for(int skip:new int[]{-1,0,1,2,4,5,8}){int width=skip<0?5:Math.min(skip,5);boolean stopped=stop>=0&&stop<8&&(skip<0||skip>=2);int full=stopped?stop:8;int expected=width*5*full*(full-1)/2+full*width*(width+1)/2+(stopped?10*stop+3:0);if(LoopBindingOwner.scan(stop,skip)!=expected)throw new AssertionError("labeled transfer/binding "+stop+":"+skip);rows++;}for(int input:new int[]{Integer.MIN_VALUE,-9,-1,0,1,7,8,9,Integer.MAX_VALUE}){int tag=input&7;int expected=tag==0?input:tag==1?input+1:~input;if(LoopBindingOwner.choose(input)!=expected)throw new AssertionError("switch break");}LoopBindingOwner.Reader reader=new LoopBindingOwner.Reader();for(Object value:new Object[]{null,new Object()}){if(reader.read(new LoopBindingOwner(value))!=value)throw new AssertionError("private getter identity");}if(LoopBindingOwner.count!=777)throw new AssertionError("static field versus local binding");System.out.println(rows+":loop:transfer:binding:ordinal:oracle");}}`

func TestNativeLoopTransferSourceBindingRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeLoopBindingFixture, "LoopBindingOwner", "LoopBindingDriver", "42:loop:transfer:binding:ordinal:oracle\n")
}
func TestNativeLoopTransferSourceBindingRenamedRoundTrip(t *testing.T) {
	f := strings.ReplaceAll(nativeLoopBindingFixture, "LoopBindingOwner", "OtherLoopLexicalScope")
	testNativePrivateSetterFixture(t, f, "OtherLoopLexicalScope", "LoopBindingDriver", "42:loop:transfer:binding:ordinal:oracle\n")
}
