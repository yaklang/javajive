package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Authored originals, compiled by two real toolchains at Java7/8 source levels.
// No bytecode metadata is patched or normalized. Candidate classpath excludes
// every original target.
func TestAdversarialAnonymousCompilerProfilesRetainOriginalMetadata(t *testing.T) {
	modern, java := t04Tools(t)
	legacy := os.Getenv("JAVA8_JAVAC")
	if legacy != "" {
		if _, err := os.Stat(legacy); err != nil {
			t.Fatal(err)
		}
		version, err := exec.Command(legacy, "-version").CombinedOutput()
		if err != nil || !strings.Contains(string(version), "javac 1.8.") {
			t.Fatalf("native javac8 oracle required: %v %s", err, version)
		}
	}
	for _, compiler := range []string{"modern_release8", "native8", "native7"} {
		for _, debug := range []string{"none", "source,lines,vars"} {
			for _, static := range []bool{false, true} {
				for _, checked := range []bool{false, true} {
					label := fmt.Sprintf("%s/%s/static=%t/checked=%t", compiler, debug, static, checked)
					t.Run(label, func(t *testing.T) {
						native := strings.HasPrefix(compiler, "native")
						if native && legacy == "" {
							t.Skip("JAVA8_JAVAC is required for the independent native javac8 oracle")
						}
						modifier, throws := "", ""
						if static {
							modifier = "static "
						}
						if checked {
							throws = " throws java.io.IOException"
						}
						parent := `class ProfileParent{static String trace="";static final java.io.IOException failure=new java.io.IOException("same");final long v;ProfileParent(long v)` + throws + `{trace+="P";this.v=v;`
						if checked {
							parent += `if(v<0)throw failure;`
						}
						parent += `}long get(){return v;}}`
						source := `class ProfileTarget{` + modifier + `ProfileParent make(final long word)` + throws + `{return new ProfileParent(word){long get(){return word^0xCAFEBABEL;}};}}`
						driver := `import java.io.*;import java.lang.reflect.*;import java.util.*;class ProfileDriver{
static int flags(Class<?> c)throws Exception{DataInputStream in=new DataInputStream(c.getResourceAsStream("/"+c.getName().replace('.','/')+".class"));if(in.readInt()!=0xCAFEBABE)throw new AssertionError();in.readInt();int count=in.readUnsignedShort();for(int i=1;i<count;i++){int tag=in.readUnsignedByte();switch(tag){case 1:in.readUTF();break;case 3:case 4:in.readInt();break;case 5:case 6:in.readLong();i++;break;case 7:case 8:case 16:case 19:case 20:in.readUnsignedShort();break;case 9:case 10:case 11:case 12:case 17:case 18:in.readInt();break;case 15:in.readByte();in.readShort();break;default:throw new AssertionError("CP"+tag);}}return in.readUnsignedShort();}
static ProfileParent invoke(ProfileTarget owner,long word)throws IOException{return owner.make(word);}
public static void main(String[] args)throws Exception{ProfileTarget owner=new ProfileTarget();int rows=0;for(long word:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}){ProfileParent.trace="";try{ProfileParent p=invoke(owner,word);if(p.v!=word||p.get()!=(word^0xCAFEBABEL)||!ProfileParent.trace.equals("P"))throw new AssertionError("values/effects");Class<?> c=p.getClass();Constructor<?> ctor=c.getDeclaredConstructors()[0];List<String> fields=new ArrayList<String>();for(Field f:c.getDeclaredFields())fields.add(f.getName()+":"+f.getType().getName()+":"+f.getModifiers()+":"+f.isSynthetic());Collections.sort(fields);System.out.println(c.getName()+":"+flags(c)+":"+c.getModifiers()+":"+c.isAnonymousClass()+":"+c.getEnclosingMethod().getName()+":"+Arrays.toString(ctor.getParameterTypes())+":"+Arrays.toString(ctor.getExceptionTypes())+":"+fields);rows++;}catch(IOException e){if(e!=ProfileParent.failure||word>=0||!ProfileParent.trace.equals("P"))throw new AssertionError("identity",e);rows++;}}System.out.println("rows:"+rows);}}
`
						original := t.TempDir()
						for name, text := range map[string]string{"ProfileParent.java": parent, "ProfileTarget.java": source, "ProfileDriver.java": driver} {
							if err := os.WriteFile(filepath.Join(original, name), []byte(text), 0600); err != nil {
								t.Fatal(err)
							}
						}
						javac := modern
						args := []string{"-proc:none", "--release", "8"}
						if native {
							javac = legacy
							sourceLevel := "8"
							if compiler == "native7" {
								sourceLevel = "7"
							}
							args = []string{"-proc:none", "-source", sourceLevel, "-target", sourceLevel}
						}
						args = append(args, "-g:"+debug, "-d", original, filepath.Join(original, "ProfileParent.java"), filepath.Join(original, "ProfileTarget.java"), filepath.Join(original, "ProfileDriver.java"))
						if out, err := exec.Command(javac, args...).CombinedOutput(); err != nil {
							t.Fatalf("authored original compile: %v %s", err, out)
						}
						oracle := t04RunJava(t, java, original, "ProfileDriver")
						if !strings.Contains(oracle, "rows:5\n") {
							t.Fatalf("original oracle missing: %s", oracle)
						}
						t.Logf("ORIGINAL_FIRST %s", oracle)
						files := map[string][]byte{}
						entries, err := os.ReadDir(original)
						if err != nil {
							t.Fatal(err)
						}
						for _, entry := range entries {
							if strings.HasSuffix(entry.Name(), ".class") {
								raw, err := os.ReadFile(filepath.Join(original, entry.Name()))
								if err != nil {
									t.Fatal(err)
								}
								files[entry.Name()] = raw
							}
						}
						compilerProfile := ModernJavac
						if native {
							compilerProfile = NativeJavac8
						}
						// Independently compiled Java7 inputs use major51. The selected
						// rebuild still uses Java8; metadata and observations, rather
						// than equal class-file versions, form the source contract.
						for _, name := range []string{"ProfileTarget.class", "ProfileTarget$1.class"} {
							cf, err := Parse(files[name])
							if err != nil {
								t.Fatal(err)
							}
							major := uint16(52)
							if compiler == "native7" {
								major = 51
							}
							if cf.MajorVersion != major || cf.MinorVersion != 0 {
								t.Fatalf("actual original compiler profile %s: %d.%d", name, cf.MajorVersion, cf.MinorVersion)
							}
						}
						jar := filepath.Join(t.TempDir(), "authored.jar")
						if err := os.WriteFile(jar, t23Zip(t, files), 0600); err != nil {
							t.Fatal(err)
						}
						archive, err := NewJarFSFromLocalWithCompilerProfile(jar, 8, compilerProfile, nil)
						if err != nil {
							t.Fatal(err)
						}
						defer archive.Close()
						rebuilt := t.TempDir()
						for name, raw := range files {
							if strings.HasPrefix(name, "ProfileTarget") {
								continue
							}
							if err := os.WriteFile(filepath.Join(rebuilt, name), raw, 0600); err != nil {
								t.Fatal(err)
							}
						}
						paths := []string{}
						for _, name := range []string{"ProfileTarget", "ProfileTarget$1"} {
							text, err := archive.ReadFile(name + ".class")
							if err != nil {
								t.Fatal(err)
							}
							path := filepath.Join(rebuilt, name+".java")
							if err := os.WriteFile(path, text, 0600); err != nil {
								t.Fatal(err)
							}
							paths = append(paths, path)
						}
						candidateArgs := []string{"-proc:none", "--release", "8"}
						if native {
							candidateArgs = []string{"-proc:none", "-source", "8", "-target", "8"}
						}
						candidateArgs = append(candidateArgs, "-g:"+debug, "-cp", rebuilt, "-d", rebuilt)
						candidateArgs = append(candidateArgs, paths...)
						if out, err := exec.Command(javac, candidateArgs...).CombinedOutput(); err != nil {
							t.Fatalf("candidate compile: %v %s", err, out)
						}
						if actual := t04RunJava(t, java, rebuilt, "ProfileDriver"); actual != oracle {
							t.Fatalf("compiled candidate observations differ:\noriginal:%s\ncandidate:%s", oracle, actual)
						}
					})
				}
			}
		}
	}
}
