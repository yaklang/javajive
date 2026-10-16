package javaclassparser

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/internal/workbudget"
)

func TestNativeConstructorConfinedCaptureRetainsEscapeBoundaries(t *testing.T) {
	rows := []struct {
		name, body, holder string
		safe               bool
	}{
		{"own storage", "retained=holder;", "", true},
		{"discard", "int unused=13;", "", true},
		{"nested holder", "retained=new $H(holder);", "", true},
		{"ignored parameter", "send(holder);", "class $H{$H(Object ignored){}}", true},
		{"same-name ordinary method", "retained=holder;", "class $H{final Object ref;$H(Object value){ref=value;}void finalize(int ignored){throw new AssertionError();}}", true},
		{"static publication", "escaped=holder;", "", false},
		{"argument publication", "send(holder);", "", false},
		{"local alias publication", "Object alias=holder;escaped=alias;", "", false},
		{"receiver operation", "holder.touch();", "", false},
		{"mirror is not THIS", "holder.getClass();", "", false},
		{"holder field observation", "send(holder.ref);", "", false},
		{"own field observation", "retained=holder;send(retained);", "", false},
		{"array publication", "Object[]array=new Object[1];array[0]=holder;", "", false},
		{"sibling publication", "$B box=new $B();box.ref=holder;escaped=box;", "", false},
		{"returned carrier", "retained=make();", "", false},
		{"constructor publication", "retained=holder;", "class $H{final Object ref;$H(Object value){ref=value;$P.send(value);}}", false},
		{"constructor callback", "retained=holder;", "class $H{final Object ref;$H(Object value){ref=value;value.toString();}}", false},
		{"holder finalizer", "retained=holder;", "class $H{final Object ref;$H(Object value){ref=value;}protected void finalize(){$P.send(ref);}}", false},
		{"volatile storage", "retained=holder;", "class $H{volatile Object ref;$H(Object value){ref=value;}}", false},
		{"foreign parent", "retained=holder;", "class $Super{$Super(){$P.send(this);}}class $H extends $Super{final Object ref;$H(Object value){ref=value;}}", false},
	}
	var source strings.Builder
	for i, row := range rows {
		prefix := fmt.Sprintf("Confinement%d", i)
		name := strings.NewReplacer("$P", prefix+"Parent", "$C", prefix+"Child", "$H", prefix+"Holder", "$B", prefix+"Box", "$Super", prefix+"Super")
		holder := row.holder
		if holder == "" {
			holder = "class $H{final Object ref;$H(Object value){ref=value;}void touch(){$P.send(ref);}}"
		}
		source.WriteString(name.Replace("class $P{Object retained;static Object escaped;static void send(Object value){escaped=value;}private Object make(){return new $H(this);}$P(){$H holder=new $H(this);" + row.body + "}}final class $C extends $P{final Object value;$C(Object value){super();this.value=value;}}class $B{Object ref;}" + holder))
	}
	files := nativeCompileClasses(t, source.String())
	for i, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			prefix := fmt.Sprintf("Confinement%d", i)
			child, err := Parse(bytes.Clone(files[prefix+"Child.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(child)
			d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }
			writes := map[string]bool{prefix + "Child\x00value\x00Ljava/lang/Object;": true}
			if got := d.constructorCaptureChainDoesNotObserve(prefix+"Parent", "()V", writes); got != row.safe {
				t.Fatalf("capture movement safe=%v want %v", got, row.safe)
			}
		})
	}
}

