package javaclassparser

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/yaklang/javajive/internal/workbudget"
)

const sourceArtifactStorageFixture = `
class StorageFamily {
 private final Object token;StorageFamily(Object token){this.token=token;}
 static class Value implements java.io.Serializable {
  private static final long serialVersionUID=1L;
  private final Object token;private transient int memo;private transient volatile Object cached;private transient boolean seen;
  int hashCalls;int reads;
  Value(Object token){this.token=token;}
  int hash(int x,int salt){if(memo==0){hashCalls++;memo=x*31+salt;}return memo;}
  Object read(){if(!seen){reads++;cached=token;seen=true;}return cached;}
 }
 class Entry {private transient int memo;Object token(){memo++;return StorageFamily.this.token;}}
 Entry entry(){return new Entry();}
}
class StorageDriver {
 static Object copy(Object x)throws Exception {java.io.ByteArrayOutputStream b=new java.io.ByteArrayOutputStream();java.io.ObjectOutputStream o=new java.io.ObjectOutputStream(b);o.writeObject(x);o.close();java.io.ObjectInputStream i=new java.io.ObjectInputStream(new java.io.ByteArrayInputStream(b.toByteArray()));Object y=i.readObject();i.close();return y;}
 public static void main(String[]args)throws Exception {
  int rows=0;
  for(Object token:new Object[]{null,new String("same-token")})for(int x:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int salt:new int[]{0,17}){
   StorageFamily family=new StorageFamily(token);StorageFamily.Entry entry=family.entry();StorageFamily.Value v=new StorageFamily.Value(token);
   int expected=(int)((long)x*31L+(long)salt);if(v.hash(x,salt)!=expected)throw new AssertionError("independent wide overflow oracle");
   int next=expected==0?(int)((long)(x+1)*31L+(long)salt):expected;
   if(v.hash(x+1,salt)!=next||v.hashCalls!=(expected==0?2:1)||v.read()!=token||v.read()!=token||v.reads!=1||entry.token()!=token||entry.token()!=token)throw new AssertionError("cache state or null/alias");
   StorageFamily.Value restored=(StorageFamily.Value)copy(v);Object read=restored.read();
   if(restored.reads!=2||!(token==null?read==null:token.equals(read))||restored.hash(x,salt)!=expected)throw new AssertionError("transient storage state");
   if(entry.getClass().getDeclaringClass()!=StorageFamily.class||v.getClass().getDeclaringClass()!=StorageFamily.class)throw new AssertionError("lexical identity");
   for(Class c:new Class[]{StorageFamily.Value.class,StorageFamily.Entry.class})for(java.lang.reflect.Field f:c.getDeclaredFields())if(java.lang.reflect.Modifier.isPrivate(f.getModifiers())&&java.lang.reflect.Modifier.isTransient(f.getModifiers())&&!f.isSynthetic())throw new AssertionError("synthetic storage contract: "+f);
   rows++;
  }
  System.out.println(rows+":storage:cache:alias:overflow:serialization");
 }
}`

func sourceArtifactOriginalStorage(t *testing.T, plain map[string][]byte, root string) map[string][]byte {
	t.Helper()
	original := map[string][]byte{}
	changed := 0
	for name, raw := range plain {
		original[name] = bytes.Clone(raw)
		if !strings.HasPrefix(name, root+"$") {
			continue
		}
		object, err := Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		for _, field := range object.Fields {
			if field.AccessFlags&0x8a == 0x82 {
				field.AccessFlags |= syntheticFieldFlag
				changed++
			}
		}
		original[name] = object.Bytes()
	}
	if changed != 4 {
		t.Fatal("fixture storage field count", changed)
	}
	return original
}

