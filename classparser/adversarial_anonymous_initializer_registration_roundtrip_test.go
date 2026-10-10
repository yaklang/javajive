package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const anonymousInitializerRegistrationBody = `static Runnable first;static int steps;
 static{steps=10;first=new Runnable(){public void run(){steps+=1;}};steps+=100;}
 static Runnable second(){return new Runnable(){public void run(){steps+=2;}};}
 static Runnable third(){return new Runnable(){public void run(){steps+=4;}};}
 static{steps+=1000;}
 static class Anchor{}
`

const anonymousInitializerRegistrationDriver = `class InitializerRegistrationDriver{public static void main(String[] args){
 if(RegistrationRoot.steps!=1110)throw new AssertionError("initialization effect order");int rows=0;
 for(int i=0;i<4;i++){Runnable first=RegistrationRoot.first,second=RegistrationRoot.second(),third=RegistrationRoot.third();
  if(!first.getClass().getName().equals(RegistrationRoot.class.getName()+"$%d")||!second.getClass().getName().equals(RegistrationRoot.class.getName()+"$%d")||!third.getClass().getName().equals(RegistrationRoot.class.getName()+"$%d"))throw new AssertionError("original anonymous declaration identity");
  if(first.getClass().getEnclosingMethod()!=null||first.getClass().getEnclosingClass()!=RegistrationRoot.class||!second.getClass().getEnclosingMethod().getName().equals("second")||!third.getClass().getEnclosingMethod().getName().equals("third"))throw new AssertionError("original registration context");
  RegistrationRoot.steps=0;first.run();if(RegistrationRoot.steps!=1)throw new AssertionError("first result");second.run();if(RegistrationRoot.steps!=3)throw new AssertionError("second result");third.run();if(RegistrationRoot.steps!=7)throw new AssertionError("third result");rows++;
 }System.out.println(rows+":initializer:anonymous:registration:effects");}}
`

func TestAdversarialAnonymousInitializerRegistrationKeepsOriginalDeclarationOrder(t *testing.T) {
	initializer := `static{steps=10;first=new Runnable(){public void run(){steps+=1;}};steps+=100;}`
	for _, variant := range []struct {
		name                 string
		first, second, third int
	}{
		{"initializer first", 1, 2, 3},
		{"initializer middle", 2, 1, 3},
		{"initializer last", 3, 1, 2},
	} {
		t.Run(variant.name, func(t *testing.T) {
			body := anonymousInitializerRegistrationBody
			if variant.name != "initializer first" {
				body = strings.Replace(body, initializer, "", 1)
				after := `static Runnable second(){return new Runnable(){public void run(){steps+=2;}};}`
				if variant.name == "initializer last" {
					after = `static Runnable third(){return new Runnable(){public void run(){steps+=4;}};}`
				}
				body = strings.Replace(body, after, after+initializer, 1)
			}
			fixture := "class RegistrationRoot{" + body + "}" + fmt.Sprintf(anonymousInitializerRegistrationDriver, variant.first, variant.second, variant.third)
			testNativeIndependentFamilyFixture(t, fixture, []string{"RegistrationRoot"}, "InitializerRegistrationDriver", "4:initializer:anonymous:registration:effects\n", nativeLexicalExactSignatures)
		})
	}
}

func TestAdversarialAnonymousInitializerRegistrationNativeCompilerProtocol(t *testing.T) {
	javac := os.Getenv("JAVA8_JAVAC")
	if javac == "" {
		t.Skip("real javac8 oracle requires JAVA8_JAVAC")
	}
	version, err := exec.Command(javac, "-version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "javac 1.8.") {
		t.Fatal("native compiler identity", err, string(version))
	}
	fixture := "class RegistrationRoot{" + anonymousInitializerRegistrationBody + "}" + fmt.Sprintf(anonymousInitializerRegistrationDriver, 1, 2, 3)
	testNativeIndependentCompilerFamilyFixture(t, func(debug string) map[string][]byte {
		root := t.TempDir()
		path := filepath.Join(root, "RegistrationRoot.java")
		if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
			t.Fatal(err)
		}
		if raw, err := exec.Command(javac, "-proc:none", "-source", "8", "-target", "8", "-g:"+debug, "-d", root, path).CombinedOutput(); err != nil {
			t.Fatal("authored original compile", err, string(raw))
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatal(err)
		}
		files := map[string][]byte{}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".class") {
				raw, err := os.ReadFile(filepath.Join(root, entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				files[entry.Name()] = raw
			}
		}
		return files
	}, NativeJavac8, javac, []string{"RegistrationRoot"}, "InitializerRegistrationDriver", "4:initializer:anonymous:registration:effects\n", nil, nativeLexicalExactSignatures)
}
