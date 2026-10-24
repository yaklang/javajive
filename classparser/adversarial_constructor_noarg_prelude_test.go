package javaclassparser

import (
	"bytes"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func constructorNoargPreludeOriginal(t *testing.T, debug, category string, nop bool) map[string][]byte {
	t.Helper()
	fixture := strings.Replace(constructorUnusedPrefixFixture, "static String trace;", "static String trace;static Object input;static int mode;", 1)
	fixture = strings.Replace(fixture, "UnusedPrefixBase(Object input,int mode){this.input=input;", "UnusedPrefixBase(){this.input=UnusedPrefixEffects.input;int mode=UnusedPrefixEffects.mode;", 1)
	fixture = strings.Replace(fixture, "super(input,mode);", "super();", 1)
	fixture = strings.Replace(fixture, `UnusedPrefixEffects.trace="";`, `UnusedPrefixEffects.trace="";UnusedPrefixEffects.input=token;UnusedPrefixEffects.mode=mode;`, 1)
	if category == "wide" {
		fixture = strings.Replace(fixture, "static Object[] before", "static long before", 1)
		fixture = strings.Replace(fixture, "mode==4?null:new Object[]{new Object()}", "Long.MIN_VALUE+mode", 1)
		fixture = strings.Replace(fixture, "Object[] unused=", "long unused=", 1)
	}
	if category == "zero arguments" {
		fixture = strings.Replace(fixture, "before(int mode){", "before(){int mode=UnusedPrefixEffects.mode;", 1)
		fixture = strings.Replace(fixture, "UnusedPrefixEffects.before(mode)", "UnusedPrefixEffects.before()", 1)
	}
	files := nativeCompileSourceReleaseClasses(t, map[string]string{"UnusedPrefixOwner.java": fixture}, debug, "8")
	object := mustParseLowering(t, bytes.Clone(files["UnusedPrefixOwner.class"]))
	for _, method := range object.Methods {
		name, _ := sourceBridgeUTF8(object, method.NameIndex)
		if name != "<init>" {
			continue
		}
		code := constructorLoweringCode(method)
		store := byte(0x4e) // astore_3
		if category == "wide" {
			store = 0x42 // lstore_3
		}
		// Independent authored fixture: javac emits this/SUPER in bytes 0..3,
		// followed by parameter/static-call/dead-store (or a no-argument call).
		// The production packet recognizer and lowering do not build the original.
		end, call := 9, 5
		if category == "zero arguments" {
			end, call = 8, 4
		}
		if code == nil {
			t.Fatal("missing authored constructor Code")
		}
		if len(code.ExceptionTable) != 0 || len(code.Code) <= end || code.Code[0] != 0x2a || code.Code[1] != 0xb7 || call == 5 && code.Code[4] != 0x1c || code.Code[call] != 0xb8 || code.Code[end-1] != store {
			t.Fatalf("independent compiler fixture changed: %x", code.Code)
		}
		old := bytes.Clone(code.Code)
		padding := 0
		code.Code = nil
		if nop {
			code.Code = append(code.Code, 0)
			padding = 1
		}
		code.Code = append(code.Code, old[4:end]...)
		code.Code = append(code.Code, old[:4]...)
		code.Code = append(code.Code, old[end:]...)
		code.AttrLen += uint32(padding)
		for _, attr := range code.Attributes {
			switch table := attr.(type) {
			case *LineNumberTableAttribute:
				for _, row := range table.LineNumberTable {
					pc := int(row.StartPc)
					if pc < 4 {
						pc += end - 4
					} else if pc < end {
						pc -= 4
					}
					row.StartPc = uint16(pc + padding)
				}
			case *UnparsedAttribute:
				if table.Name != "LocalVariableTable" || len(table.Info) < 2 || len(table.Info) != 2+10*int(binary.BigEndian.Uint16(table.Info)) {
					t.Fatal("unexpected authored debug table", table.Name)
				}
				for at := 2; at < len(table.Info); at += 10 {
					start := int(binary.BigEndian.Uint16(table.Info[at:]))
					length := int(binary.BigEndian.Uint16(table.Info[at+2:]))
					slot := int(binary.BigEndian.Uint16(table.Info[at+8:]))
					if slot == 3 && start == end {
						start, length = end-4+padding, length+4
					} else if start == 0 && length == len(old) {
						length += padding
					} else {
						t.Fatal("unexpected authored local interval")
					}
					binary.BigEndian.PutUint16(table.Info[at:], uint16(start))
					binary.BigEndian.PutUint16(table.Info[at+2:], uint16(length))
				}
			default:
				t.Fatalf("unexpected authored Code attribute %T", attr)
			}
		}
	}
	files["UnusedPrefixOwner.class"] = object.Bytes()
	return files
}

func TestAdversarialConstructorNoargPreludePreservesOriginalAbruptOrder(t *testing.T) {
	for _, category := range []string{"reference", "wide", "zero arguments"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, padding := range []string{"plain", "nop"} {
				for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
					t.Run(category+"/"+debug+"/"+padding+"/"+string(profile), func(t *testing.T) {
						files := constructorNoargPreludeOriginal(t, debug, category, padding == "nop")
						for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
							t.Run(policy, func(t *testing.T) {
								if policy == "no-source-rewrites" {
									t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
								}
								if policy == "no-core-cleanups" {
									t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
								}
								constructorNoargPreludeRoundTrip(t, files, debug, profile)
							})
						}
					})
				}
			}
		}
	}
}

