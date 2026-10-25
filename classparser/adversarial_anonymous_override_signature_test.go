package javaclassparser

import (
	"strings"
	"testing"
)

// The anonymous class owns no type variables. Its method refers to the outer
// class binders while its generic superclass owns an independent result binder.
// An unchanged driver checks both dispatch and reflective declaration identity.
func anonymousOverrideSignatureFixture() string {
	source := strings.Replace(projectedOverrideSignatureFixture,
		"class Sub extends Task<V>{V apply(Pair<K,V> input){return input.value;}}",
		"Task<V> task(){return new Task<V>(){V apply(Pair<K,V> input){return input.value;}};}", 1)
	source = strings.ReplaceAll(source, "new Sub()", "task()")
	source = strings.Replace(source, "SuffixOwner.Sub.class.getDeclaredMethod", "new SuffixOwner<Integer,String>().task().getClass().getDeclaredMethod", 1)
	source = strings.Replace(source, "int rows=0;", `Class<?> anonymous=new SuffixOwner<Integer,String>().task().getClass();
 if(!anonymous.isAnonymousClass()||anonymous.getTypeParameters().length!=0||!anonymous.getEnclosingMethod().getName().equals("task"))throw new AssertionError("anonymous lexical identity");
 int rows=0;`, 1)
	source = strings.ReplaceAll(source, "outer.new Sub()", "outer.task()")
	return source
}

func TestAdversarialAnonymousOverrideRetainsLexicalBindings(t *testing.T) {
	source := anonymousOverrideSignatureFixture()
	for label, owner := range map[string]string{"original": "SuffixOwner", "renamed": "RenamedAnonymous", "supplementary_unicode": "𐐀Owner"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(label+"/"+debug, func(t *testing.T) {
				t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
				t.Setenv("JDEC_NATIVE_ANONYMOUS_OFF", "1")
				constructorSuffixFamilyRoundTrip(t, strings.ReplaceAll(source, "SuffixOwner", owner), debug, []string{owner, owner + "$Pair", owner + "$Task", owner + "$1"}, "12:projected:override:identity:binding\n")
			})
		}
	}
}

// Captured locals are distinct constructor ports after an enclosing instance
// and a varargs SUPER operand. Neither arity nor slot position identifies them.
func anonymousCapturedOverrideFixture() string {
	source := anonymousOverrideSignatureFixture()
	source = strings.Replace(source, "abstract class Task<T>{abstract", "abstract class Task<T>{Task(int...options){if(options.length!=2||options[0]!=3||options[1]!=7)throw new AssertionError(\"super operands\");}abstract", 1)
	source = strings.Replace(source, "Task<V> task(){return new Task<V>(){V apply(Pair<K,V> input){return input.value;}};}", "Task<V> task(final V captured,final boolean choose){return new Task<V>(3,7){V apply(Pair<K,V> input){return choose?captured:input.value;}};}", 1)
	source = strings.ReplaceAll(source, "task()", "task(null,false)")
	source = strings.Replace(source, "SuffixOwner<Integer,String>.Task<String> task=outer.task(null,false);", "SuffixOwner<Integer,String>.Task<String> task=outer.task(value,true);", 1)
	source = strings.Replace(source, "if(outer.execute(input)!=value||task.apply(input)!=value||input.key!=key)", "SuffixOwner.Pair<Integer,String> other=new SuffixOwner.Pair<Integer,String>(key,new String(\"other\")); if(outer.execute(input)!=value||task.apply(other)!=value||outer.task(null,false).apply(input)!=value||input.key!=key)", 1)
	return source
}

func TestAdversarialAnonymousOverrideCapturedPortsRemainDistinct(t *testing.T) {
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			t.Setenv("JDEC_NATIVE_MEMBER_OFF", "1")
			t.Setenv("JDEC_NATIVE_ANONYMOUS_OFF", "1")
			constructorSuffixFamilyRoundTrip(t, anonymousCapturedOverrideFixture(), debug, []string{"SuffixOwner", "SuffixOwner$Pair", "SuffixOwner$Task", "SuffixOwner$1"}, "12:projected:override:identity:binding\n")
		})
	}
}
