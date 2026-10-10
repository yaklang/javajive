package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const nativeMemberDeepMarkerFixture = `class DeepMarkerEffects{static String trace="";static boolean fail;static final RuntimeException failure=new RuntimeException("same");}
class NativeArchiveOwner{static Runnable make(final Object x){return new Runnable(){public void run(){if(x==null)throw new IllegalArgumentException();DeepMarkerEffects.trace+="R";}};}static Object nested(){return Middle.build();}static class Middle{static Object build(){return new Deep();}static class Deep{private Deep(){DeepMarkerEffects.trace+="D";if(DeepMarkerEffects.fail)throw DeepMarkerEffects.failure;}}}}
class DeepMarkerDriver{public static void main(String[]args){Object token=new Object();int rows=0;for(boolean fail:new boolean[]{false,true})for(Object x:new Object[]{null,token}){DeepMarkerEffects.trace="";DeepMarkerEffects.fail=fail;Runnable r=NativeArchiveOwner.make(x);try{r.run();if(x==null)throw new AssertionError("missing null failure");}catch(IllegalArgumentException e){if(x!=null)throw e;}try{Object d=NativeArchiveOwner.nested();if(fail||!d.getClass().getName().equals("NativeArchiveOwner$Middle$Deep"))throw new AssertionError("deep identity");}catch(RuntimeException e){if(!fail||e!=DeepMarkerEffects.failure)throw new AssertionError("deep failure",e);}if(!DeepMarkerEffects.trace.equals(x==null?"D":"RD"))throw new AssertionError("trace");rows++;}System.out.println(rows+":"+DeepMarkerEffects.trace);}}`

func TestNativeMemberDeepMarkerSharesOriginalAnonymousIdentity(t *testing.T) {
	javac, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, nativeMemberDeepMarkerFixture, debug)
			original := t.TempDir()
			for n, raw := range files {
				if e := os.WriteFile(filepath.Join(original, n), raw, 0600); e != nil {
					t.Fatal(e)
				}
			}
			oracle := t04RunJava(t, java, original, "DeepMarkerDriver")
			if oracle != "4:RD\n" {
				t.Fatalf("original %q", oracle)
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
					for _, n := range []string{"NativeArchiveOwner$Middle", "NativeArchiveOwner$Middle$Deep", "NativeArchiveOwner$1"} {
						raw, e := z.ReadFile(n + ".class")
						if e != nil || !strings.Contains(string(raw), "body owned by") {
							t.Fatalf("ownership %s %v\n%s", n, e, raw)
						}
					}
					src, e := z.ReadFile("NativeArchiveOwner.class")
					if e != nil || strings.Contains(string(src), DecompileStubMarker) {
						t.Fatalf("source %v\n%s", e, src)
					}
					output := t.TempDir()
					for n, raw := range files {
						if strings.HasPrefix(n, "NativeArchiveOwner") {
							continue
						}
						if e := os.WriteFile(filepath.Join(output, n), raw, 0600); e != nil {
							t.Fatal(e)
						}
					}
					file := filepath.Join(output, "NativeArchiveOwner.java")
					if e := os.WriteFile(file, src, 0600); e != nil {
						t.Fatal(e)
					}
					if out, e := exec.Command(javac, "-proc:none", "--release", "8", "-cp", output, "-d", output, file).CombinedOutput(); e != nil {
						t.Fatalf("rebuilt %v %s\n%s", e, out, src)
					}
					if got := t04RunJava(t, java, output, "DeepMarkerDriver"); got != oracle {
						t.Fatalf("JVM %s != %s", got, oracle)
					}
					for n, want := range files {
						if !strings.HasPrefix(n, "NativeArchiveOwner") {
							continue
						}
						raw, e := os.ReadFile(filepath.Join(output, n))
						if e != nil {
							t.Fatal(e)
						}
						if got := nativeBinaryShape(t, raw); got != nativeBinaryShape(t, want) {
							t.Fatalf("ABI %s\n%s\n%s", n, nativeBinaryShape(t, want), got)
						}
					}
				})
			}
		})
	}
}
