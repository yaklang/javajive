package javaclassparser

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Metadata is part of the owned declaration, not a prefix to discard while
// extracting its class header. Arrays and annotation strings deliberately
// contain braces and class keywords so a textual header search is not evidence.
func TestNativeMemberClassAnnotationsRetainOriginalMetadataAndCapture(t *testing.T) {
	const source = `package marks;
import java.lang.annotation.*;
@Retention(RetentionPolicy.RUNTIME) @Target(ElementType.TYPE) @interface VisibleClass{String text();int[] values();Class<?> type();Mode mode();Detail detail();}
@Retention(RetentionPolicy.CLASS) @Target(ElementType.TYPE) @interface HiddenClass{String value();}
@Retention(RetentionPolicy.RUNTIME) @interface Detail{String value();}
enum Mode{FIRST,SECOND}
class Parent{final Object seen;Parent(long n){seen=owner();}Object owner(){return null;}}
@Deprecated @VisibleClass(text="root",values={3,5,7},type=String.class,mode=Mode.FIRST,detail=@Detail("root-inner")) @HiddenClass("root-hidden")
class NativeAnnotationOwner{
 @Deprecated @VisibleClass(text=" class Bogus { } package false; ",values={-1,0,2147483647},type=Child.class,mode=Mode.SECOND,detail=@Detail("child-inner")) @HiddenClass("child-hidden")
 class Child extends Parent{Child(){super(0L);}Object owner(){return NativeAnnotationOwner.this;}}
 Child make(){return new Child();}
}
class Driver{public static void main(String[]args){NativeAnnotationOwner outer=new NativeAnnotationOwner();NativeAnnotationOwner.Child c=outer.make();if(c.seen!=outer||c.owner()!=outer)throw new AssertionError("capture");VisibleClass root=NativeAnnotationOwner.class.getDeclaredAnnotation(VisibleClass.class),child=NativeAnnotationOwner.Child.class.getDeclaredAnnotation(VisibleClass.class);if(!NativeAnnotationOwner.class.isAnnotationPresent(Deprecated.class)||!NativeAnnotationOwner.Child.class.isAnnotationPresent(Deprecated.class)||root==null||child==null||root.mode()!=Mode.FIRST||child.mode()!=Mode.SECOND||child.type()!=NativeAnnotationOwner.Child.class||!child.detail().value().equals("child-inner"))throw new AssertionError("metadata");System.out.println(root.text()+":"+java.util.Arrays.toString(root.values())+":"+child.text()+":"+java.util.Arrays.toString(child.values())+":"+child.type().getName()+":"+child.mode()+":"+child.detail().value());}}
`
	javac, java := t04Tools(t)
	snapshot := func(t *testing.T, raw []byte) string {
		t.Helper()
		obj, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		var annotations []string
		markerCount := 0
		for _, attr := range obj.Attributes {
			if d, ok := attr.(*DeprecatedAttribute); ok {
				if d == nil || d.AttrLen != 0 {
					t.Fatal("invalid original Deprecated marker")
				}
				markerCount++
			}
			if a, ok := attr.(*RuntimeVisibleAnnotationsAttribute); ok {
				data, err := json.Marshal(struct {
					Invisible   bool
					Annotations []*AnnotationAttribute
				}{a.IsInvisible, a.Annotations})
				if err != nil {
					t.Fatal(err)
				}
				annotations = append(annotations, string(data))
			}
		}
		if markerCount != 1 {
			t.Fatalf("Deprecated marker changed: %d", markerCount)
		}
		if len(annotations) != 2 {
			t.Fatalf("both original retention tables required, got%v", annotations)
		}
		data, _ := json.Marshal(annotations)
		return string(data)
	}
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := nativeCompileDebugClasses(t, source, debug)
			original := t.TempDir()
			for name, raw := range files {
				file := filepath.Join(original, name)
				if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			oracle := t04RunJava(t, java, original, "marks.Driver")
			for _, policy := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
				t.Run(policy, func(t *testing.T) {
					if policy == "no-source-rewrites" {
						t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
					}
					if policy == "no-core-cleanups" {
						t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
					}
					z := nativeArchive(t, files)
					child, err := z.ReadFile("marks/NativeAnnotationOwner$Child.class")
					if err != nil || !strings.Contains(string(child), "original member body owned by") {
						t.Fatalf("ownership %v %s", err, child)
					}
					output := t.TempDir()
					for name, raw := range files {
						if strings.HasPrefix(name, "marks/NativeAnnotationOwner") {
							continue
						}
						file := filepath.Join(output, name)
						if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
							t.Fatal(err)
						}
						if err := os.WriteFile(file, raw, 0600); err != nil {
							t.Fatal(err)
						}
					}
					src, err := z.ReadFile("marks/NativeAnnotationOwner.class")
					if err != nil || strings.Contains(string(src), DecompileStubMarker) {
						t.Fatalf("source %v %s", err, src)
					}
					file := filepath.Join(output, "NativeAnnotationOwner.java")
					if err := os.WriteFile(file, src, 0600); err != nil {
						t.Fatal(err)
					}
					if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", output, "-d", output, file).CombinedOutput(); err != nil {
						t.Fatalf("rebuilt %v %s\n%s", err, out, src)
					}
					if got := t04RunJava(t, java, output, "marks.Driver"); got != oracle {
						t.Fatalf("JVM metadata/capture changed %s != %s", got, oracle)
					}
					for _, name := range []string{"marks/NativeAnnotationOwner.class", "marks/NativeAnnotationOwner$Child.class"} {
						raw, err := os.ReadFile(filepath.Join(output, name))
						if err != nil {
							t.Fatal(err)
						}
						if want, got := snapshot(t, files[name]), snapshot(t, raw); got != want {
							t.Fatalf("original annotation value graph changed %s\n%s\n%s", name, want, got)
						}
					}
				})
			}
		})
	}
}
