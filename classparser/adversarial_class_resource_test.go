package javaclassparser

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A resource is a byte supplier for an explicit defining loader, not a class
// path alias. The caller is compiled without the payload and remains unchanged.
const classResourceDriver = `import java.io.*;import java.lang.reflect.*;
public class ResourceDriver {
 static class Definer extends ClassLoader {Definer(){super(null);}Class<?> loadPayload(String name,String resource)throws Exception{
  InputStream in=ResourceDriver.class.getClassLoader().getResourceAsStream(resource);if(in==null)throw new AssertionError("missing resource");
  ByteArrayOutputStream out=new ByteArrayOutputStream();byte[] b=new byte[256];for(int n;(n=in.read(b))!=-1;)out.write(b,0,n);in.close();byte[] raw=out.toByteArray();return defineClass(name,raw,0,raw.length);}}
 public static void main(String[]args)throws Exception{
  String name=args[0],resource=args[1];try{Class.forName(name,false,ResourceDriver.class.getClassLoader());throw new AssertionError("resource leaked onto classpath");}catch(ClassNotFoundException expected){}
  System.setProperty("resource.counter","3");int rows=0;
  for(int load=0;load<2;load++){Class<?> c=new Definer().loadPayload(name,resource);if(!System.getProperty("resource.counter").equals(""+(3+load)))throw new AssertionError("eager initialization");
   Method m=c.getDeclaredMethod("combine",long.class,long.class);Parameter[] p=m.getParameters();
   if(!p[0].isNamePresent()||!p[0].getName().equals("left")||!p[1].getName().equals("right")||p[0].getModifiers()!=16||p[1].getModifiers()!=0)throw new AssertionError("metadata");
   for(long a:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long b:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
    long expected=java.math.BigInteger.valueOf(a).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(b)).longValue();
    if(((Long)m.invoke(null,a,b)).longValue()!=expected)throw new AssertionError("wide arithmetic");rows++;}
   if(!System.getProperty("resource.counter").equals(""+(4+load)))throw new AssertionError("initialization multiplicity");
  }System.out.println(rows+":resource:loader:initialization:metadata:overflow");}}
`

type classResourceCompletion interface {
	ReadClassResource(string) ([]byte, string, error)
	FinishClassResource(string, []byte) ([]byte, error)
	FinishOutput(string, []byte) (string, []byte, error)
}

func TestAdversarialClassResourcesKeepDefiningLoaderAndPhysicalPublication(t *testing.T) {
	modern, java := t04Tools(t)
	native := os.Getenv("JAVA8_JAVAC")
	if native == "" {
		t.Fatal("native original compiler required")
	}
	driver := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, classResourceDriver, "ResourceDriver", "none", false)
	driverDir := t.TempDir()
	for name, raw := range driver {
		if err := os.WriteFile(filepath.Join(driverDir, name), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, owner := range []string{"ResourcePayload", "RenamedResourcePayload"} {
		for _, extension := range []string{".raw", ".payload"} {
			for _, debug := range []string{"none", "source,lines,vars"} {
				t.Run(owner+extension+"/"+debug, func(t *testing.T) {
					fixture := fmt.Sprintf(`public class %s {static {System.setProperty("resource.counter", ""+(Integer.parseInt(System.getProperty("resource.counter"))+1));}public static long combine(final long left,long right){return left*31L+right;}}`, owner)
					files := compileLambdaParameterMetadataFixture(t, native, NativeJavac8, fixture, owner, debug, true)
					resource := "hidden/" + owner + extension
					original := files[owner+".class"]
					jar := filepath.Join(t.TempDir(), "original.jar")
					if err := os.WriteFile(jar, t23Zip(t, map[string][]byte{resource: original}), 0600); err != nil {
						t.Fatal(err)
					}
					run := func(target string) ([]byte, error) {
						return exec.Command(java, "-Xverify:all", "-cp", driverDir+string(os.PathListSeparator)+target, "ResourceDriver", owner, resource).CombinedOutput()
					}
					const expected = "50:resource:loader:initialization:metadata:overflow\n"
					if log, err := run(jar); err != nil || string(log) != expected {
						t.Fatal("independent original", err, string(log))
					}
					a, err := NewJarReconstructionArtifactFromLocal(jar, 8, ModernJavac, nil)
					if err != nil {
						t.Fatal(err)
					}
					defer a.Close()
					api, ok := interface{}(a).(classResourceCompletion)
					if !ok {
						t.Fatal("physical resource completion is unavailable")
					}
					source, binaryName, err := api.ReadClassResource(resource)
					if err != nil || binaryName != owner || legacySourceHasFailureMarker(string(source)) {
						t.Fatal("resource source", binaryName, err, string(source))
					}
					fresh := compileLambdaParameterMetadataFixture(t, modern, ModernJavac, string(source), owner, debug, false)[owner+".class"]
					before := sha256.Sum256(fresh)
					finished, err := api.FinishClassResource(resource, fresh)
					if err != nil {
						t.Fatal("resource completion", err)
					}
					published, routed, err := api.FinishOutput(owner+".class", fresh)
					if err != nil || published != resource || !bytes.Equal(routed, finished) {
						t.Fatal("physical output route", published, err)
					}
					if sha256.Sum256(fresh) != before || bytes.Equal(original, finished) {
						t.Fatal("mutated input or reused original class")
					}
					if data, err := a.FinishClass(owner+".class", fresh); err == nil || data != nil {
						t.Fatal("resource became ordinary class authority")
					}
					packed := filepath.Join(t.TempDir(), "rebuilt.jar")
					write := func(entries map[string][]byte) {
						if err := os.WriteFile(packed, t23Zip(t, entries), 0600); err != nil {
							t.Fatal(err)
						}
					}
					write(map[string][]byte{resource: finished})
					if log, err := run(packed); err != nil || string(log) != expected {
						t.Fatal("fresh resource behavior", err, string(log))
					}
					write(map[string][]byte{owner + ".class": finished})
					if log, err := run(packed); err == nil || !strings.Contains(string(log), "resource leaked onto classpath") {
						t.Fatal("publication negative control", err, string(log))
					}
					write(map[string][]byte{})
					if log, err := run(packed); err == nil || !strings.Contains(string(log), "missing resource") {
						t.Fatal("omission negative control", err, string(log))
					}
				})
			}
		}
	}
}
