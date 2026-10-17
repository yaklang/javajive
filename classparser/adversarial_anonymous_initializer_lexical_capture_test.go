package javaclassparser

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"github.com/yaklang/javajive/internal/workbudget"
)

const lexicalInitializerCaptureFixture = `import java.util.*;class LexicalInitializerOwner{static<T>Iterable<T> produce(final Iterable<T> input){return new Iterable<T>(){public Iterator<T> iterator(){return new Iterator<T>(){final Iterator<T> backing=input.iterator();public boolean hasNext(){return backing.hasNext();}public T next(){return backing.next();}public void remove(){throw new UnsupportedOperationException();}};}};}}
class LexicalInitializerInput<T> implements Iterable<T>{final List<T> data;int producers;boolean fail;final RuntimeException failure=new RuntimeException("identity");LexicalInitializerInput(List<T> data){this.data=data;}public Iterator<T> iterator(){producers++;if(fail)throw failure;return data.iterator();}}
class LexicalInitializerDriver{public static void main(String[]args){int rows=0;for(String word:new String[]{null,"",new String("word"),new String(new char[]{0}),new String(new char[]{(char)0xd800}),new String(new char[]{(char)0xd83d,(char)0xde00})}){List<String> data=Arrays.asList(word,new String("tail"));LexicalInitializerInput<String> input=new LexicalInitializerInput<String>(data);Iterable<String> result=LexicalInitializerOwner.produce(input);if(input.producers!=0)throw new AssertionError("early producer");Iterator<String> cursor=result.iterator();if(input.producers!=1)throw new AssertionError("producer count");for(String expected:data){if(!cursor.hasNext()||cursor.next()!=expected)throw new AssertionError("identity order");}if(cursor.hasNext())throw new AssertionError("exhaustion");try{cursor.next();throw new AssertionError("missing exhaustion");}catch(NoSuchElementException expected){}try{cursor.remove();throw new AssertionError("mutation");}catch(UnsupportedOperationException expected){}if(!result.getClass().isAnonymousClass()||!cursor.getClass().isAnonymousClass()||!cursor.getClass().getEnclosingMethod().getName().equals("iterator"))throw new AssertionError("scope");input.fail=true;try{result.iterator();throw new AssertionError("missing producer failure");}catch(RuntimeException expected){if(expected!=input.failure||input.producers!=2)throw new AssertionError("producer failure identity");}rows++;}System.out.println(rows+":lexical:initializer:producer:identity:scope");}}`

func lexicalInitializerCaptureShape(shape string) (string, string) {
	owner, fixture := "LexicalInitializerOwner", lexicalInitializerCaptureFixture
	if shape == "array-elements" {
		fixture = strings.ReplaceAll(fixture, "List<String>", "List<String[]>")
		fixture = strings.ReplaceAll(fixture, "Input<String>", "Input<String[]>")
		fixture = strings.ReplaceAll(fixture, "Iterable<String>", "Iterable<String[]>")
		fixture = strings.ReplaceAll(fixture, "Iterator<String>", "Iterator<String[]>")
		fixture = strings.ReplaceAll(fixture, `Arrays.asList(word,new String("tail"))`, `Arrays.<String[]>asList(new String[]{word},new String[]{new String("tail")})`)
		fixture = strings.ReplaceAll(fixture, "for(String expected:data)", "for(String[] expected:data)")
	}
	if shape == "renamed" {
		owner = "LexicalSnapshotOwner"
		fixture = strings.ReplaceAll(fixture, "LexicalInitializerOwner", owner)
		fixture = strings.ReplaceAll(fixture, "backing", "snapshotCursor")
	}
	return owner, fixture
}