func TestNativeConstructorConfinedCaptureFrameAndCallRoles(t *testing.T) {
	plain := constructorEffectValue{kind: 'L'}
	root := constructorEffectValue{kind: 'L', receiver: true}
	held := constructorEffectValue{kind: 'L', retainsReceiver: true}
	if plain.reachesReceiver() || !root.reachesReceiver() || !held.reachesReceiver() || held.receiver {
		t.Fatal("reachability and exact receiver identity conflated")
	}
	keys := map[string]bool{}
	for _, value := range []constructorEffectValue{plain, root, held} {
		key := constructorEffectFrameKey(3, []constructorEffectValue{value}, nil, true, nil, 0)
		if keys[key] {
			t.Fatal("different receiver roles share a cached frame")
		}
		keys[key] = true
	}
	prior := &constructorEffectLoopFrame{locals: []constructorEffectValue{plain}, initialized: true}
	if _, _, accepted := constructorEffectLoopJoin(prior, []constructorEffectValue{held}, nil, true); accepted {
		t.Fatal("loop join discarded receiver reachability")
	}
	files := nativeCompileClasses(t, `class ConfinedRole{final Object accept(Object value){return value;}}`)
	obj, err := Parse(files["ConfinedRole.class"])
	if err != nil {
		t.Fatal(err)
	}
	d := NewClassObjectDumper(obj)
	member := values.NewJavaClassMember("ConfinedRole", "accept", "(Ljava/lang/Object;)Ljava/lang/Object;", nil)
	remaining := 512
	if _, accepted := d.constructorReceiverBindMethod(obj, member, core.OP_INVOKEVIRTUAL, &remaining, held); accepted {
		t.Fatal("receiver-reaching argument acquired receiver-free call permission")
	}
	d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
	if _, accepted := d.constructorMotionClassWithSnapshot("ConfinedRole", true); accepted {
		t.Fatal("caller-owned root acquired a fresh-snapshot role")
	}
}

func TestNativeConstructorConfinedCaptureOwnsResolverSnapshot(t *testing.T) {
	files := nativeCompileClasses(t, `final class SnapshotCaptureRoot{}class SnapshotCaptureHolder{final Object value;SnapshotCaptureHolder(Object value){this.value=value;}}`)
	root, err := Parse(bytes.Clone(files["SnapshotCaptureRoot.class"]))
	if err != nil {
		t.Fatal(err)
	}
	shared := bytes.Clone(files["SnapshotCaptureHolder.class"])
	d := NewClassObjectDumper(root)
	d.constructorReceiverFinalizerSilent = true
	d.foldSiblingResolver = func(name string) ([]byte, bool) { return shared, name == "SnapshotCaptureHolder" }
	snapshot, known := d.constructorMotionClassWithSnapshot("SnapshotCaptureHolder", true)
	if !known {
		t.Fatal("owned original snapshot missing")
	}
	code := func(object *ClassObject) *CodeAttribute {
		t.Helper()
		for _, method := range object.Methods {
			name, _ := sourceBridgeUTF8(object, method.NameIndex)
			if name != "<init>" {
				continue
			}
			for _, attr := range method.Attributes {
				if body, ok := attr.(*CodeAttribute); ok {
					return body
				}
			}
		}
		t.Fatal("constructor body missing")
		return nil
	}
	member := values.NewJavaClassMember("SnapshotCaptureHolder", "<init>", "(Ljava/lang/Object;)V", nil)
	remaining := 512
	if retained, accepted := d.constructorConfinedCapture(member, []constructorEffectValue{{kind: 'L', receiver: true}}, &remaining); !retained || !accepted {
		t.Fatal("original capture-copy body refused")
	}
	borrowed, err := Parse(shared)
	if err != nil {
		t.Fatal(err)
	}
	before := bytes.Clone(code(snapshot).Code)
	code(borrowed).Code[0] = core.OP_RETURN
	if !bytes.Equal(code(snapshot).Code, before) {
		t.Fatal("a later provider-buffer mutation changed the owned body")
	}
	remaining = 512
	if _, accepted := d.constructorConfinedCapture(member, []constructorEffectValue{{kind: 'L', receiver: true}}, &remaining); accepted {
		t.Fatal("changed original body reused an earlier capture certificate")
	}
}

