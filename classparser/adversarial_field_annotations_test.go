package javaclassparser

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Retention CLASS is deliberately checked in the classfile, independently of
// reflection (which cannot distinguish a missing invisible annotation). Field
// initialization forms and enum constant spelling must share the same metadata.
func TestAdversarialFieldDeclarationAnnotationsPreserveVisibility(t *testing.T) {
	files := nativeCompileClasses(t, `import java.lang.annotation.*;
@Retention(RetentionPolicy.RUNTIME) @Target(ElementType.FIELD) @interface VisibleField {String value();}
@Retention(RetentionPolicy.CLASS) @Target(ElementType.FIELD) @interface InvisibleField {int value();}
@Retention(RetentionPolicy.RUNTIME) @Target({ElementType.FIELD,ElementType.TYPE_USE}) @interface BothField {String value();}
class AnnotationFields {
 @VisibleField("constant") @InvisibleField(1) static final int constant=7;
 @VisibleField("blank") @InvisibleField(2) final Object blank;
 @VisibleField("computed") @InvisibleField(3) static final Object computed;
 @VisibleField("ordinary") @InvisibleField(4) @BothField("reference") Object ordinary;
@BothField("array") Object[] array;
@BothField("generic") java.util.List<@BothField("argument") String> generic;
 AnnotationFields(Object n){blank=n;} static {computed=new Object();}
}
enum AnnotationEnum {@VisibleField("first") @InvisibleField(5) FIRST,@VisibleField("second") @InvisibleField(6) SECOND;}`)
	javac, _ := t04Tools(t)
	snapshot := func(t *testing.T, raw []byte) string {
		t.Helper()
		obj, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		attrs := map[string][]string{}
		for _, field := range obj.Fields {
			name, ok := sourceBridgeUTF8(obj, field.NameIndex)
			if !ok {
				t.Fatal("field name")
			}
			for _, attribute := range field.Attributes {
				if a, ok := attribute.(*TypeAnnotationsAttribute); ok {
					b, err := json.Marshal(struct {
						Invisible   bool
						Annotations []*TypeAnnotation
					}{a.IsInvisible, a.Annotations})
					if err != nil {
						t.Fatal(err)
					}
					attrs[name] = append(attrs[name], string(b))
				}
				if a, ok := attribute.(*RuntimeVisibleAnnotationsAttribute); ok {
					b, err := json.Marshal(struct {
						Invisible   bool
						Annotations []*AnnotationAttribute
					}{a.IsInvisible, a.Annotations})
					if err != nil {
						t.Fatal(err)
					}
					attrs[name] = append(attrs[name], string(b))
				}
			}
		}
		b, err := json.Marshal(attrs)
		if err != nil {
			t.Fatal(err)
		}
		if len(attrs) == 0 {
			t.Fatal("missing original or rebuilt field annotations")
		}
		return string(b)
	}
	for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
		t.Run(string(mode), func(t *testing.T) {
			output := t.TempDir()
			resolve := func(n string) ([]byte, bool) { raw, ok := files[n+".class"]; return raw, ok }
			for _, name := range []string{"VisibleField", "InvisibleField", "BothField"} {
				if err := os.WriteFile(filepath.Join(output, name+".class"), files[name+".class"], 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"AnnotationFields", "AnnotationEnum"} {
				raw := files[name+".class"]
				var source string
				var err error
				if mode == "legacy" {
					source, err = DecompileWithResolver(raw, resolve)
				} else {
					var result DecompileResult
					result, err = DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolve})
					source = result.Source
				}
				if err != nil || strings.Contains(source, DecompileStubMarker) {
					t.Fatalf("source %v %s", err, source)
				}
				file := filepath.Join(output, name+".java")
				if err := os.WriteFile(file, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", output, "-d", output, file).CombinedOutput(); err != nil {
					t.Fatalf("rebuilt %v %s\n%s", err, out, source)
				}
				rebuilt, err := os.ReadFile(filepath.Join(output, name+".class"))
				if err != nil {
					t.Fatal(err)
				}
				if want, got := snapshot(t, raw), snapshot(t, rebuilt); want != got {
					t.Fatalf("field annotations changed\n%s\n%s", want, got)
				}
			}
		})
	}
}