func sourceArtifactWriteFiles(t *testing.T, files map[string][]byte) string {
	t.Helper()
	dir := t.TempDir()
	for name, raw := range files {
		file := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// The original binary caller supplies the algorithm and reflection oracle.
// Candidate javac sees no original owned class; restoration must preserve its
// executable output and recover all original field/capture/serialization ABI.
func TestNativeJarSourceArtifactStorageRetainsAlgorithmsAndMetadata(t *testing.T) {
	modern, java := t04Tools(t)
	for _, compiler := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		for _, root := range []string{"StorageFamily", "AlternateCacheOwner"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(string(compiler)+"/"+root+"/"+debug, func(t *testing.T) {
					fixture := strings.ReplaceAll(sourceArtifactStorageFixture, "StorageFamily", root)
					if root != "StorageFamily" {
						fixture = strings.NewReplacer("memo", "stateWord", "cached", "heldValue", "seen", "initialized").Replace(fixture)
					}
					javac := modern
					var plain map[string][]byte
					if compiler == NativeJavac8 {
						javac = os.Getenv("JAVA8_JAVAC")
						plain = nativePrivateEnumCompile(t, fixture, root, debug)
					} else {
						plain = nativeCompileSourceReleaseClasses(t, map[string]string{root + ".java": fixture}, debug, "8")
					}
					original := sourceArtifactOriginalStorage(t, plain, root)
					want := "20:storage:cache:alias:overflow:serialization\n"
					if got := t04RunJava(t, java, sourceArtifactWriteFiles(t, original), "StorageDriver"); got != want {
						t.Fatal("authored original oracle", got)
					}
					jar := filepath.Join(t.TempDir(), "original.jar")
					if err := os.WriteFile(jar, t23Zip(t, original), 0600); err != nil {
						t.Fatal(err)
					}
					for _, mode := range []string{"normal", "no-source-rewrites", "no-core-cleanups"} {
						t.Run(mode, func(t *testing.T) {
							if mode == "no-source-rewrites" {
								t.Setenv("JDEC_NO_SOURCE_REWRITES", "1")
							}
							if mode == "no-core-cleanups" {
								t.Setenv("JDEC_NO_CORE_CLEANUPS", "1")
							}
							artifact, err := NewJarSourceArtifactFromLocal(jar, 8, compiler, nil)
							if err != nil {
								t.Fatal(err)
							}
							defer artifact.Close()
							out := sourceArtifactWriteFiles(t, map[string][]byte{"StorageDriver.class": original["StorageDriver.class"]})
							var paths, owned []string
							for name := range original {
								if strings.HasPrefix(name, root) {
									owned = append(owned, name)
								}
							}
							sort.Strings(owned)
							for _, name := range owned {
								source, err := artifact.ReadFile(name)
								if err != nil || strings.Contains(string(source), DecompileStubMarker) || strings.Contains(string(source), "// decompile dump failed") {
									t.Fatalf("source %s: %v %s", name, err, source)
								}
								file := filepath.Join(out, strings.TrimSuffix(name, ".class")+".java")
								if err := os.WriteFile(file, source, 0600); err != nil {
									t.Fatal(err)
								}
								paths = append(paths, file)
							}
							args := []string{"-proc:none", "--release", "8"}
							if compiler == NativeJavac8 {
								args = []string{"-proc:none", "-source", "8", "-target", "8"}
							}
							args = append(args, "-g:"+debug, "-cp", out, "-sourcepath", out, "-d", out)
							if log, err := exec.Command(javac, append(args, paths...)...).CombinedOutput(); err != nil {
								t.Fatal("source compilation without original owned classes", err, string(log))
							}
							for _, name := range owned {
								compiled, err := os.ReadFile(filepath.Join(out, name))
								if err != nil {
									t.Fatal(err)
								}
								restored, err := artifact.RestoreClassMetadata(name, compiled)
								if err != nil || nativeBinaryShape(t, original[name]) != nativeBinaryShape(t, restored) || constructorInvocationThrowsDeclarations(t, original[name]) != constructorInvocationThrowsDeclarations(t, restored) {
									t.Fatalf("complete owned ABI %s: %v", name, err)
								}
								if err := os.WriteFile(filepath.Join(out, name), restored, 0600); err != nil {
									t.Fatal(err)
								}
							}
							caller, err := os.ReadFile(filepath.Join(out, "StorageDriver.class"))
							if err != nil || !bytes.Equal(caller, original["StorageDriver.class"]) {
								t.Fatal("original binary caller changed", err)
							}
							if got := t04RunJava(t, java, out, "StorageDriver"); got != want {
								t.Fatal("algorithm, reflective metadata or serialization changed", got)
							}
						})
					}
				})
			}
		}
	}
}

