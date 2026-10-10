package javaclassparser

import (
	"strings"
	"testing"
)

// The root is unrelated to Parent. Only Worker inherits the protected member
// type; Reader obtains that type scope from its proved lexical Worker owner.
// The original caller is never recompiled, and the superclass callback observes
// the enclosing capture before the source-level constructor body can execute.
func nativeInheritedContextSources(shape string) map[string]string {
	echo := `public Object echo(Object token) {
  Value v = make(token);
  if (v.outer() != this) throw new AssertionError("foreign enclosing identity");
  return v.token;
 }`
	if shape == "deep" || shape == "deep-call" {
		echo = `Value create(Object token) { return make(token); }
  public Object echo(Object token) { return new Reader().echo(token); }
  class Reader {
   Object echo(Object token) {
    Value v = create(token);
    if (v.outer() != Worker.this) throw new AssertionError("foreign enclosing identity");
    return v.token;
   }
  }`
		if shape == "deep-call" {
			echo = strings.Replace(echo, "Value create(Object token) { return make(token); }\n  ", "", 1)
			echo = strings.Replace(echo, "Value v = create(token);", "Value v = make(token);", 1)
		}
	}
	decoy := ""
	if shape == "shadow" {
		decoy = `public static class Value {
   public final Object token;
   public Value(Object token) { this.token = token; }
  }`
	}
	return map[string]string{
		"base/Parent.java": `package base;
public abstract class Parent {
 public final Object observed;
 protected Parent() { observed = observe(); }
 protected abstract Object observe();
 protected class Value {
  public final Object token;
  Value(Object token) { this.token = token; }
  public Object outer() { return Parent.this; }
 }
 protected Value make(Object token) { return new Value(token); }
}`,
		"use/Owner.java": `package use;
public class Owner {
 final Object token;
 public Owner(Object token) { this.token = token; }
 ` + decoy + `
 public class Worker extends base.Parent {
  public Worker() {}
  protected Object observe() { return Owner.this.token; }
  ` + echo + `
 }
 public Worker worker() { return new Worker(); }
}`,
		"use/Driver.java": `package use;
public class Driver {
 public static void main(String[] args) {
  Object shared = new Object();
  int rows = 0;
  for (Object outer : new Object[]{null, shared, "same", new String("same")}) {
   Owner owner = new Owner(outer);
   Owner.Worker worker;
   try { worker = owner.worker(); }
   catch (NullPointerException failure) {
    throw new AssertionError("early inherited-context capture timing", failure);
   }
   for (Object token : new Object[]{null, shared, "same", new String("same")}) {
    if (worker.observed != outer || worker.echo(token) != token ||
        worker.getClass().getDeclaringClass() != Owner.class)
     throw new AssertionError("inherited binding/capture timing");
    rows++;
   }
  }
  System.out.println(rows + ":inherited-context");
 }
}`,
	}
}

func TestAdversarialInheritedMemberDeclarationUsesOriginalLexicalContext(t *testing.T) {
	for _, rename := range []string{"original", "renamed"} {
		for _, shape := range []string{"direct", "deep", "deep-call", "shadow"} {
			t.Run(rename+"/"+shape, func(t *testing.T) {
				sources := nativeInheritedContextSources(shape)
				owners := []string{"base/Parent", "use/Owner"}
				if rename == "renamed" {
					replacer := strings.NewReplacer("Parent", "DeclaringBase", "Owner", "EnclosingScope", "Worker", "Runner", "Reader", "Inspector", "Value", "Cursor")
					renamed := map[string]string{}
					for name, source := range sources {
						renamed[replacer.Replace(name)] = replacer.Replace(source)
					}
					sources = renamed
					for i := range owners {
						owners[i] = replacer.Replace(owners[i])
					}
				}
				testNativeIndependentCompiledFamilyFixture(t, func(debug string) map[string][]byte {
					return nativeCompileSourceReleaseClasses(t, sources, debug, "8")
				}, owners, "use.Driver", "16:inherited-context\n", nil, nativeLexicalExactSignatures)
			})
		}
	}
}
