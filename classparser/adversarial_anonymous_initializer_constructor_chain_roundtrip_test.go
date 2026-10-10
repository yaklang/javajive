package javaclassparser

import (
	"strings"
	"testing"
)

const initializerConstructorChainFixture = `abstract class ChainInitParent{ChainInitParent(){ChainInitEffects.trace+="P";}abstract int read();}
class ChainInitEffects{static String trace="";static boolean fail;static final RuntimeException marker=new IllegalStateException("original");static int argument(){trace+="A";if(fail)throw marker;return 7;}static int touch(int n){trace+=n+":";return n;}}
public class ChainInitOwner{
 final int before=ChainInitEffects.touch(11);
 final ChainInitParent value=new ChainInitParent(){int read(){return before;}};
 final int after=ChainInitEffects.touch(13);final int input;
 public ChainInitOwner(){this(ChainInitEffects.argument());ChainInitEffects.trace+="Z";}
 public ChainInitOwner(long n){this((int)n);ChainInitEffects.trace+="L";}
 public ChainInitOwner(int n){input=n;ChainInitEffects.trace+="I";}
}
class ChainInitDriver{public static void main(String[]args)throws Exception{int rows=0;
 for(long n:new long[]{Long.MIN_VALUE,-1,0,Long.MAX_VALUE}){ChainInitEffects.trace="";ChainInitOwner owner=new ChainInitOwner(n);if(owner.input!=(int)n||owner.value.read()!=11||owner.after!=13||!ChainInitEffects.trace.equals("11:P13:IL"))throw new AssertionError("wide delegation / initializer count / order:"+ChainInitEffects.trace);if(owner.value.getClass().getEnclosingConstructor()!=null||owner.value.getClass().getEnclosingMethod()!=null||!owner.value.getClass().isAnonymousClass())throw new AssertionError("original initializer ownership");rows++;}
 ChainInitEffects.trace="";ChainInitOwner owner=new ChainInitOwner();if(owner.input!=7||!ChainInitEffects.trace.equals("A11:P13:IZ"))throw new AssertionError("argument / initializer / tail order:"+ChainInitEffects.trace);rows++;
 ChainInitEffects.trace="";ChainInitEffects.fail=true;try{new ChainInitOwner();throw new AssertionError("missing pre-delegation failure");}catch(RuntimeException actual){if(actual!=ChainInitEffects.marker||!ChainInitEffects.trace.equals("A"))throw new AssertionError("failure identity / initializer ran early");rows++;}
 System.out.println(rows+":initializer:delegation:once:order:scope:failure");}}
`

func TestAdversarialAnonymousInitializerConstructorChainRunsOnceInOriginalScope(t *testing.T) {
	fixture := initializerConstructorChainFixture
	testNativePrivateSetterSourceFixture(t, map[string]string{"ChainInitOwner.java": fixture}, "ChainInitOwner", "ChainInitDriver", "6:initializer:delegation:once:order:scope:failure\n")
}

func TestAdversarialAnonymousInitializerConstructorChainWithReceiverFreeSDKArguments(t *testing.T) {
	fixture := strings.Replace(initializerConstructorChainFixture, "this(ChainInitEffects.argument());", "this(ChainInitEffects.argument()+Runtime.getRuntime().availableProcessors());", 1)
	fixture = strings.Replace(fixture, "owner.input!=7", "owner.input!=7+Runtime.getRuntime().availableProcessors()", 1)
	testNativePrivateSetterSourceFixture(t, map[string]string{"ChainInitOwner.java": fixture}, "ChainInitOwner", "ChainInitDriver", "6:initializer:delegation:once:order:scope:failure\n")
}