func TestNativeJarSourceArtifactDoesNotRelaxPlainSourceOrCaptureProofs(t *testing.T) {
	plain := nativeCompileClasses(t, sourceArtifactStorageFixture)
	original := sourceArtifactOriginalStorage(t, plain, "StorageFamily")
	root, err := Parse(original["StorageFamily.class"])
	if err != nil {
		t.Fatal(err)
	}
	z := nativeArchive(t, original)
	defer z.Close()
	if p := z.nativeMemberEntry(root); p != nil && p.planKnown {
		t.Fatal("plain-source ownership silently acquired a metadata obligation")
	}
	for _, variant := range []string{"original", "capture private", "capture removed", "capture duplicate", "capture descriptor", "storage static", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			files := make(map[string][]byte, len(original))
			for name, raw := range original {
				files[name] = bytes.Clone(raw)
			}
			child, err := Parse(files["StorageFamily$Entry.class"])
			if err != nil {
				t.Fatal(err)
			}
			capture := -1
			for i, field := range child.Fields {
				name, _ := sourceBridgeUTF8(child, field.NameIndex)
				if name == "this$0" {
					capture = i
				}
			}
			if capture < 0 {
				t.Fatal("fixture lacks the original enclosing capture")
			}
			switch variant {
			case "capture private":
				child.Fields[capture].AccessFlags |= 2
			case "capture removed":
				child.Fields = append(child.Fields[:capture], child.Fields[capture+1:]...)
			case "capture duplicate":
				child.Fields = append(child.Fields, child.Fields[capture])
			case "capture descriptor":
				for i, field := range child.Fields {
					if i != capture {
						child.Fields[capture].DescriptorIndex = field.DescriptorIndex
						break
					}
				}
			case "storage static":
				for i, field := range child.Fields {
					if i != capture {
						field.AccessFlags |= 8
					}
				}
			}
			files["StorageFamily$Entry.class"] = child.Bytes()
			view := nativeArchive(t, files)
			defer view.Close()
			view.sourceFieldMetadata = &sourceFieldMetadataAuthority{jar: view}
			d := view.nativeMemberReader(root)
			if variant == "budget" {
				d.Work = workbudget.New(context.Background(), Limits{MaxRequestWork: 1})
			}
			if variant == "canceled" {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, Limits{})
			}
			plan := d.planNativeMemberFamily()
			if (plan != nil) != (variant == "original") {
				t.Fatal("metadata obligation changed original capture/constructor admission", variant)
			}
		})
	}
}

func TestNativeJarSourceArtifactFieldMetadataBindsArchiveAndReleaseNamespace(t *testing.T) {
	base := nativeCompileClasses(t, sourceFieldMetadataFixture)["MetadataStorage.class"]
	files := map[string][]byte{
		"MetadataStorage.class": metadataStorageOriginal(t, base, "flag"),
		"META-INF/MANIFEST.MF":  []byte("Manifest-Version: 1.0\nMulti-Release: true\n\n"),
	}
	versionSource := strings.Replace(sourceFieldMetadataFixture, "int memo", "long memo", 1)
	versionSource = strings.Replace(versionSource, "return memo;", "return (int)memo;", 1)
	version := nativeCompileClasses(t, versionSource)["MetadataStorage.class"]
	files["META-INF/versions/9/MetadataStorage.class"] = metadataStorageOriginal(t, version, "flag")
	jar := filepath.Join(t.TempDir(), "metadata.jar")
	if err := os.WriteFile(jar, t23Zip(t, files), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := NewJarSourceArtifactFromLocal(jar, 8, ModernJavac, func(string) ([]byte, bool) { return base, true })
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for _, row := range []struct {
		name string
		raw  []byte
		ok   bool
	}{
		{"MetadataStorage.class", base, true},
		{"MetadataStorage.class", version, false},
		{"META-INF/versions/9/MetadataStorage.class", version, true},
		{"META-INF/versions/9/MetadataStorage.class", base, false},
		{"MetadataStorage.class", base, true},
		{"Unowned.class", base, false},
		{"../MetadataStorage.class", base, false},
		{"MetadataStorage.raw", base, false},
		{"META-INF/versions/09/MetadataStorage.class", version, false},
	} {
		result, err := a.RestoreClassMetadata(row.name, row.raw)
		if (err == nil) != row.ok || !row.ok && result != nil {
			t.Fatalf("namespace %s accepted=%v want=%v: %v", row.name, err == nil, row.ok, err)
		}
	}
	// A resolver result is a declaration, never an original output authority.
	unowned, _ := Parse(base)
	other, _ := Parse(files["MetadataStorage.class"])
	other.Fields[0].AccessFlags |= 8
	if fields := a.jar.sourceFieldMetadata.fields(unowned, nil); len(fields) != 0 {
		t.Fatal("a projected/unmarked model acquired original field permission")
	}
	if fields := a.jar.sourceFieldMetadata.fields(other, nil); len(fields) != 0 {
		t.Fatal("a mutated model acquired original field permission")
	}
}
