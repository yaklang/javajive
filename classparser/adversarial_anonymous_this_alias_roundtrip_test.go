package javaclassparser

import (
	"strings"
	"testing"
)

const anonymousThisAliasFixture = `public class EnumForestOwner{
 public enum Kind{A{public int code(){return 17;}},B{public int code(){return 31;}};public abstract int code();}
 public interface Access{Object token();Access next();}
 public static Access make(final Object token){return new Access(){
  public Object token(){return token;}
  public Access next(){final Access original=this;return new Access(){public Object token(){return original.token();}public Access next(){return original;}};}
 };}
}
class EnumForestDriver{public static void main(String[]args){Object sentinel=new Object();int rows=0;
 for(Object token:new Object[]{null,sentinel}){EnumForestOwner.Access first=EnumForestOwner.make(token),second=first.next();if(first.token()!=token||second.token()!=token||second.next()!=first)throw new AssertionError("capture identity");
 if(!first.getClass().isAnonymousClass()||!second.getClass().isAnonymousClass()||first.getClass().getEnclosingClass()!=EnumForestOwner.class||second.getClass().getEnclosingClass()!=first.getClass()||!second.getClass().getEnclosingMethod().getName().equals("next"))throw new AssertionError("lexical ownership");rows++;}
 if(EnumForestOwner.Kind.A.code()!=17||EnumForestOwner.Kind.B.code()!=31||!EnumForestOwner.Kind.A.getClass().isAnonymousClass()||EnumForestOwner.Kind.A.getClass().getSuperclass()!=EnumForestOwner.Kind.class)throw new AssertionError("enum dispatch/ownership");
 System.out.println(rows+":enum-role:nested-anonymous:alias:ownership");}}
`

func TestAdversarialAnonymousThisLocalAliasRetainsIdentityAndLexicalOwner(t *testing.T) {
	for _, owner := range []string{"EnumForestOwner", "RenamedEnumForestOwner"} {
		t.Run(owner, func(t *testing.T) {
			source := strings.ReplaceAll(anonymousThisAliasFixture, "EnumForestOwner", owner)
			testNativePrivateSetterCompiledFixture(t, owner, "EnumForestDriver", "2:enum-role:nested-anonymous:alias:ownership\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": source}, debug, "8")
			})
		})
	}
}
