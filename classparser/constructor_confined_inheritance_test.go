package javaclassparser

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestAdversarialConstructorInheritedConfinedStorageHasExactRoles(t *testing.T) {
	rows := []struct {
		name, base, holder string
		want               bool
	}{

		{"private independent identity", "class $B{final Object identity=new Object();final boolean top;$B(Object p){top=p==null;}}", "class $H extends $B{final Object reader;$H(Object v){super(null);reader=v;}}", true},
		{"type guard and original abrupt branch", "class $B{final Object identity=new Object();$B(Object p){if(!(this instanceof $H))throw new Error(\"foreign\");}}", "class $H extends $B{final Object reader;$H(Object v){super(null);reader=v;}}", true},
		{"new carrier callback", "class $B{$B(Object v){new $Box(v);}}class $Box{$Box(Object v){v.toString();}}", "class $H extends $B{$H(Object v){super(v);}}", false},
		{"thrown carrier publication", "class $B{$B(Object v){if(v!=null)throw new $Box(v);}}class $Box extends RuntimeException{final Object ref;$Box(Object v){ref=v;}}", "class $H extends $B{$H(Object v){super(v);}}", false},
		{"fresh wrapper factory", "class $B{final boolean top;$B(Object p){top=p==null;}}", "class $H extends $B{final Object reader,wrapper;$H(Object v){super(null);reader=v;wrapper=top?$F.wrap(this):null;}}class $F{static Object wrap(Object v){return new $Box(v);}}class $Box{final Object ref;$Box(Object v){ref=v;}}", true},
		{"unused factory argument", "class $B{final boolean top;$B(Object p){top=p==null;}}", "class $H extends $B{final Object reader,wrapper;$H(Object v){super(null);reader=v;wrapper=top?$F.wrap(this):null;}}class $F{static Object wrap(Object ignored){return new $Box(null);}}class $Box{final Object ref;$Box(Object v){ref=v;}}", true},
		{"factory publishes input", "class $B{final boolean top;$B(Object p){top=p==null;}}", "class $H extends $B{final Object reader,wrapper;$H(Object v){super(null);reader=v;wrapper=top?$F.wrap(this):null;}}class $F{static Object wrap(Object v){$P.send(v);return new $Box(v);}}class $Box{final Object ref;$Box(Object v){ref=v;}}", false},
		{"factory publishes result", "class $B{final boolean top;$B(Object p){top=p==null;}}", "class $H extends $B{final Object reader,wrapper;$H(Object v){super(null);reader=v;wrapper=top?$F.wrap(this):null;}}class $F{static Object wrap(Object v){$Box b=new $Box(v);$P.send(b);return b;}}class $Box{final Object ref;$Box(Object v){ref=v;}}", false},
		{"factory returns input", "class $B{final boolean top;$B(Object p){top=p==null;}}", "class $H extends $B{final Object reader,wrapper;$H(Object v){super(null);reader=v;wrapper=top?$F.wrap(this):null;}}class $F{static Object wrap(Object v){return v;}}", false},
		{"factory constructor callback", "class $B{final boolean top;$B(Object p){top=p==null;}}", "class $H extends $B{final Object reader,wrapper;$H(Object v){super(null);reader=v;wrapper=top?$F.wrap(this):null;}}class $F{static Object wrap(Object v){return new $Box(v);}}class $Box{final Object ref;$Box(Object v){ref=v;v.toString();}}", false},
		{"factory holder finalizer", "class $B{final boolean top;$B(Object p){top=p==null;}}", "class $H extends $B{final Object reader,wrapper;$H(Object v){super(null);reader=v;wrapper=top?$F.wrap(this):null;}}class $F{static Object wrap(Object v){return new $Box(v);}}class $Box{final Object ref;$Box(Object v){ref=v;}protected void finalize(){$P.send(ref);}}", false},
		{"factory synchronized monitor", "class $B{final boolean top;$B(Object p){top=p==null;}}", "class $H extends $B{final Object reader,wrapper;$H(Object v){super(null);reader=v;wrapper=top?$F.wrap(this):null;}}class $F{static synchronized Object wrap(Object v){return new $Box(v);}}class $Box{final Object ref;$Box(Object v){ref=v;}}", false},
		{"parent copies input", "class $B{final Object ref;$B(Object v){ref=v;}}", "class $H extends $B{$H(Object v){super(v);}}", true},
		{"abstract parent and conditional flag", "abstract class $B{final Object parent;final boolean top;final int ordinal;$B(Object p,int n){parent=p;top=p==null;ordinal=n;}}", "class $H extends $B{final Object reader;$H(Object v){super(null,0);reader=v;}}", true},
		{"parent ignores input", "class $B{$B(Object ignored){}}", "class $H extends $B{$H(Object v){super(v);}}", true},
		{"constructor overload chain", "class $B{final Object ref;$B(Object v){ref=v;}}", "class $H extends $B{$H(Object v){this(0,v);}private $H(int n,Object v){super(v);}}", true},
		{"both branch storage", "class $B{final Object ref;$B(Object v){if(v==null)ref=null;else ref=v;}}", "class $H extends $B{$H(Object v){super(v);}}", true},
		{"two inherited levels", "class $B{final Object ref;$B(Object v){ref=v;}}class $Mid extends $B{$Mid(Object v){super(v);}}", "class $H extends $Mid{$H(Object v){super(v);}}", true},
		{"parent publishes holder", "class $B{$B(Object v){$P.send(this);}}", "class $H extends $B{$H(Object v){super(v);}}", false},
		{"parent publishes input", "class $B{$B(Object v){$P.send(v);}}", "class $H extends $B{$H(Object v){super(v);}}", false},
		{"conditional publication", "class $B{$B(Object v){if(v!=null)$P.send(v);}}", "class $H extends $B{$H(Object v){super(v);}}", false},
		{"parent callback on holder", "class $B{$B(Object v){touch();}void touch(){}}", "class $H extends $B{$H(Object v){super(v);}void touch(){$P.send(this);}}", false},
		{"parent callback on input", "class $B{$B(Object v){v.toString();}}", "class $H extends $B{$H(Object v){super(v);}}", false},
		{"inherited finalizer", "class $B{Object ref;$B(Object v){ref=v;}protected void finalize(){$P.send(ref);}}", "class $H extends $B{$H(Object v){super(v);}}", false},
		{"volatile parent field", "class $B{volatile Object ref;$B(Object v){ref=v;}}", "class $H extends $B{$H(Object v){super(v);}}", false},
		{"read retained input", "class $B{Object ref;$B(Object v){ref=v;$P.send(ref);}}", "class $H extends $B{$H(Object v){super(v);}}", false},
		{"parent constructor handler", "class $B{$B(Object v){try{$P.send(v);}catch(RuntimeException ignored){}}}", "class $H extends $B{$H(Object v){super(v);}}", false},
		{"cyclic parent protocol", "class $B{$B(Object v){for(int i=0;i<2;i++){} }}", "class $H extends $B{$H(Object v){super(v);}}", false},
	}
	var source strings.Builder
	for i, row := range rows {
		prefix := fmt.Sprintf("InheritedCopy%d", i)
		rename := strings.NewReplacer("$P", prefix+"Parent", "$C", prefix+"Child", "$H", prefix+"Holder", "$Box", prefix+"Box", "$F", prefix+"Factory", "$B", prefix+"Base", "$Mid", prefix+"Middle")
		source.WriteString(rename.Replace(`class $P{Object retained;static Object escaped;static void send(Object v){escaped=v;}$P(){retained=new $H(this);}}final class $C extends $P{final Object capture;$C(Object v){super();capture=v;}}` + row.base + row.holder))
	}
	files := nativeCompileDebugClasses(t, source.String(), "none")
	for i, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			prefix := fmt.Sprintf("InheritedCopy%d", i)
			root, err := Parse(bytes.Clone(files[prefix+"Child.class"]))
			if err != nil {
				t.Fatal(err)
			}
			d := NewClassObjectDumper(root)
			d.foldSiblingResolver = func(name string) ([]byte, bool) { raw, ok := files[name+".class"]; return bytes.Clone(raw), ok }
			if got := d.constructorCaptureChainDoesNotObserve(prefix+"Parent", "()V", map[string]bool{prefix + "Child\x00capture\x00Ljava/lang/Object;": true}); got != row.want {
				t.Fatalf("inherited holder confinement=%v, want=%v", got, row.want)
			}
		})
	}
}
