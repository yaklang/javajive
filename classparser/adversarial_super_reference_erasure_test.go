package javaclassparser

import (
	"strings"
	"testing"
)

// The SAM's instantiated String port and the superclass declaration's erased
// Object port are different contracts. A super method reference keeps both:
// an arrow can accidentally replace the compiler helper's physical descriptor.
const superReferenceErasureFixture = `interface CheckedPair<T,R>{R apply(T value,java.io.IOException failure)throws java.io.IOException;}
class GenericParent<T> {
 int calls;Object seen;boolean initialized;
 GenericParent(){initialized=true;}
 public Object take(T value,java.io.IOException failure)throws java.io.IOException{if(!initialized)throw new AssertionError("uninitialized receiver");calls++;seen=value;if(failure!=null)throw failure;return value;}
}
public class SuffixOwner extends GenericParent<String> {
 final CheckedPair<String,Object> action;
 SuffixOwner(){super();action=super::take;}
 public Object take(String value,java.io.IOException failure){throw new AssertionError("virtual dispatch");}
}
class SuffixDriver {public static void main(String[] args)throws Exception {
 int rows=0;java.io.IOException failure=new java.io.IOException("identity");
 for(Object value:new Object[]{null,"text",new Object(),Integer.valueOf(19)})for(boolean fail:new boolean[]{false,true}){
  SuffixOwner owner=new SuffixOwner();if(owner.calls!=0||!owner.initialized)throw new AssertionError("factory invokes target");
  CheckedPair raw=owner.action;Object result=null;Throwable caught=null;
  try{result=raw.apply(value,fail?failure:null);}catch(Throwable e){caught=e;}
  if(value!=null&&!(value instanceof String)){if(!(caught instanceof ClassCastException)||owner.calls!=0||owner.seen!=null)throw new AssertionError("SAM adaptation before body",caught);}
  else if(owner.calls!=1||owner.seen!=value||caught!=(fail?failure:null)||!fail&&result!=value)throw new AssertionError("special target / identity / checked exception",caught);
  rows++;
 }
 System.out.println(rows+":super:generic:constructor:erasure");}}
`

func TestAdversarialSuperReferenceKeepsConstructorAndGenericDeclarationPorts(t *testing.T) {
	for _, kind := range []string{"constructor concrete", "method generic", "constructor generic"} {
		for _, profile := range []SourceCompilerProfile{NativeJavac8, ModernJavac} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(kind+"/"+string(profile)+"/"+debug, func(t *testing.T) {
					fixture := superReferenceErasureFixture
					if kind == "constructor concrete" {
						fixture = strings.NewReplacer("GenericParent<T>", "GenericParent", "GenericParent<String>", "GenericParent", "take(T value", "take(String value").Replace(fixture)
					}
					if kind == "method generic" {
						fixture = strings.ReplaceAll(fixture, "action=super::take;", "action=make();")
						fixture = strings.ReplaceAll(fixture, "public Object take(String", "CheckedPair<String,Object> make(){return super::take;} public Object take(String")
					}
					constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, fixture, debug, []string{"SuffixOwner"}, "8:super:generic:constructor:erasure\n", 8, 8, profile)
				})
			}
		}
	}
}

func TestAdversarialSuperReferenceSubstitutesReifiableArrayInputs(t *testing.T) {
	for _, rank := range []string{"[]", "[][]"} {
		t.Run(rank, func(t *testing.T) {
			value := "new String[]{\"element\"}"
			if rank == "[][]" {
				value = "new String[][]{new String[]{\"element\"}}"
			}
			fixture := strings.NewReplacer("GenericParent<String>", "GenericParent<String"+rank+">", "CheckedPair<String,Object>", "CheckedPair<String"+rank+",Object>", "take(String value", "take(String"+rank+" value", "instanceof String", "instanceof String"+rank, `new Object[]{null,"text",new Object()`, "new Object[]{null,"+value+",new Object()").Replace(superReferenceErasureFixture)
			constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, fixture, "source,lines,vars", []string{"SuffixOwner"}, "8:super:generic:constructor:erasure\n", 8, 8, ModernJavac)
		})
	}
}

// The declaration provider must prove a JDK parent through the same original
// generic edge as an application parent; a hand-authored lookalike alone does
// not exercise that boundary. The unmodified caller checks SAM casts and the
// selected superclass's null/checked-exception contract.
func TestAdversarialSuperReferenceBindsExternalGenericDeclaration(t *testing.T) {
	const fixture = `interface CheckedPair<A,B,R>{R apply(A value,B failure)throws java.io.IOException;}
public class SuffixOwner extends java.nio.file.SimpleFileVisitor<java.nio.file.Path> {
 final CheckedPair<java.nio.file.Path,java.io.IOException,java.nio.file.FileVisitResult> action;
 protected SuffixOwner(){action=super::visitFileFailed;}
 public java.nio.file.FileVisitResult visitFileFailed(java.nio.file.Path value,java.io.IOException failure){throw new AssertionError("virtual dispatch");}
}
class SuffixDriver {public static void main(String[] args)throws Exception {
 int rows=0;java.io.IOException failure=new java.io.IOException("identity");
 for(Object value:new Object[]{null,java.nio.file.Paths.get("authored-path"),new Object()})for(boolean fail:new boolean[]{false,true}){
  SuffixOwner owner=new SuffixOwner();CheckedPair raw=owner.action;Throwable caught=null;
  try{raw.apply(value,fail?failure:null);}catch(Throwable e){caught=e;}
  if(value!=null&&!(value instanceof java.nio.file.Path)){if(!(caught instanceof ClassCastException))throw new AssertionError("SAM cast",caught);}
  else if(value==null||!fail){if(!(caught instanceof NullPointerException))throw new AssertionError("super null contract",caught);}
  else if(caught!=failure)throw new AssertionError("special target / exception identity",caught);
  rows++;
 }System.out.println(rows+":super:external:binding:exceptions");}}
`
	for _, profile := range []SourceCompilerProfile{NativeJavac8, ModernJavac} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			t.Run(string(profile)+"/"+debug, func(t *testing.T) {
				constructorSuffixFamilyAtTargetWithCompilerRoundTrip(t, fixture, debug, []string{"SuffixOwner"}, "6:super:external:binding:exceptions\n", 8, 8, profile)
			})
		}
	}
}
