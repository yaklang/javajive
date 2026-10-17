package javaclassparser

import "testing"

func TestAdversarialUnrelatedAnonymousForestKeepsLocalPrivateCalls(t *testing.T) {
	const source = `
class ScopeRoot {int calls;boolean fail;final RuntimeException expected=new IllegalStateException("same");
 private Object read(Object token){calls++;if(fail)throw expected;return token;}
 class Iterator {java.util.function.Supplier<Object> next(final Object token){class Entry implements java.util.function.Supplier<Object>{public Object get(){return read(token);}}return new Entry();}}
 class View {java.util.function.Supplier<Object> create(final Object token){return new java.util.function.Supplier<Object>(){public Object get(){return ScopeRoot.this.read(token);}};}}
}
class ScopeDriver {public static void main(String[]args){int rows=0;for(boolean fail:new boolean[]{false,true})for(boolean nil:new boolean[]{false,true}){
 ScopeRoot root=new ScopeRoot();root.fail=fail;Object token=nil?null:new Object();java.util.function.Supplier<Object> local=root.new Iterator().next(token),anonymous=root.new View().create(token);
 if(root.calls!=0)throw new AssertionError("eager private call");for(java.util.function.Supplier<Object> s:new java.util.function.Supplier[]{local,anonymous,local}){try{Object value=s.get();if(fail||value!=token)throw new AssertionError("private call result");}catch(RuntimeException actual){if(!fail||actual!=root.expected)throw new AssertionError("same failure",actual);}}
 if(root.calls!=3||!local.getClass().isLocalClass()||local.getClass().getEnclosingClass()!=ScopeRoot.Iterator.class||!local.getClass().getEnclosingMethod().getName().equals("next")||!anonymous.getClass().isAnonymousClass()||anonymous.getClass().getEnclosingClass()!=ScopeRoot.View.class)throw new AssertionError("call count or lexical scope");rows++;
 }System.out.println(rows+":independent-scopes:private-call:identity:null:deferred");}}
`
	testResourceCompletionFamily(t, source, "ScopeRoot", "ScopeDriver", "4:independent-scopes:private-call:identity:null:deferred\n")
}
