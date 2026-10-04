package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"testing"
)

func TestNativeClassAnnotationOwnershipRequiresOriginalRetentionAndTarget(t *testing.T) {
	files := nativeCompileClasses(t, `package policy;import java.lang.annotation.*;
@Retention(RetentionPolicy.RUNTIME) @Target(ElementType.TYPE) @interface RuntimeMark{}
@Target(ElementType.TYPE) @interface DefaultClass{}
@Retention(RetentionPolicy.RUNTIME) @Target(ElementType.TYPE_USE) @interface TypeUse{}
@interface NoTarget{}
@Retention(RetentionPolicy.SOURCE) @Target(ElementType.TYPE) @interface SourceMark{}
@Retention(RetentionPolicy.RUNTIME) @Target(ElementType.METHOD) @interface MethodOnly{}
class NotAnnotation{}
@RuntimeMark @DefaultClass @TypeUse @NoTarget class Subject{}`)
	for _, scenario := range []string{"original", "missing definition", "wrong definition identity", "not annotation", "runtime in invisible table", "class in visible table", "source retention", "method only target", "duplicate use", "nil use", "budget", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			obj, err := Parse(files["policy/Subject.class"])
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(obj)
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				if scenario == "missing definition" {
					return nil, false
				}
				if scenario == "wrong definition identity" {
					return files["policy/Subject.class"], true
				}
				raw, ok := files[name+".class"]
				return raw, ok
			}
			var visible, invisible *RuntimeVisibleAnnotationsAttribute
			for _, a := range obj.Attributes {
				if table, ok := a.(*RuntimeVisibleAnnotationsAttribute); ok {
					if table.IsInvisible {
						invisible = table
					} else {
						visible = table
					}
				}
			}
			if visible == nil || invisible == nil {
				t.Fatal("fixture lacks both retention tables")
			}
			switch scenario {
			case "not annotation":
				visible.Annotations[0].TypeName = "Lpolicy/NotAnnotation;"
			case "runtime in invisible table":
				visible.IsInvisible = true
			case "class in visible table":
				invisible.IsInvisible = false
			case "source retention":
				visible.Annotations[0].TypeName = "Lpolicy/SourceMark;"
			case "method only target":
				visible.Annotations[0].TypeName = "Lpolicy/MethodOnly;"
			case "duplicate use":
				visible.Annotations = append(visible.Annotations, visible.Annotations[0])
			case "nil use":
				visible.Annotations = append(visible.Annotations, nil)
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := d.nativeMemberAnnotationTablesRepresentable(); got != (scenario == "original") {
				t.Fatalf("class annotation representable=%v", got)
			}
		})
	}
}

func TestNativeClassAnnotationPolicyRejectsMalformedOriginalDefinitions(t *testing.T) {
	files := nativeCompileClasses(t, `import java.lang.annotation.*;@Retention(RetentionPolicy.RUNTIME) @Target(ElementType.TYPE) @interface Defined{}`)
	for _, scenario := range []string{"duplicate retention", "duplicate target", "wrong retention enum", "wrong target enum", "unknown retention", "wrong value name", "invisible policy", "nil policy", "nil element", "wrong target shape"} {
		t.Run(scenario, func(t *testing.T) {
			obj, err := Parse(files["Defined.class"])
			if err != nil {
				t.Fatal(err)
			}
			var table *RuntimeVisibleAnnotationsAttribute
			var retention, target *AnnotationAttribute
			for _, a := range obj.Attributes {
				if x, ok := a.(*RuntimeVisibleAnnotationsAttribute); ok {
					table = x
					for _, ann := range x.Annotations {
						if ann.TypeName == "Ljava/lang/annotation/Retention;" {
							retention = ann
						}
						if ann.TypeName == "Ljava/lang/annotation/Target;" {
							target = ann
						}
					}
				}
			}
			if table == nil || retention == nil || target == nil {
				t.Fatal("definition policy")
			}
			switch scenario {
			case "duplicate retention":
				table.Annotations = append(table.Annotations, retention)
			case "duplicate target":
				table.Annotations = append(table.Annotations, target)
			case "wrong retention enum":
				retention.ElementValuePairs[0].Value.(*EnumConstValue).TypeName = "LDefined;"
			case "wrong target enum":
				target.ElementValuePairs[0].Value.([]*ElementValuePairAttribute)[0].Value.(*EnumConstValue).TypeName = "LDefined;"
			case "unknown retention":
				retention.ElementValuePairs[0].Value.(*EnumConstValue).ConstName = "UNKNOWN"
			case "wrong value name":
				retention.ElementValuePairs[0].Name = "wrong"
			case "invisible policy":
				table.IsInvisible = true
			case "nil policy":
				table.Annotations = append(table.Annotations, nil)
			case "nil element":
				retention.ElementValuePairs[0] = nil
			case "wrong target shape":
				target.ElementValuePairs[0].Tag = 's'
				target.ElementValuePairs[0].Value = "TYPE"
			}
			if _, _, valid := nativeAnnotationDeclarationPolicy(obj, NewClassObjectDumper(obj)); valid {
				t.Fatal("malformed original declaration policy accepted")
			}
		})
	}
}
