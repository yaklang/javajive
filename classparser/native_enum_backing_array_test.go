package javaclassparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
)

// This is valid JVM bytecode, not an invalid class-file shape. The hidden
// backing array is populated in reverse declaration order while name/ordinal
// and both executable factories remain canonical. Source automatically creates
// declaration-order values(); raw fallback must refuse this semantic change.
func TestNativeEnumRefusesValidReorderedInlineBackingArray(t *testing.T) {
	const fixture = `enum BackingChoice{LEFT,RIGHT;}class BackingDriver{public static void main(String[]args){BackingChoice[] values=BackingChoice.values();if(values.length!=2||values[0]!=BackingChoice.RIGHT||values[1]!=BackingChoice.LEFT||values[0].ordinal()!=1||values[1].ordinal()!=0||BackingChoice.valueOf("LEFT")!=BackingChoice.LEFT)throw new AssertionError("original reordered backing");System.out.println("reordered:backing:ordinal:identity");}}`
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, fixture, debug)
			obj, err := Parse(files["BackingChoice.class"])
			if err != nil {
				t.Fatal(err)
			}
			var helper *MemberInfo
			var array, initializer *CodeAttribute
			for _, m := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, m.NameIndex)
				for _, a := range m.Attributes {
					if c, ok := a.(*CodeAttribute); ok {
						if n == "$values" {
							helper, array = m, c
						}
						if n == "<clinit>" {
							initializer = c
						}
					}
				}
			}
			if helper == nil || array == nil || initializer == nil {
				t.Fatal("original compiler packets")
			}
			decoder := core.NewDecompiler(array.Code, nil)
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			var reads []*core.OpCode
			for _, op := range decoder.Opcodes() {
				if nativeEnumOpcode(op, core.OP_GETSTATIC) {
					reads = append(reads, op)
				}
			}
			if len(reads) != 2 {
				t.Fatal("original array fields")
			}
			packet := append([]byte(nil), array.Code[:len(array.Code)-1]...)
			a, b := int(reads[0].CurrentOffset)+1, int(reads[1].CurrentOffset)+1
			packet[a], packet[b] = packet[b], packet[a]
			packet[a+1], packet[b+1] = packet[b+1], packet[a+1]
			decoder = core.NewDecompiler(initializer.Code, nil)
			if err := decoder.ParseOpcode(); err != nil {
				t.Fatal(err)
			}
			pc := -1
			for _, op := range decoder.Opcodes() {
				if nativeEnumMemberOperand(obj, op, core.OP_INVOKESTATIC, obj.GetClassName(), "$values", "()[LBackingChoice;") {
					pc = int(op.CurrentOffset)
				}
			}
			if pc < 0 {
				t.Fatal("array helper invocation")
			}
			code := append([]byte(nil), initializer.Code[:pc]...)
			code = append(code, packet...)
			code = append(code, initializer.Code[pc+3:]...)
			initializer.Code, initializer.Attributes = code, nil
			initializer.AttrLen = uint32(12 + len(code))
			if initializer.MaxStack < array.MaxStack {
				initializer.MaxStack = array.MaxStack
			}
			methods := obj.Methods[:0]
			for _, m := range obj.Methods {
				if m != helper {
					methods = append(methods, m)
				}
			}
			obj.Methods = methods
			files["BackingChoice.class"] = obj.Bytes()
			original := t.TempDir()
			for n, raw := range files {
				if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := t04RunJava(t, java, original, "BackingDriver"); got != "reordered:backing:ordinal:identity\n" {
				t.Fatalf("original oracle:%q", got)
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
					raw, err := z.ReadFile("BackingChoice.class")
					if err != nil || !strings.Contains(string(raw), "enum regeneration") || !strings.Contains(string(raw), "decompile dump failed") {
						t.Fatalf("original backing semantics must be explicitly refused:%v\n%s", err, raw)
					}
				})
			}
		})
	}
}

// Canonical producer instructions are insufficient if they execute before
// constant assignments. The original factories then return two null elements.
// Both erased regions must be proved in order on the original CFG.
func TestNativeEnumRefusesBackingProducedBeforeConstants(t *testing.T) {
	const fixture = `enum EarlyBackingChoice{LEFT,RIGHT;}class EarlyBackingDriver{public static void main(String[]args){EarlyBackingChoice[] v=EarlyBackingChoice.values();if(v.length!=2||v[0]!=null||v[1]!=null||EarlyBackingChoice.LEFT==null||EarlyBackingChoice.RIGHT==null)throw new AssertionError("original early backing");try{EarlyBackingChoice.valueOf("RIGHT");throw new AssertionError("missing original enum lookup failure");}catch(NullPointerException expected){}System.out.println("early:backing:nulls:constants:identity");}}`
	_, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, fixture, debug)
			obj, err := Parse(files["EarlyBackingChoice.class"])
			if err != nil {
				t.Fatal(err)
			}
			modified := false
			for _, method := range obj.Methods {
				n, _ := sourceBridgeUTF8(obj, method.NameIndex)
				if n != "<clinit>" {
					continue
				}
				ops, known := nativeEnumMethodOps(obj, method, nil)
				if !known || len(ops) != 15 || !nativeEnumOpcode(ops[12], core.OP_INVOKESTATIC) || !nativeEnumOpcode(ops[13], core.OP_PUTSTATIC) {
					t.Fatal("original backing packet")
				}
				start, end := int(ops[12].CurrentOffset), int(ops[14].CurrentOffset)
				for _, a := range method.Attributes {
					if code, ok := a.(*CodeAttribute); ok {
						packet := append([]byte(nil), code.Code[start:end]...)
						packet = append(packet, code.Code[:start]...)
						packet = append(packet, code.Code[end:]...)
						code.Code, code.Attributes = packet, nil
						code.AttrLen = uint32(12 + len(packet))
						modified = true
					}
				}
			}
			if !modified {
				t.Fatal("no producer moved")
			}
			files["EarlyBackingChoice.class"] = obj.Bytes()
			original := t.TempDir()
			for n, raw := range files {
				if err := os.WriteFile(filepath.Join(original, n), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if got := t04RunJava(t, java, original, "EarlyBackingDriver"); got != "early:backing:nulls:constants:identity\n" {
				t.Fatalf("original oracle %q", got)
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
					raw, err := z.ReadFile("EarlyBackingChoice.class")
					if err != nil || !strings.Contains(string(raw), "enum regeneration") || !strings.Contains(string(raw), "decompile dump failed") {
						t.Fatalf("early backing must be refused:%v\n%s", err, raw)
					}
				})
			}
		})
	}
}
