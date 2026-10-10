package javaclassparser

import (
	"strings"
	"testing"
)

// Constructor loops exercise assertion projection after the physical SUPER
// boundary. The original Parent and driver remain independent, checking capture
// visibility, failure priority, disabled status and Throwable payload identity.
func TestNativeMemberConstructorLoopAssertionKeepsInitializationAndFailureOrderRoundTrip(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		state := "disabled"
		if enabled {
			state = "enabled"
		}
		t.Run(state, func(t *testing.T) {
			f := strings.Replace(nativeMemberAssertionFixture, `Child(long n){super(n);}`, `Child(long n,boolean value,Object token){super(n);for(int i=0;i<3;i++){MemberAssertionEffects.trace+="E";assert condition(value):message(token);}}`, 1)
			f = strings.Replace(f, `Child make(long n){return new Child(n);}`, `Child make(long n,boolean value,Object token){return new Child(n,value,token);}`, 1)
			start := strings.Index(f, "class MemberAssertionDriver{")
			if start < 0 {
				t.Fatal("fixture driver")
			}
			f = f[:start] + `class MemberAssertionDriver{public static void main(String[]args)throws Exception{boolean enabled=ENABLED;ClassLoader.getSystemClassLoader().setClassAssertionStatus("MemberAssertionOwner",enabled);MemberAssertionOwner owner=new MemberAssertionOwner();MemberAssertionEffects.trace="";MemberAssertionOwner.Child child=owner.make(Long.MIN_VALUE,true,"success");if(child.observed!=owner||!MemberAssertionEffects.trace.equals(enabled?"PECECEC":"PEEE"))throw new AssertionError("constructor success capture/effects");Throwable token=new IllegalArgumentException("payload");MemberAssertionEffects.trace="";try{MemberAssertionOwner.Child other=owner.make(Long.MIN_VALUE,false,token);if(enabled||other.observed!=owner)throw new AssertionError("missing failure or capture");}catch(AssertionError e){if(!enabled||e.getCause()!=token)throw new AssertionError("message identity",e);}if(!MemberAssertionEffects.trace.equals(enabled?"PECM":"PEEE"))throw new AssertionError("constructor failure effect order");MemberAssertionEffects.trace="";try{owner.make(1,false,token);throw new AssertionError("missing parent failure");}catch(AssertionError e){if(!"original constructor argument".equals(e.getMessage())||e.getCause()!=null)throw new AssertionError("failure priority",e);}if(!MemberAssertionEffects.trace.equals("P"))throw new AssertionError("assertion evaluated before parent");java.lang.reflect.Field flag=child.getClass().getDeclaredField("$assertionsDisabled");if(!flag.isSynthetic()||flag.getModifiers()!=0x1018)throw new AssertionError("assertion metadata");System.out.println("constructor-assertions:"+enabled+":capture:payload:priority");}}`
			status := "false"
			if enabled {
				status = "true"
			}
			f = strings.Replace(f, "ENABLED", status, 1)
			testNativePrivateSetterFixture(t, f, "MemberAssertionOwner", "MemberAssertionDriver", "constructor-assertions:"+status+":capture:payload:priority\n")
		})
	}
}
