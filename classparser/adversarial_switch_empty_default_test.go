package javaclassparser

import (
	"strings"
	"testing"
)

func TestAdversarialExplicitSwitchDefaultThrowRemainsAbrupt(t *testing.T) {
	const fixture = `
public class ThrowRoute{
 static final RuntimeException FAILURE=new IllegalStateException("identity");StringBuilder trace=new StringBuilder();int calls,fail;
 void event(String text){trace.append(text);if(++calls==fail)throw FAILURE;}
 int dispatch(int code){switch(code){case 0:event("A;");break;case 1:event("B;");break;default:throw new RuntimeException();}event("C;");return trace.length();}
}
class ThrowDriver{public static void main(String[]args){int rows=0;
 for(int code=-2;code<4;code++)for(int fail=0;fail<4;fail++){
 ThrowRoute target=new ThrowRoute();target.fail=fail;Throwable actual=null;int value=-1;try{value=target.dispatch(code);}catch(Throwable error){actual=error;}
 if(code==0||code==1){String first=code==0?"A;":"B;";String expected=fail==1?first:first+"C;";
 if(!expected.equals(target.trace.toString())||(fail==1||fail==2?actual!=ThrowRoute.FAILURE:actual!=null||value!=4))throw new AssertionError("selected arm "+code+" fail="+fail,actual);
 }else if(actual==null||actual.getClass()!=RuntimeException.class||actual.getMessage()!=null||target.trace.length()!=0)throw new AssertionError("lost explicit throw "+code,actual);rows++;
 }System.out.println(rows+":switch:explicit-throw:effects:failure");}}
`
	for _, spelling := range []string{"original", "renamed"} {
		t.Run(spelling, func(t *testing.T) {
			text, owner := fixture, "ThrowRoute"
			if spelling == "renamed" {
				text = strings.NewReplacer("ThrowRoute", "AbruptChoice", "dispatch(", "select(", "event(", "record(").Replace(text)
				owner = "AbruptChoice"
			}
			testAuthoredOwnerAcrossSourceModes(t, text, owner, "ThrowDriver", "24:switch:explicit-throw:effects:failure\n")
		})
	}
}

func TestAdversarialEmptySwitchDefaultsRetainNormalCompletion(t *testing.T) {
	const fixture = `
public class DefaultRoute{
 static final RuntimeException FAILURE=new IllegalStateException("identity");StringBuilder trace=new StringBuilder();int calls,fail;
 void event(String text){trace.append(text);if(++calls==fail)throw FAILURE;}
 void consume(int code){switch(code){case 1:event("A;");break;case 3:event("B;");}}
 boolean retry(int[] codes){int i=0;while(true){event("S;");switch(codes[i++]){case 1:return false;case 3:event("E;");return true;}}}
 boolean retryNested(int[] codes,boolean boxed){int i=0;while(true){event("S;");int code=codes[i++];if(boxed){switch(code){case 1:return false;case 3:event("E;");return true;}}else{if(code==1)return false;if(code==3){event("E;");return true;}}}}
}
class DefaultDriver{public static void main(String[]args){int rows=0;
 for(int code:new int[]{Integer.MIN_VALUE,-1,0,1,2,3,4,Integer.MAX_VALUE})for(int fail=0;fail<3;fail++){
 DefaultRoute target=new DefaultRoute();target.fail=fail;Throwable actual=null;try{target.consume(code);}catch(Throwable error){actual=error;}String expected=code==1?"A;":code==3?"B;":"";
 if(!expected.equals(target.trace.toString())||(fail==1&&!expected.isEmpty()?actual!=DefaultRoute.FAILURE:actual!=null))throw new AssertionError("normal default "+code+" fail="+fail,actual);rows++;
 }
 for(int variant=0;variant<3;variant++)for(int count=0;count<4;count++)for(int terminal:new int[]{1,3})for(int fail=0;fail<7;fail++){
 int[] codes=new int[count+1];for(int i=0;i<count;i++)codes[i]=(i&1)==0?-1:2;codes[count]=terminal;DefaultRoute target=new DefaultRoute();target.fail=fail;
 StringBuilder expected=new StringBuilder();int events=count+1+(terminal==3?1:0);boolean thrown=fail>0&&fail<=events;int reached=thrown?fail:events;for(int i=0;i<reached;i++)expected.append(i<=count?"S;":"E;");
 boolean result=false;Throwable actual=null;try{result=variant==0?target.retry(codes):target.retryNested(codes,variant==2);}catch(Throwable error){actual=error;}
 if(thrown?actual!=DefaultRoute.FAILURE:actual!=null||result!=(terminal==3))throw new AssertionError("retry "+count+" terminal="+terminal+" fail="+fail,actual);
 if(!expected.toString().equals(target.trace.toString()))throw new AssertionError("retry effects "+target.trace+" != "+expected);rows++;
 }
 System.out.println(rows+":switch:normal:retry:effects:failure");}}
`
	for _, spelling := range []string{"original", "renamed"} {
		t.Run(spelling, func(t *testing.T) {
			text, owner := fixture, "DefaultRoute"
			if spelling == "renamed" {
				text = strings.NewReplacer("DefaultRoute", "SilentChoice", "consume(", "accept(", "retry(", "offer(", "event(", "record(").Replace(text)
				owner = "SilentChoice"
			}
			testAuthoredOwnerAcrossSourceModes(t, text, owner, "DefaultDriver", "192:switch:normal:retry:effects:failure\n")
		})
	}
}
