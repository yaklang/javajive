package javaclassparser

import (
	"strings"
	"testing"
)

const initializerTypeScopeFixture = `interface InitializerScopeBound{int number();}class InitializerScopeToken implements InitializerScopeBound{final int n;InitializerScopeToken(int n){this.n=n;}public int number(){return n;}}interface InitializerScopeSupplier<T>{T get();}
class InitializerScopeOwner<T extends InitializerScopeBound>{T value;final InitializerScopeSupplier<T> supplier=new InitializerScopeSupplier<T>(){public T get(){return value;}};InitializerScopeOwner(){}InitializerScopeOwner(T token){value=token;}}
class InitializerScopeDriver{public static void main(String[]args)throws Exception{int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(boolean missing:new boolean[]{false,true}){InitializerScopeToken token=missing?null:new InitializerScopeToken(n);for(InitializerScopeOwner<InitializerScopeToken> o:new InitializerScopeOwner[]{new InitializerScopeOwner<InitializerScopeToken>(),new InitializerScopeOwner<InitializerScopeToken>(token)}){if(o.supplier.get()!=o.value||o.supplier.get()!=null&&o.supplier.get().number()!=n||o.supplier.getClass().getDeclaredMethod("get").getReturnType()!=InitializerScopeBound.class)throw new AssertionError("initializer lexical binding, identity and word");rows++;}}System.out.println(rows+":initializer:lexical");}}`

func TestAdversarialInitializerTypeScopeAcrossOriginalConstructors(t *testing.T) {
	for _, prefix := range []string{"InitializerScope", "RenamedInitializer"} {
		t.Run(prefix, func(t *testing.T) {
			for _, kind := range []string{"field", "named block", "delegating"} {
				t.Run(kind, func(t *testing.T) {
					source := initializerTypeScopeFixture
					owner := "InitializerScopeOwner$1"
					if kind == "named block" {
						source = strings.ReplaceAll(source, "final InitializerScopeSupplier<T> supplier=new InitializerScopeSupplier<T>(){public T get(){return value;}};", "final InitializerScopeSupplier<T> supplier;{class Entry implements InitializerScopeSupplier<T>{public T get(){return value;}}supplier=new Entry();}")
						owner = "InitializerScopeOwner$1Entry"
					}
					if kind == "delegating" {
						source = strings.ReplaceAll(source, "InitializerScopeOwner(){}", "InitializerScopeOwner(){this(null);}")
					}
					source = strings.ReplaceAll(source, "InitializerScope", prefix)
					owner = strings.ReplaceAll(owner, "InitializerScope", prefix)
					testIndependentFlatClosedCalleeFamily(t, source, prefix, "20:initializer:lexical\n", []string{owner})
				})
			}
		})
	}
}