func TestNativeAnonymousInitializerLexicalCaptureRequiresOriginalPath(t *testing.T) {
	files := nativeCompileClasses(t, nativeAnonymousNestedFixture)
	variants := []string{"original", "missing witness", "foreign witness", "missing PC", "changed PC", "changed inner PC", "wrong name", "wrong type", "foreign receiver", "foreign receiver type", "mutable alias", "opaque", "cycle", "missing binding", "invalid binding", "missing object", "copied object", "copied certificate", "wrong scope", "missing renderer", "missing query", "budget", "canceled"}
	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			z := nativeArchive(t, files)
			defer z.Close()
			root, err := Parse(files["NestedOwner.class"])
			if err != nil {
				t.Fatal(err)
			}
			plan := z.nativeMemberReader(root).planNativeAnonymousForest()
			if plan == nil || plan.forest == nil {
				t.Fatal("original complete family")
			}
			forest := plan.forest
			object := forest.units["NestedOwner$1$1"].object
			key := "origin()Ljava/lang/Object;"
			var read *nativeMemberLexicalRead
			for _, entry := range forest.reads[object.GetClassName()][key] {
				if entry.prior != nil {
					read = entry
				}
			}
			if read == nil {
				t.Fatal("original two-hop lexical path")
			}
			ctx := &class_context.ClassContext{ClassName: object.GetClassName(), FunctionName: "origin", CurrentMethodDesc: "()Ljava/lang/Object;"}
			ref := values.NewJavaRef(&utils.VariableId{}, nil, types.NewJavaClass(object.GetClassName()))
			ref.IsThis = true
			var path []*nativeMemberLexicalRead
			for node := read; node != nil; node = node.prior {
				path = append(path, node)
			}
			var value values.JavaValue = ref
			for i := len(path) - 1; i >= 0; i-- {
				node := path[i]
				field := values.NewRefMember(value, node.field, types.NewJavaClass(node.descriptor[1:len(node.descriptor)-1]))
				field.HasOriginPC, field.OriginPC = true, node.pc
				value = field
			}
			field := value.(*values.RefMember)
			original := &values.JavaClassMember{Name: read.owner, Member: read.field, Description: read.descriptor}
			witness := *original
			if variant == "foreign witness" {
				witness.Description = "Ljava/lang/String;"
			}
			if variant != "missing witness" {
				field.MarkOriginalFieldRead(&witness, read.pc)
			}
			d := NewClassObjectDumper(object)
			d.nativeAnonymousForest = forest
			d.nativeAnonymousBindings = map[string]string{nativeMemberCaptureIndexKey(read.owner, read.field): "token"}
			switch variant {
			case "missing PC":
				field.HasOriginPC = false
			case "changed PC":
				field.OriginPC++
			case "changed inner PC":
				field.Object.(*values.RefMember).OriginPC++
			case "wrong name":
				field.Member = "other"
			case "wrong type":
				field.JavaType = types.NewJavaClass("java.lang.String")
			case "foreign receiver":
				ref.IsThis = false
			case "foreign receiver type":
				ref.ResetVarType(types.NewJavaClass("Foreign"))
			case "mutable alias":
				field.Object = values.NewJavaRef(&utils.VariableId{}, field.Object, field.Object.Type())
			case "opaque":
				operand := field.Object
				field.Object = values.NewCustomValue(func(*class_context.ClassContext) string { t.Fatal("query rendered opaque source"); return "" }, func() types.JavaType { return operand.Type() })
			case "cycle":
				field.Object.(*values.RefMember).Object = field
			case "missing binding":
				delete(d.nativeAnonymousBindings, nativeMemberCaptureIndexKey(read.owner, read.field))
			case "invalid binding":
				d.nativeAnonymousBindings[nativeMemberCaptureIndexKey(read.owner, read.field)] = "token.iterator()"
			case "missing object":
				delete(forest.objects, object.GetClassName())
			case "copied object":
				copy := *object
				d.obj = &copy
			case "copied certificate":
				copy := *read
				read = &copy
			case "wrong scope":
				ctx.CurrentMethodDesc = "()I"
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				cx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(cx, workbudget.Limits{})
			}
			d.wireNativeAnonymousForestCaptures(ctx)
			// Querying does not render source or latch a committed failure. The
			// initializer additionally requires the immutable GETFIELD witness.
			if variant == "missing renderer" {
				ctx.SourceLexicalCapturedField = nil
			}
			if variant == "missing query" {
				ctx.SourceLexicalCapturedFieldType = nil
			}
			if variant == "copied certificate" {
				if _, ok := d.nativeAnonymousLexicalCaptureReadSource(read, field, field.OriginPC, field.Member, ctx); ok {
					t.Fatal("detached certificate accepted")
				}
				return
			}
			if got := nativeAnonymousInitializerLexicalCaptureField(original, field, ctx); got != (variant == "original") {
				t.Fatalf("initializer certificate=%v", got)
			}
			if d.nativeCaptureFailed {
				t.Fatal("read-only query committed failure state")
			}
		})
	}
}
func TestAdversarialAnonymousInitializerLexicalCapture(t *testing.T) {
	for _, shape := range []string{"reference", "array-elements", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			owner, fixture := lexicalInitializerCaptureShape(shape)
			testNativePrivateSetterCompiledFixture(t, owner, "LexicalInitializerDriver", "6:lexical:initializer:producer:identity:scope\n", func(t *testing.T, debug string) map[string][]byte {
				return nativeCompileSourceReleaseClasses(t, map[string]string{owner + ".java": fixture}, debug, "8")
			}, nativeLexicalExactSignatures)
		})
	}
}
func TestAdversarialAnonymousInitializerLexicalCaptureJavac8(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("JAVA8_JAVAC required for actual compiler")
	}
	for _, shape := range []string{"reference", "array-elements", "renamed"} {
		t.Run(shape, func(t *testing.T) {
			owner, fixture := lexicalInitializerCaptureShape(shape)
			testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte { return nativePrivateEnumCompile(t, fixture, owner, debug) }, NativeJavac8, javac, []string{owner}, "LexicalInitializerDriver", "6:lexical:initializer:producer:identity:scope\n", nil, nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialAnonymousInitializerLexicalCaptureOracleRejectsEffectMutants(t *testing.T) {
	_, java := t04Tools(t)
	original := nativeCompileSourceReleaseClasses(t, map[string]string{"LexicalInitializerOwner.java": lexicalInitializerCaptureFixture}, "none", "8")
	mutations := []struct{ name, before, after string }{
		{"original", "", ""},
		{"duplicate-producer", "final Iterator<T> backing=input.iterator();", "final Iterator<T> backing=(input.iterator()==null?input.iterator():input.iterator());"},
		{"consume-two-elements", "public T next(){return backing.next();}", "public T next(){backing.next();return backing.next();}"},
		{"accepted-mutation", "public void remove(){throw new UnsupportedOperationException();}", "public void remove(){}"},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			files := original
			if mutation.name != "original" {
				if strings.Count(lexicalInitializerCaptureFixture, mutation.before) != 1 {
					t.Fatal("ambiguous mutation")
				}
				fixture := strings.Replace(lexicalInitializerCaptureFixture, mutation.before, mutation.after, 1)
				files = nativeCompileSourceReleaseClasses(t, map[string]string{"LexicalInitializerOwner.java": fixture}, "none", "8")
			}
			out := t.TempDir()
			for name, raw := range original {
				if strings.HasPrefix(name, "LexicalInitializerOwner") {
					var exists bool
					raw, exists = files[name]
					if !exists {
						t.Fatalf("mutant lost target %s", name)
					}
				}
				if err := os.WriteFile(filepath.Join(out, name), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, assertions := range []string{"-ea", "-da"} {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				output, err := exec.CommandContext(ctx, java, "-Xverify:all", assertions, "-cp", out, "LexicalInitializerDriver").CombinedOutput()
				interrupted := ctx.Err() != nil
				cancel()
				if mutation.name == "original" {
					if err != nil || string(output) != "6:lexical:initializer:producer:identity:scope\n" {
						t.Fatalf("original %s: %v %s", assertions, err, output)
					}
				} else if interrupted || err == nil || !strings.Contains(string(output), "java.lang.AssertionError") || strings.Contains(string(output), "VerifyError") || strings.Contains(string(output), "NoClassDefFoundError") {
					t.Fatalf("mutant %s did not fail an independent semantic assertion: %v %s", assertions, err, output)
				}
			}
		})
	}
}
