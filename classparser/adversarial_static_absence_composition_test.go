package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func staticAbsenceCompositionFixture(t *testing.T, owner, debug, kind string) ([]byte, []byte, *JarReconstructionArtifact) {
	t.Helper()
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native target compiler required")
	}
	source := `public final class OWNER {public static int calls;public static final long fold(long n){calls++;return n*31L+calls;}}`
	if kind == "legacy empty" {
		source = `public final class OWNER {}`
	}
	source = strings.ReplaceAll(source, "OWNER", owner)
	check := `int rows=0;for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){rows++;long expected=java.math.BigInteger.valueOf(n).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(rows)).longValue();if(OWNER.fold(n)!=expected||OWNER.calls!=rows)throw new AssertionError("word/effect/order");}if(c.getDeclaredMethod("fold",long.class).isSynthetic()!=MARKER)throw new AssertionError("method marker");`
	if kind == "legacy empty" {
		check = "int rows=0;"
	}
	check = strings.ReplaceAll(check, "MARKER", fmt.Sprint(kind == "synthetic method"))
	driver := strings.ReplaceAll(`class AbsenceDriver{public static void main(String[]a)throws Exception{Class<?> c=Class.forName("OWNER",false,AbsenceDriver.class.getClassLoader());if(c.getDeclaredConstructors().length!=0)throw new AssertionError("constructor inventory");try{c.getDeclaredConstructor();throw new AssertionError("default constructor");}catch(NoSuchMethodException expected){}CHECK System.out.println(rows+":absence:method:word:effect");}}`, "CHECK", check)
	driver = strings.ReplaceAll(driver, "OWNER", owner)
	files := nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": source, "AbsenceDriver.java": driver}, debug, "8")
	if strings.HasPrefix(kind, "legacy") {
		dir := t.TempDir()
		path := filepath.Join(dir, owner+".java")
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
		if log, err := exec.Command(native, "-proc:none", "-source", "1.4", "-target", "1.4", "-g:"+debug, "-d", dir, path).CombinedOutput(); err != nil {
			t.Fatal("actual legacy compiler", err, string(log))
		}
		raw, err := os.ReadFile(filepath.Join(dir, owner+".class"))
		if err != nil {
			t.Fatal(err)
		}
		files[owner+".class"] = raw
	}
	original := mustParseLowering(t, files[owner+".class"])
	expectedVersion := uint16(52)
	if strings.HasPrefix(kind, "legacy") {
		expectedVersion = 48
	}
	if original.MajorVersion != expectedVersion {
		t.Fatal("actual compiler target", original.MajorVersion)
	}
	kept := []*MemberInfo{}
	removed := 0
	for _, m := range original.Methods {
		name, _ := sourceBridgeUTF8(original, m.NameIndex)
		if name == "<init>" {
			removed++
			continue
		}
		if kind == "synthetic method" && name == "fold" {
			m.AccessFlags |= syntheticMethodFlag
		}
		kept = append(kept, m)
	}
	if removed != 1 {
		t.Fatal("author only absent original default")
	}
	original.Methods = kept
	raw := original.Bytes()
	files[owner+".class"] = raw
	expected := "5:absence:method:word:effect\n"
	if kind == "legacy empty" {
		expected = "0:absence:method:word:effect\n"
	}
	run := func(dir, stage string) {
		t.Helper()
		for _, mode := range []string{"-ea", "-da"} {
			out, err := enumFreshOracleRun(t, java, mode, dir, "AbsenceDriver")
			if err != nil || string(out) != expected {
				t.Fatal("original-first oracle", stage, err, string(out))
			}
		}
	}
	run(sourceArtifactWriteFiles(t, files), "original")
	jar := filepath.Join(t.TempDir(), "authored.jar")
	if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{owner + ".class": raw}), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	generated, err := a.ReadFile(owner + ".class")
	if err != nil || legacySourceHasFailureMarker(string(generated)) {
		t.Fatal("complete source", err, string(generated))
	}
	out := sourceArtifactWriteFiles(t, map[string][]byte{"AbsenceDriver.class": files["AbsenceDriver.class"], owner + ".java": generated})
	if log, err := exec.Command(modern, "-proc:none", "--release", "8", "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out, filepath.Join(out, owner+".java")).CombinedOutput(); err != nil {
		t.Fatal("fresh compile", err, string(log), string(generated))
	}
	path := filepath.Join(out, owner+".class")
	fresh, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	saved := bytes.Clone(fresh)
	finished, err := a.FinishClass(owner+".class", fresh)
	if err != nil {
		t.Fatal("constructor absence and declaration markers", err)
	}
	if !bytes.Equal(fresh, saved) {
		t.Fatal("input changed")
	}
	obj, done := mustParseLowering(t, fresh), mustParseLowering(t, finished)
	for _, m := range done.Methods {
		name, _ := sourceBridgeUTF8(done, m.NameIndex)
		desc, _ := sourceBridgeUTF8(done, m.DescriptorIndex)
		if name == "<init>" {
			t.Fatal("constructor retained")
		}
		found := false
		for _, prior := range obj.Methods {
			n, _ := sourceBridgeUTF8(obj, prior.NameIndex)
			d, _ := sourceBridgeUTF8(obj, prior.DescriptorIndex)
			if n == name && d == desc {
				found = true
				if !reflect.DeepEqual(constructorLoweringCode(prior), constructorLoweringCode(m)) {
					t.Fatal("fresh executable changed")
				}
			}
		}
		if !found {
			t.Fatal("invented method")
		}
	}
	if again, err := a.FinishClass(owner+".class", finished); err != nil || !bytes.Equal(again, finished) {
		t.Fatal("not idempotent", err)
	}
	if err := os.WriteFile(path, finished, 0600); err != nil {
		t.Fatal(err)
	}
	run(out, "candidate")
	return raw, fresh, a
}

