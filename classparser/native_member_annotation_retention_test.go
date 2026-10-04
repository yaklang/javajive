package javaclassparser

import (
	"context"
	"fmt"
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

// Platform annotations are resolved from the same exact original classfile
// evidence as archive declarations. An explicit provider, even invalid, wins.
func TestNativeClassAnnotationUsesOriginalPlatformDefinition(t *testing.T) {
	files := nativeCompileClasses(t, `@Deprecated class PlatformAnnotationSubject{}`)
	for _, release := range []int{8, 9, 11, 16, 17, 21, 10} {
		for _, scenario := range []string{"original platform", "invalid explicit", "wrong identity", "wrong table"} {
			t.Run(fmt.Sprintf("%d/%s", release, scenario), func(t *testing.T) {
				obj, err := Parse(files["PlatformAnnotationSubject.class"])
				if err != nil {
					t.Fatal(err)
				}
				d := NewClassObjectDumper(obj)
				d.options.TargetSourceVersion = release
				switch scenario {
				case "invalid explicit":
					d.foldSiblingResolver = func(string) ([]byte, bool) { return []byte{0, 1}, true }
				case "wrong identity":
					d.declarationResolver = func(string) ([]byte, bool) { return files["PlatformAnnotationSubject.class"], true }
				case "wrong table":
					for _, a := range obj.Attributes {
						if table, ok := a.(*RuntimeVisibleAnnotationsAttribute); ok {
							table.IsInvisible = true
						}
					}
				}
				want := scenario == "original platform" && release != 10
				if got := d.nativeMemberAnnotationTablesRepresentable(); got != want {
					t.Fatalf("original policy=%v want=%v", got, want)
				}
			})
		}
	}
}

func TestNativeClassDeprecatedMarkerRequiresExactOriginalPair(t *testing.T) {
	files := nativeCompileClasses(t, `@Deprecated class MarkerPair{}`)
	for _, scenario := range []string{"original", "unpaired marker", "unpaired annotation", "duplicate marker", "duplicate annotation", "nonempty marker", "nil marker", "invisible annotation", "budget", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			obj, err := Parse(files["MarkerPair.class"])
			if err != nil {
				t.Fatal(err)
			}
			var marker *DeprecatedAttribute
			var table *RuntimeVisibleAnnotationsAttribute
			for _, a := range obj.Attributes {
				if d, ok := a.(*DeprecatedAttribute); ok {
					marker = d
				}
				if d, ok := a.(*RuntimeVisibleAnnotationsAttribute); ok {
					table = d
				}
			}
			if marker == nil || table == nil || len(table.Annotations) != 1 {
				t.Fatal("independent javac fixture lacks exact marker pair")
			}
			var work *workbudget.Budget
			switch scenario {
			case "unpaired marker":
				table.Annotations = nil
			case "unpaired annotation":
				var attrs []AttributeInfo
				for _, a := range obj.Attributes {
					if _, ok := a.(*DeprecatedAttribute); !ok {
						attrs = append(attrs, a)
					}
				}
				obj.Attributes = attrs
			case "duplicate marker":
				obj.Attributes = append(obj.Attributes, marker)
			case "duplicate annotation":
				table.Annotations = append(table.Annotations, table.Annotations[0])
			case "nonempty marker":
				marker.AttrLen = 1
			case "nil marker":
				obj.Attributes = append(obj.Attributes, (*DeprecatedAttribute)(nil))
			case "invisible annotation":
				table.IsInvisible = true
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeMemberDeprecatedMarkerRepresentable(obj, work); got != (scenario == "original") {
				t.Fatalf("original marker pair proof=%v", got)
			}
		})
	}
}