func constructorNoargPreludeRoundTrip(t *testing.T, files map[string][]byte, debug string, profile SourceCompilerProfile) ([]byte, []byte, *JarReconstructionArtifact) {
	t.Helper()
	compiler, java := t04Tools(t)
	if profile == NativeJavac8 {
		compiler = os.Getenv("JAVA8_JAVAC")
		if compiler == "" {
			t.Fatal("independent native compiler required")
		}
	}
	run := func(dir string, succeeds bool) {
		t.Helper()
		for _, assertions := range []string{"-ea", "-da"} {
			out, err := enumFreshOracleRun(t, java, assertions, dir, "UnusedPrefixDriver")
			if succeeds && (err != nil || string(out) != "10:unused:prefix:ordered:effects\n") {
				t.Fatal("original-first prefix oracle", err, string(out))
			}
			if !succeeds && (err == nil || !strings.Contains(string(out), "original pre-SUPER call/order/identity")) {
				t.Fatal("unlowered order counterfactual inactive", err, string(out))
			}
		}
	}
	run(sourceArtifactWriteFiles(t, files), true)
	const owner = "UnusedPrefixOwner"
	jar := filepath.Join(t.TempDir(), "original.jar")
	if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{owner + ".class": files[owner+".class"]}), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := NewJarReconstructionArtifactFromLocal(jar, 8, profile, func(n string) ([]byte, bool) { b, ok := files[n+".class"]; return bytes.Clone(b), ok })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	source, err := a.ReadFile(owner + ".class")
	if err != nil || legacySourceHasFailureMarker(string(source)) {
		t.Fatal("complete source publication", err, string(source))
	}
	deps := map[string][]byte{}
	for name, raw := range files {
		if name != owner+".class" {
			deps[name] = raw
		}
	}
	out := sourceArtifactWriteFiles(t, deps)
	path := filepath.Join(out, owner+".java")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-proc:none", "--release", "8", "-g:" + debug}
	if profile == NativeJavac8 {
		args = []string{"-proc:none", "-source", "8", "-target", "8", "-g:" + debug}
	}
	args = append(args, "-cp", out, "-sourcepath", out, "-d", out, path)
	if log, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
		t.Fatal("fresh constructor compilation", err, string(log), string(source))
	}
	run(out, false)
	fresh, err := os.ReadFile(filepath.Join(out, owner+".class"))
	if err != nil {
		t.Fatal(err)
	}
	saved := bytes.Clone(fresh)
	finished, err := a.FinishClass(owner+".class", fresh)
	if err != nil || !bytes.Equal(saved, fresh) {
		t.Fatal("owned postcompile completion", err)
	}
	if err := sourceMethodMetadataClosed(mustParseLowering(t, files[owner+".class"]), finished, nil); err != nil {
		t.Fatal("original method ABI", err)
	}
	if again, err := a.FinishClass(owner+".class", finished); err != nil || !bytes.Equal(again, finished) {
		t.Fatal("idempotent completion", err)
	}
	if err := os.WriteFile(filepath.Join(out, owner+".class"), finished, 0600); err != nil {
		t.Fatal(err)
	}
	run(out, true)
	return files[owner+".class"], fresh, a
}
