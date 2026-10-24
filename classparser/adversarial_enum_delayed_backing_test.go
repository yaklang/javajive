package javaclassparser

import (
	"github.com/yaklang/javajive/classparser/decompiler/core"
	"os"
	"strings"
	"testing"
)

// A JVM-valid movement of the compiler factory past a user callback changes
// what values() observes during initialization. Dominance and exactly-once
// execution alone cannot authorize regenerating the array earlier in source.
func TestAdversarialEnumBackingRefusesObservableDelayedInitialization(t *testing.T) {
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native javac8 required")
	}
	fixture := `public enum DelayedBackingOwner{LEFT,RIGHT;static{DelayedBackingEffects.after=DelayedBackingEffects.sample();}}class DelayedBackingEffects{static String after;static String sample(){try{DelayedBackingOwner.values();return "ready";}catch(NullPointerException e){return "not-ready";}}}class DelayedBackingDriver{public static void main(String[]args){DelayedBackingOwner[] v=DelayedBackingOwner.values();if(v.length!=2||v[0]!=DelayedBackingOwner.LEFT||v[1]!=DelayedBackingOwner.RIGHT||!DelayedBackingEffects.after.equals("not-ready"))throw new AssertionError("original delayed producer order");System.out.println("delayed:backing:original:order");}}`
	files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, "DelayedBackingOwner", "none", false)
	obj, e := Parse(files["DelayedBackingOwner.class"])
	if e != nil {
		t.Fatal(e)
	}
	changed := false
	for _, m := range obj.Methods {
		n, _ := sourceBridgeUTF8(obj, m.NameIndex)
		if n != "<clinit>" {
			continue
		}
		ops, known := nativeEnumMethodOps(obj, m, nil)
		if !known {
			t.Fatal("original initializer")
		}
		start, end := -1, -1
		for i, op := range ops {
			member := constructorMotionMember(obj, op, core.OP_PUTSTATIC)
			if member != nil && member.Name == "DelayedBackingOwner" && member.Member == "$VALUES" {
				if i < 10 {
					t.Fatal("original inline array packet")
				}
				start = int(ops[i-10].CurrentOffset)
				end = int(op.CurrentOffset) + 3
			}
		}
		if start < 0 || end < 0 {
			t.Fatal("original compiler backing packet")
		}
		for _, a := range m.Attributes {
			if code, ok := a.(*CodeAttribute); ok {
				if len(code.Attributes) != 0 || len(code.ExceptionTable) != 0 || code.Code[len(code.Code)-1] != core.OP_RETURN {
					t.Fatal("straight-line legal mutant domain")
				}
				packet := append([]byte(nil), code.Code[start:end]...)
				prefix := append([]byte(nil), code.Code[:start]...)
				prefix = append(prefix, code.Code[end:len(code.Code)-1]...)
				prefix = append(prefix, packet...)
				prefix = append(prefix, core.OP_RETURN)
				if len(prefix) != len(code.Code) {
					t.Fatal("mutant length changed")
				}
				code.Code = prefix
				changed = true
			}
		}
	}
	if !changed {
		t.Fatal("no original packet moved")
	}
	files["DelayedBackingOwner.class"] = obj.Bytes()
	_, java := t04Tools(t)
	if got := t04RunJava(t, java, sourceArtifactWriteFiles(t, files), "DelayedBackingDriver"); got != "delayed:backing:original:order\n" {
		t.Fatal("original legal bytecode oracle", got)
	}
	for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
		t.Run(policy, func(t *testing.T) {
			if policy == "no-source-rewrites" {
				t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
			}
			if policy == "no-core-cleanups" {
				t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
			}
			z := nativeArchive(t, files)
			defer z.Close()
			source, err := z.ReadFile("DelayedBackingOwner.class")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(source), "decompile dump failed") || !strings.Contains(string(source), "backing producer follows non-regenerated initializer effects") {
				t.Fatalf("source enum cannot move its implicit backing ahead of an observable original effect\n%s", source)
			}
		})
	}
}
