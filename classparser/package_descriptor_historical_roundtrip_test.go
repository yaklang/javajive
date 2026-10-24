package javaclassparser

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Compile the package descriptor explicitly, including a descriptor with no
// annotations. Its class existence and package annotations are the public
// contract; an unchanged binary caller supplies the independent oracle.
func TestAdversarialHistoricalPackageDescriptorPreservesNestedAnnotations(t *testing.T) {
	modern, java := t04Tools(t)
	for _, profile := range []SourceCompilerProfile{ModernJavac, NativeJavac8} {
		compiler := modern
		if profile == NativeJavac8 {
			compiler = os.Getenv("JAVA8_JAVAC")
			if compiler == "" {
				t.Skip("JAVA8_JAVAC required")
			}
		}
		for _, declaration := range []struct{ major, flags uint16 }{{49, 0x0200}, {49, 0x0600}, {52, 0x1600}} {
			for _, pkg := range []string{"packet", "alternate.location"} {
				for _, annotated := range []bool{true} {
					for _, debug := range []string{"none", "source,lines,vars"} {
						t.Run(fmt.Sprintf("%s/%s/flags=%x/%s", profile, pkg, declaration.flags, debug), func(t *testing.T) {
							base := strings.ReplaceAll(pkg, ".", "/") + "/"
							annotation := ""
							if annotated {
								annotation = `@Labels.Label(text="a\u0000b\ud83d\ude03", words={-2147483648,0,2147483647}, shape=String[][].class, kind=java.lang.annotation.ElementType.PACKAGE) @Hidden(19)`
							}
							units := map[string][]byte{
								base + "package-info.java": []byte(annotation + "\npackage " + pkg + ";\n"),
								base + "Labels.java":       []byte("package " + pkg + `;import java.lang.annotation.*;public class Labels{@Retention(RetentionPolicy.RUNTIME)@Target(ElementType.PACKAGE)public @interface Label{String text();int[] words();Class<?> shape();ElementType kind();}}`),
								base + "Hidden.java":       []byte("package " + pkg + `;import java.lang.annotation.*;@Retention(RetentionPolicy.CLASS)@Target(ElementType.PACKAGE)public @interface Hidden{int value();}`),
								base + "Marker.java":       []byte("package " + pkg + ";public class Marker{}"),
								"DescriptorDriver.java":    []byte(fmt.Sprintf(`import %s.*;public class DescriptorDriver{public static void main(String[]args)throws Exception{Class<?> descriptor=Class.forName("%s.package-info",false,Marker.class.getClassLoader());if(descriptor.getDeclaredMethods().length!=0||descriptor.getDeclaredFields().length!=0||descriptor.getInterfaces().length!=0)throw new AssertionError("invented descriptor members");Package p=Marker.class.getPackage();Labels.Label a=p.getAnnotation(Labels.Label.class);if(%t){if(a==null||!a.text().equals("a\u0000b\ud83d\ude03")||!java.util.Arrays.equals(a.words(),new int[]{-2147483648,0,2147483647})||a.shape()!=String[][].class||a.kind()!=java.lang.annotation.ElementType.PACKAGE||p.getAnnotations().length!=1)throw new AssertionError("package metadata");int[] copy=a.words();copy[0]=1;if(a.words()[0]!=Integer.MIN_VALUE)throw new AssertionError("annotation array identity");}else if(a!=null||p.getAnnotations().length!=0)throw new AssertionError("invented package annotation");if(descriptor.getModifiers()!=%d||descriptor.isSynthetic()!=%t)throw new AssertionError("declaration flags:"+descriptor.getModifiers());System.out.println("descriptor:identity:annotations:retention");}}`, pkg, pkg, annotated, declaration.flags|0x0400, declaration.flags&0x1000 != 0)),
							}
							originalDir := sourceArtifactWriteFiles(t, units)
							args := []string{"-proc:none", "-Xpkginfo:always", "-g:" + debug, "-d", originalDir}
							if profile == ModernJavac {
								args = append(args, "--release", "8")
							}
							for name := range units {
								args = append(args, filepath.Join(originalDir, name))
							}
							if log, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
								t.Fatalf("original compile: %v\n%s", err, log)
							}
							descriptorPath := filepath.Join(originalDir, base+"package-info.class")
							originalBytes, err := os.ReadFile(descriptorPath)
							if err != nil {
								t.Fatal(err)
							}
							originalDescriptor := mustParseLowering(t, originalBytes)
							originalDescriptor.MajorVersion = declaration.major
							originalDescriptor.AccessFlags = declaration.flags
							if err := os.WriteFile(descriptorPath, originalDescriptor.Bytes(), 0600); err != nil {
								t.Fatal(err)
							}
							want := "descriptor:identity:annotations:retention\n"
							if got := t04RunJava(t, java, originalDir, "DescriptorDriver"); got != want {
								t.Fatal("original oracle", got)
							}
							files := map[string][]byte{}
							if err := filepath.Walk(originalDir, func(p string, info os.FileInfo, err error) error {
								if err != nil {
									return err
								}
								if !info.IsDir() && strings.HasSuffix(p, ".class") {
									rel, e := filepath.Rel(originalDir, p)
									if e != nil {
										return e
									}
									raw, e := os.ReadFile(p)
									if e != nil {
										return e
									}
									files[filepath.ToSlash(rel)] = raw
								}
								return nil
							}); err != nil {
								t.Fatal(err)
							}
							jar := filepath.Join(t.TempDir(), "original.jar")
							if err := os.WriteFile(jar, t23Zip(t, files), 0600); err != nil {
								t.Fatal(err)
							}
							a, err := NewJarReconstructionArtifactFromLocal(jar, 8, profile, nil)
							if err != nil {
								t.Fatal(err)
							}
							defer a.Close()
							name := base + "package-info.class"
							source, err := a.ReadFile(name)
							if err != nil {
								t.Fatal(err)
							}
							outFiles := map[string][]byte{}
							for n, b := range files {
								if n != name {
									outFiles[n] = b
								}
							}
							out := sourceArtifactWriteFiles(t, outFiles)
							path := filepath.Join(out, base+"package-info.java")
							if err := os.WriteFile(path, source, 0600); err != nil {
								t.Fatal(err)
							}
							args = []string{"-proc:none", "-Xpkginfo:always", "-g:" + debug, "-cp", out, "-d", out}
							if profile == ModernJavac {
								args = append(args, "--release", "8")
							}
							args = append(args, path)
							if log, err := exec.Command(compiler, args...).CombinedOutput(); err != nil {
								t.Fatalf("fresh descriptor: %v\n%s\n%s", err, log, source)
							}
							fresh, err := os.ReadFile(filepath.Join(out, name))
							if err != nil {
								t.Fatal(err)
							}
							before := bytes.Clone(fresh)
							done, err := a.FinishClass(name, fresh)
							if err != nil {
								t.Fatal("descriptor publication", err)
							}
							if !bytes.Equal(fresh, before) {
								t.Fatal("descriptor publication must retain compiler bytes")
							}
							if err := os.WriteFile(filepath.Join(out, name), done, 0600); err != nil {
								t.Fatal(err)
							}
							if got := t04RunJava(t, java, out, "DescriptorDriver"); got != want {
								t.Fatal("finished caller", got)
							}
							if again, err := a.FinishClass(name, done); err != nil || !bytes.Equal(again, done) {
								t.Fatal("idempotent publication", err)
							}
							old, new := mustParseLowering(t, files[name]), mustParseLowering(t, done)
							for _, invisible := range []bool{true} {
								count := func(o *ClassObject) int {
									n := 0
									for _, attr := range o.Attributes {
										if a, ok := attr.(*RuntimeVisibleAnnotationsAttribute); ok && a.IsInvisible == invisible {
											n += len(a.Annotations)
										}
									}
									return n
								}
								if count(old) != count(new) {
									t.Fatal("annotation retention changed")
								}
							}
						})
					}
				}
			}
		}
	}
}