func TestAdversarialStaticAbsenceComposesWithLegacyVersionAndSyntheticMethods(t *testing.T) {
	for _, kind := range []string{"legacy empty", "legacy algorithm", "synthetic method"} {
		for _, owner := range []string{"StaticPacket", "RenamedFactory"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(kind+"/"+owner+"/"+debug, func(t *testing.T) { staticAbsenceCompositionFixture(t, owner, debug, kind) })
			}
		}
	}
}

func TestAdversarialStaticAbsenceRequiresExactNonSyntheticMethodFlags(t *testing.T) {
	original, fresh, artifact := staticAbsenceCompositionFixture(t, "StaticPacket", "none", "synthetic method")
	for _, variant := range []string{"missing marker", "visibility", "static", "final", "bridge", "synchronized", "descriptor", "extra method", "invented marker"} {
		t.Run(variant, func(t *testing.T) {
			obj := mustParseLowering(t, bytes.Clone(fresh))
			var method *MemberInfo
			for _, m := range obj.Methods {
				if n, _ := sourceBridgeUTF8(obj, m.NameIndex); n == "fold" {
					method = m
				}
			}
			if method == nil {
				t.Fatal("original ordinary static method")
			}
			old := bytes.Clone(original)
			switch variant {
			case "visibility":
				method.AccessFlags &^= 1
			case "static":
				method.AccessFlags &^= 8
			case "final":
				method.AccessFlags &^= 16
			case "bridge":
				method.AccessFlags |= 64
			case "synchronized":
				method.AccessFlags |= 32
			case "descriptor":
				method.DescriptorIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("(I)J"))
			case "extra method":
				copy := *method
				copy.NameIndex = uint16(obj.ConstantPoolManager.AddUtf8Info("other"))
				obj.Methods = append(obj.Methods, &copy)
			case "invented marker":
				// Put the new marker on the compiler default, which had no original
				// constructor declaration and cannot borrow another method's marker.
				for _, m := range obj.Methods {
					if n, _ := sourceBridgeUTF8(obj, m.NameIndex); n == "<init>" {
						m.AccessFlags |= syntheticMethodFlag
					}
				}
			}
			input := obj.Bytes()
			if _, err := Parse(input); err != nil {
				t.Fatal("semantic counterexample", err)
			}
			saved := bytes.Clone(input)
			result, err := artifact.source.jar.sourceConstructorLowering.lowerStaticClassDefaultConstructor("StaticPacket", old, input, nil)
			if err != nil {
				t.Fatal(err)
			}
			if variant == "missing marker" {
				if bytes.Equal(result, input) {
					t.Fatal("closed default not removed")
				}
				done := mustParseLowering(t, result)
				for _, m := range done.Methods {
					if n, _ := sourceBridgeUTF8(done, m.NameIndex); n == "fold" && m.AccessFlags&syntheticMethodFlag != 0 {
						t.Fatal("executable step stole metadata ownership")
					}
				}
			} else if !bytes.Equal(result, input) {
				t.Fatal("invalid declaration authorized constructor deletion")
			}
			if !bytes.Equal(input, saved) {
				t.Fatal("input mutated")
			}
		})
	}
}
