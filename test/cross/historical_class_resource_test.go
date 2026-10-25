package cross

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func testHistoricalClassResourceUsesActualReconstructionAndPacking(t *testing.T) {
	for _, owner := range []string{"StoredPayload", "RenamedStoredPayload"} {
		t.Run(owner, func(t *testing.T) {
			original := rebuiltCompileFixture(t, t.TempDir(), owner, `public class `+owner+` {public static long sum(long a,long b){return a*31+b;}}`, "8")
			resource := "hidden/loader-resource.raw"
			jar := filepath.Join(t.TempDir(), "original.jar")
			rebuiltFixtureJar(t, jar, map[string][]byte{resource: original})
			driverDir := t.TempDir()
			compileJava(t, driverDir, map[string]string{"ResourceAuditDriver.java": `import java.io.*;import java.lang.reflect.*;
public class ResourceAuditDriver extends ClassLoader {
 ResourceAuditDriver(){super(null);}Class<?> define(String name,byte[] b){return defineClass(name,b,0,b.length);}
 public static void main(String[]args)throws Exception{
  try{Class.forName(args[0]);throw new AssertionError("classpath leak");}catch(ClassNotFoundException expected){}
  InputStream in=ResourceAuditDriver.class.getClassLoader().getResourceAsStream(args[1]);if(in==null)throw new AssertionError("missing resource");
  ByteArrayOutputStream out=new ByteArrayOutputStream();byte[] b=new byte[1024];for(int n;(n=in.read(b))!=-1;)out.write(b,0,n);in.close();
  Method m=new ResourceAuditDriver().define(args[0],out.toByteArray()).getDeclaredMethod("sum",long.class,long.class);int rows=0;
  for(long a:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long c:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){
   long expected=java.math.BigInteger.valueOf(a).multiply(java.math.BigInteger.valueOf(31)).add(java.math.BigInteger.valueOf(c)).longValue();
   if(((Long)m.invoke(null,a,c)).longValue()!=expected)throw new AssertionError("arithmetic");rows++;}System.out.println(rows+":resource:actual-pipeline");}}
`})
			driverHash := sha256.Sum256(readClass(t, driverDir, "ResourceAuditDriver.class"))
			run := func(target string) ([]byte, error) {
				return exec.Command(lookJava(t), "-Xverify:all", "-cp", driverDir+string(os.PathListSeparator)+target, "ResourceAuditDriver", owner, resource).CombinedOutput()
			}
			const expected = "25:resource:actual-pipeline\n"
			if out, err := run(jar); err != nil || string(out) != expected {
				t.Fatal("original oracle", err, string(out))
			}
			sources, classes := t.TempDir(), t.TempDir()
			plan := historicalRenderAtCompilerTarget(t, jar, sources, 8, "reconstruction", nil)
			defer plan.close()
			if plan.units != 1 || plan.failures != 0 || !reflect.DeepEqual(plan.files, []string{filepath.Join(sources, owner+".java")}) {
				t.Fatal("physical resource source filename", plan.files, plan.units, plan.failures)
			}
			args := append([]string{"-proc:none", "--release", "8", "-g:none", "-cp", classes, "-sourcepath", sources, "-d", classes}, plan.files...)
			if log, err := exec.Command(lookJavac(t), args...).CombinedOutput(); err != nil {
				t.Fatal("fresh compiler", err, string(log))
			}
			report, err := historicalFinishArtifactOutputs("explicit-reconstruction-artifact-v1", plan.reconstruction.FinishOutput, classes, t.TempDir())
			if err != nil || len(report.Failures) != 0 || len(report.RestoredClasses) != 0 || !reflect.DeepEqual(report.RestoredResources, []string{resource}) {
				t.Fatal("resource completion inventory", report, err)
			}
			if _, err := os.Stat(filepath.Join(classes, owner+".class")); !os.IsNotExist(err) {
				t.Fatal("intermediate class leaked", err)
			}
			finished, err := os.ReadFile(filepath.Join(classes, resource))
			if err != nil || bytes.Equal(finished, original) {
				t.Fatal("original bytes reused", err)
			}
			if err := os.WriteFile(filepath.Join(classes, "noise.raw"), original, 0600); err != nil {
				t.Fatal(err)
			}
			packed := filepath.Join(t.TempDir(), "rebuilt.jar")
			zipRecompiledClassesAndResourcesToJar(t, classes, packed, jar, report.RestoredResources)
			z, err := zip.OpenReader(packed)
			if err != nil {
				t.Fatal(err)
			}
			if len(z.File) != 1 || z.File[0].Name != resource {
				t.Fatal("unapproved output or resource omission", z.File)
			}
			z.Close()
			if out, err := run(packed); err != nil || string(out) != expected {
				t.Fatal("fresh actual pipeline", err, string(out))
			}
			if sha256.Sum256(readClass(t, driverDir, "ResourceAuditDriver.class")) != driverHash {
				t.Fatal("caller changed")
			}
			if err := os.Remove(filepath.Join(classes, resource)); err != nil {
				t.Fatal(err)
			}
			if err := writeClassTreeJarWithResources(classes, packed, nil, report.RestoredResources); err == nil {
				t.Fatal("resource omission masked")
			}
		})
	}
}
