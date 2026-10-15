package javaclassparser

import "testing"

// Type tests observe the runtime type, not the fields whose stores move. A
// checked view must retain THIS on every alias so later effects stay checked.
func TestNativeConstructorReceiverTypeViewsPreserveCaptureObservation(t *testing.T) {
	const source = `interface ViewMarker {}
 class ViewInstanceBase {int result;ViewInstanceBase(int n){result=this instanceof ViewMarker?n:-n;}}
 final class ViewInstanceChild extends ViewInstanceBase implements ViewMarker {Object capture;ViewInstanceChild(int n){super(n);}}
 class ViewCastBase {int result;ViewCastBase(int n){Object alias=(ViewMarker)this;result=alias==this?n:-n;}}
 final class ViewCastChild extends ViewCastBase implements ViewMarker {Object capture;ViewCastChild(int n){super(n);}}
 final class ViewFailChild extends ViewCastBase {Object capture;ViewFailChild(int n){super(n);}}
 class ViewBooleanBase {static boolean result;ViewBooleanBase(int n){result=this instanceof ViewMarker;}}
 final class ViewBooleanChild extends ViewBooleanBase implements ViewMarker {Object capture;ViewBooleanChild(int n){super(n);}}
 class ViewReadBase {Object capture,seen;ViewReadBase(int n){Object alias=(ViewMarker)this;seen=((ViewReadBase)alias).capture;}}
 final class ViewReadChild extends ViewReadBase implements ViewMarker {ViewReadChild(int n){super(n);}}
 class ViewWriteBase {Object capture;ViewWriteBase(int n){Object alias=(ViewMarker)this;((ViewWriteBase)alias).capture=null;}}
 final class ViewWriteChild extends ViewWriteBase implements ViewMarker {ViewWriteChild(int n){super(n);}}
 class ViewPublishBase {static Object saved;ViewPublishBase(int n){saved=(ViewMarker)this;}}
 final class ViewPublishChild extends ViewPublishBase implements ViewMarker {Object capture;ViewPublishChild(int n){super(n);}}
 class ViewCallBase {static void save(Object x){}ViewCallBase(int n){save((ViewMarker)this);}}
 final class ViewCallChild extends ViewCallBase implements ViewMarker {Object capture;ViewCallChild(int n){super(n);}}
 class ViewHeapBase {Object self,seen;ViewHeapBase(int n){self=(ViewMarker)this;seen=self;}}
 final class ViewHeapChild extends ViewHeapBase implements ViewMarker {Object capture;ViewHeapChild(int n){super(n);}}
 class ViewOpenChild extends ViewCastBase implements ViewMarker {Object capture;ViewOpenChild(int n){super(n);}}
 `
	files := nativeCompileClasses(t, source)
	resolve := func(name string) ([]byte, bool) { b, ok := files[name+".class"]; return b, ok }
	for _, tc := range []struct {
		child, base, field string
		safe               bool
	}{
		{"ViewInstanceChild", "ViewInstanceBase", "ViewInstanceChild", true},
		{"ViewCastChild", "ViewCastBase", "ViewCastChild", true},
		{"ViewFailChild", "ViewCastBase", "ViewFailChild", true},
		{"ViewBooleanChild", "ViewBooleanBase", "ViewBooleanChild", true},
		{"ViewReadChild", "ViewReadBase", "ViewReadBase", false},
		{"ViewWriteChild", "ViewWriteBase", "ViewWriteBase", false},
		{"ViewPublishChild", "ViewPublishBase", "ViewPublishChild", false},
		{"ViewCallChild", "ViewCallBase", "ViewCallChild", false},
		{"ViewHeapChild", "ViewHeapBase", "ViewHeapChild", false},
		{"ViewOpenChild", "ViewCastBase", "ViewOpenChild", false},
	} {
		t.Run(tc.child, func(t *testing.T) {
			obj, e := Parse(files[tc.child+".class"])
			if e != nil {
				t.Fatal(e)
			}
			d := &ClassObjectDumper{obj: obj, foldSiblingResolver: resolve}
			d.options.TargetSourceVersion = 8
			writes := map[string]bool{tc.field + "\x00capture\x00Ljava/lang/Object;": true}
			if got := d.constructorCaptureChainDoesNotObserve(tc.base, "(I)V", writes); got != tc.safe {
				t.Fatalf("type-view observation=%v want=%v", got, tc.safe)
			}
		})
	}
}
