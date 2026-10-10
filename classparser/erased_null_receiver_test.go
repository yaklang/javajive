package javaclassparser

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAdversarialErasedNullReceiverWithoutDependencyMetadataRoundTrip(t *testing.T) {
	t.Parallel()
	javac, java := t04Tools(t)
	const source = `interface NullCapture<T> {void accept(T value);}
class NullSink<T> implements NullCapture<T> {
  final StringBuilder trace=new StringBuilder();
  public void accept(T value) {trace.append("object;");}
  public void accept(String value) {trace.append("string;");}
}
public class ErasedNullReceiver<T> {
  final NullSink<? super T> sink;
  final NullCapture<? extends T> upper;
  int reads;
  boolean failReceiver;
  ErasedNullReceiver(NullSink<? super T> sink,NullCapture<? extends T> upper) {this.sink=sink;this.upper=upper;}
  NullSink<? super T> receiver() {reads++;if(failReceiver)throw new IllegalStateException("receiver");return sink;}
  void direct() {sink.accept((T)null);}
  void effectful() {receiver().accept((T)null);}
  void upper() {upper.accept(null);}
  void string() {sink.accept((String)null);}
  public static void main(String[] args) {
    NullSink<String> strings=new NullSink<>();
    ErasedNullReceiver<String> a=new ErasedNullReceiver<>(strings,strings);
    NullSink<Integer> numbers=new NullSink<>();
    ErasedNullReceiver<Integer> b=new ErasedNullReceiver<>(numbers,numbers);
    for(int i=0;i<3;i++) {a.direct();a.effectful();a.upper();a.string();b.direct();b.effectful();b.upper();b.string();}
    System.out.print(strings.trace+":"+a.reads+":"+numbers.trace+":"+b.reads);
    ErasedNullReceiver<String> missing=new ErasedNullReceiver<>(null,strings);
    try {missing.direct();}catch(NullPointerException ex){System.out.print(":direct-null");}
    try {missing.effectful();}catch(NullPointerException ex){System.out.print(":effect-null:"+missing.reads);}
    a.failReceiver=true;
    try {a.effectful();}catch(IllegalStateException ex){System.out.print(":"+ex.getMessage()+":"+a.reads);}
  }
}`
	for _, debug := range []string{"-g", "-g:none"} {
		original := t.TempDir()
		src := filepath.Join(original, "ErasedNullReceiver.java")
		if err := os.WriteFile(src, []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(javac, "-proc:none", "--release", "8", debug, "-d", original, src).CombinedOutput(); err != nil {
			t.Fatalf("original: %v\n%s", err, out)
		}
		want := t04RunJava(t, java, original, "ErasedNullReceiver")
		raw := readClassBytes(t, original, "ErasedNullReceiver")
		resolve := func(name string) ([]byte, bool) {
			b, err := os.ReadFile(filepath.Join(original, filepath.FromSlash(name)+".class"))
			return b, err == nil
		}
		compiled := map[string]string{}
		// Verify both isolated classfiles and available dependency metadata.
		for _, resolver := range []func(string) ([]byte, bool){nil, resolve} {
			for _, mode := range []DecompileMode{Precision, Compatibility, "legacy"} {
				var generated string
				var err error
				// The independent compiler and JVM always see the original helpers.
				if mode == "legacy" {
					generated, err = DecompileWithResolver(raw, resolver)
				} else {
					var result DecompileResult
					result, err = DecompileWithOptions(raw, DecompileOptions{Mode: mode, TargetSourceVersion: 8, Resolve: resolver})
					generated = result.Source
				}
				if err != nil {
					t.Fatal(err)
				}
				rebuilt, cached := compiled[generated]
				if !cached {
					rebuilt = t.TempDir()
					src := filepath.Join(rebuilt, "ErasedNullReceiver.java")
					if err := os.WriteFile(src, []byte(generated), 0644); err != nil {
						t.Fatal(err)
					}
					if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-cp", original, "-d", rebuilt, src).CombinedOutput(); err != nil {
						t.Fatalf("rebuild %s/%s: %v\n%s\n%s", mode, debug, err, out, generated)
					}
					compiled[generated] = rebuilt
				}
				if got := t04RunJava(t, java, rebuilt+string(os.PathListSeparator)+original, "ErasedNullReceiver"); got != want {
					t.Fatalf("%s/%s got=%q want=%q\n%s", mode, debug, got, want, generated)
				}
			}
		}
	}
}