func TestNativeConstructorConfinedCaptureRootMutation(t *testing.T) {
	const source = `class ConfinedMutationParent{Object retained;ConfinedMutationParent(){retained=new ConfinedMutationHolder(this);}}class ConfinedMutationHolder{final Object value;ConfinedMutationHolder(Object value){this.value=value;}}final class ConfinedMutationRoot extends ConfinedMutationParent{final Object value;ConfinedMutationRoot(Object value){this.value=value;}}`
	files := nativeCompileClasses(t, source)
	changed := nativeCompileClasses(t, strings.Replace(source, "final Object value;ConfinedMutationRoot", "protected void finalize(){ConfinedMutationSink.value=this;}final Object value;ConfinedMutationRoot", 1)+`class ConfinedMutationSink{static Object value;}`)
	object, known := jdkConstructorClassBytes("java/lang/Object", 8)
	if !known {
		t.Fatal("original Object evidence missing")
	}
	for _, mutate := range []bool{false, true} {
		t.Run(fmt.Sprintf("mutate=%v", mutate), func(t *testing.T) {
			root, err := Parse(bytes.Clone(files["ConfinedMutationRoot.class"]))
			if err != nil {
				t.Fatal(err)
			}
			other, err := Parse(bytes.Clone(changed["ConfinedMutationRoot.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(root)
			observed := false
			d.foldSiblingResolver = func(name string) ([]byte, bool) {
				if name == "java/lang/Object" {
					// Supply actual original bytes: do not rely on another runtime
					// profile to incidentally rediscover the changed finalizer.
					return bytes.Clone(object), true
				}
				if name == "ConfinedMutationHolder" {
					observed = true
					if mutate {
						*root = *other
					}
				}
				raw, ok := files[name+".class"]
				return bytes.Clone(raw), ok
			}
			writes := map[string]bool{"ConfinedMutationRoot\x00value\x00Ljava/lang/Object;": true}
			if accepted := d.constructorCaptureChainDoesNotObserve("ConfinedMutationParent", "()V", writes); accepted == mutate || !observed {
				t.Fatalf("accepted=%v mutation=%v holder observed=%v", accepted, mutate, observed)
			}
		})
	}
}

func TestNativeConstructorConfinedCaptureWideInputsAndProofLimits(t *testing.T) {
	files := nativeCompileClasses(t, `final class ConfinedWideRoot{}class ConfinedWideHolder{final long before;final Object retained;final double after;final Object independent;ConfinedWideHolder(long before,Object retained,double after,Object independent){this.before=before;this.retained=retained;this.after=after;this.independent=independent;}}`)
	root, err := Parse(bytes.Clone(files["ConfinedWideRoot.class"]))
	if err != nil {
		t.Fatal(err)
	}
	member := values.NewJavaClassMember("ConfinedWideHolder", "<init>", "(JLjava/lang/Object;DLjava/lang/Object;)V", nil)
	base := []constructorEffectValue{{kind: 'J'}, {kind: 'L', receiver: true}, {kind: 'D'}, {kind: 'L'}}
	rows := []struct {
		name               string
		edit               func([]constructorEffectValue)
		fuel               int
		retained, accepted bool
	}{
		{"exact receiver after category two word", nil, 512, true, true},
		{"retained carrier after category two word", func(v []constructorEffectValue) { v[1].receiver = false; v[1].retainsReceiver = true }, 512, true, true},
		{"independent inputs", func(v []constructorEffectValue) { v[1].receiver = false }, 512, false, true},
		{"receiver in later reference slot", func(v []constructorEffectValue) { v[1].receiver = false; v[3].receiver = true }, 512, true, true},
		{"wrong wide computational kind", func(v []constructorEffectValue) { v[0].kind = 'I' }, 512, false, false},
		{"uninitialized reference argument", func(v []constructorEffectValue) { v[1].allocation = 7 }, 512, false, false},
		{"primitive receiver reachability", func(v []constructorEffectValue) { v[0].receiver = true }, 512, false, false},
		{"contradictory exact and retained roles", func(v []constructorEffectValue) { v[1].retainsReceiver = true }, 512, false, false},
		{"no fuel", nil, 0, false, false},
		{"insufficient shared fuel", nil, 3, false, false},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			d := NewClassObjectDumper(root)
			d.constructorReceiverFinalizerSilent = true
			d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }
			args := append([]constructorEffectValue(nil), base...)
			if row.edit != nil {
				row.edit(args)
			}
			fuel := row.fuel
			retained, accepted := d.constructorConfinedCapture(member, args, &fuel)
			if retained != row.retained || accepted != row.accepted {
				t.Fatalf("retained=%v accepted=%v want %v/%v", retained, accepted, row.retained, row.accepted)
			}
		})
	}
}
