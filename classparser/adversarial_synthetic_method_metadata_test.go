package javaclassparser

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

const syntheticMethodMetadataFixture = `interface MethodFlagContract {long apply(long input);}
public class MethodFlagOwner implements MethodFlagContract {
 public static int calls;private final long base;
 public MethodFlagOwner(long base){this.base=base;}
 public static long mix(long left,long right){calls++;return (left*31L+right)^(left>>>17);}
 public long apply(long input){return mix(input,base);}
}
class MethodFlagDriver {public static void main(String[]args)throws Throwable {
 java.lang.reflect.Method mix=MethodFlagOwner.class.getDeclaredMethod("mix",long.class,long.class),apply=MethodFlagOwner.class.getDeclaredMethod("apply",long.class),contract=MethodFlagContract.class.getDeclaredMethod("apply",long.class);
 if(!mix.isSynthetic()||!apply.isSynthetic()||!contract.isSynthetic()||MethodFlagOwner.class.getDeclaredMethods().length!=2||MethodFlagContract.class.getDeclaredMethods().length!=1||MethodFlagOwner.class.getDeclaredConstructor(long.class).isSynthetic())throw new AssertionError("synthetic method universe");
 java.lang.invoke.MethodHandle handle=java.lang.invoke.MethodHandles.lookup().findStatic(MethodFlagOwner.class,"mix",java.lang.invoke.MethodType.methodType(long.class,long.class,long.class));int rows=0;
 for(long base:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long input:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
  MethodFlagContract value=new MethodFlagOwner(base);long expected=java.math.BigInteger.valueOf(input).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(base)).longValue()^(input>>>17);MethodFlagOwner.calls=0;
  if(value.apply(input)!=expected||MethodFlagOwner.calls!=1||(long)handle.invokeExact(input,base)!=expected||MethodFlagOwner.calls!=2||MethodFlagOwner.mix(input,base)!=expected||MethodFlagOwner.calls!=3)throw new AssertionError("interface/handle/static binding or overflow");rows++;
 }System.out.println(rows+":synthetic:interface:binding:wide:reflection");}}
`

func TestAdversarialSyntheticMethodMetadataKeepsOrdinaryBindingsAndFreshCode(t *testing.T) {
	compiler, java := t04Tools(t)
	for _, debug := range []string{"none", "source,lines,vars"} {
		t.Run(debug, func(t *testing.T) {
			files := compileLambdaParameterMetadataFixture(t, compiler, ModernJavac, syntheticMethodMetadataFixture, "MethodFlagOwner", debug, false)
			for _, name := range []string{"MethodFlagOwner.class", "MethodFlagContract.class"} {
				object := mustParseLowering(t, bytes.Clone(files[name]))
				for _, method := range object.Methods {
					n, _ := sourceBridgeUTF8(object, method.NameIndex)
					if n == "mix" || n == "apply" {
						method.AccessFlags |= syntheticMethodFlag
					}
				}
				files[name] = object.Bytes()
			}
			run := func(dir string, want bool) {
				t.Helper()
				for _, assertions := range []string{"-ea", "-da"} {
					log, err := exec.Command(java, assertions, "-Xverify:all", "-cp", dir, "MethodFlagDriver").CombinedOutput()
					if want && (err != nil || string(log) != "25:synthetic:interface:binding:wide:reflection\n") {
						t.Fatalf("independent original caller: %v\n%s", err, log)
					}
					if !want && (err == nil || !bytes.Contains(log, []byte("synthetic method universe"))) {
						t.Fatal("unrestored metadata negative control inactive", err, string(log))
					}
				}
			}
			run(sourceArtifactWriteFiles(t, files), true)
			owned := map[string][]byte{"MethodFlagOwner.class": files["MethodFlagOwner.class"], "MethodFlagContract.class": files["MethodFlagContract.class"]}
			jar := filepath.Join(t.TempDir(), "ordinary-synthetic.jar")
			if err := os.WriteFile(jar, t23Zip(t, owned), 0600); err != nil {
				t.Fatal(err)
			}
			a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			out := sourceArtifactWriteFiles(t, map[string][]byte{"MethodFlagDriver.class": files["MethodFlagDriver.class"]})
			paths := []string{}
			for _, name := range []string{"MethodFlagOwner", "MethodFlagContract"} {
				source, err := a.ReadFile(name + ".class")
				if err != nil || legacySourceHasFailureMarker(string(source)) {
					t.Fatalf("ordinary synthetic source %s: %v\n%s", name, err, source)
				}
				path := filepath.Join(out, name+".java")
				if err := os.WriteFile(path, source, 0600); err != nil {
					t.Fatal(err)
				}
				paths = append(paths, path)
			}
			if log, err := exec.Command(compiler, append([]string{"-proc:none", "--release", "8", "-g:" + debug, "-cp", out, "-sourcepath", out, "-d", out}, paths...)...).CombinedOutput(); err != nil {
				t.Fatal("owned originals absent from compiler classpath", err, string(log))
			}
			run(out, false)
			for name := range owned {
				fresh, err := os.ReadFile(filepath.Join(out, name))
				if err != nil {
					t.Fatal(err)
				}
				if got, err := a.source.RestoreClassMetadata(name, fresh); err == nil || got != nil {
					t.Fatal("ordinary metadata API was weakened instead of discharging the new transaction", name, err)
				}
				input := bytes.Clone(fresh)
				finished, err := a.FinishClass(name, fresh)
				if err != nil || !bytes.Equal(input, fresh) {
					t.Fatal("complete ordinary declaration reconstruction", name, err)
				}
				// A direct flag edit must leave every byte outside the method
				// access_flags unchanged, including all Code and opaque tables.
				reader, _ := NewClassReaderWithBudget(fresh, nil)
				layout := &compiledMetadataLayout{methods: map[*MemberInfo]compiledMetadataSpan{}}
				object, err := parseWithMetadataLayout(reader, nil, layout)
				if err != nil {
					t.Fatal(err)
				}
				want := bytes.Clone(fresh)
				for _, method := range object.Methods {
					n, _ := sourceBridgeUTF8(object, method.NameIndex)
					if n == "mix" || n == "apply" {
						offset := layout.methods[method].start
						want[offset] |= 0x10
					}
				}
				if !bytes.Equal(want, finished) {
					t.Fatal("ordinary synthetic restoration changed fresh executable or non-flag metadata bytes", name)
				}
				again, err := a.FinishClass(name, finished)
				if err != nil || !bytes.Equal(again, finished) {
					t.Fatal("ordinary synthetic completion not idempotent", err)
				}
				if err := os.WriteFile(filepath.Join(out, name), finished, 0600); err != nil {
					t.Fatal(err)
				}
			}
			run(out, true)
		})
	}
}
