package javaclassparser

import (
	"encoding/hex"
	"encoding/json"
	"sync"
	"testing"

	"github.com/yaklang/javajive/classparser/classes"
	"github.com/yaklang/javajive/classparser/decompiler/core/callbinding"
)

func TestJDKInvocationCatalogCompleteProfiles(t *testing.T) {
	var document struct {
		Profiles []struct {
			Release       int                          `json:"release"`
			ArchiveSHA256 string                       `json:"archive_sha256"`
			Classes       map[string]callbinding.Class `json:"classes"`
			Provenance    map[string]struct {
				SHA256 string `json:"sha256"`
				Major  int    `json:"major"`
			} `json:"provenance"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(jdkInvocationCatalogJSON, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Profiles) != 4 {
		t.Fatal("expected exact profiles 8/11/17/21")
	}
	for _, profile := range document.Profiles {
		if digest, err := hex.DecodeString(profile.ArchiveSHA256); err != nil || len(digest) != 32 {
			t.Fatal("missing archive provenance")
		}
		for name, cls := range profile.Classes {
			if cls.Name != name || !cls.MembersComplete || !cls.ParentsComplete {
				t.Fatalf("incomplete identity %d/%s", profile.Release, name)
			}
			if got, ok := jdkInvocationMetadata(name, profile.Release); !ok || len(got.Methods) != len(cls.Methods) {
				t.Fatalf("profile lookup %d/%s", profile.Release, name)
			}
			for _, parent := range cls.Parents {
				if _, ok := profile.Classes[parent]; !ok {
					t.Fatalf("missing ancestor %s", parent)
				}
			}
			source, ok := profile.Provenance[name]
			if !ok || source.Major != profile.Release+44 {
				t.Fatal("class version provenance mismatch", name)
			}
			if digest, err := hex.DecodeString(source.SHA256); err != nil || len(digest) != 32 {
				t.Fatal("missing class provenance", name)
			}
			for _, method := range cls.Methods {
				if !method.ExceptionsKnown {
					t.Fatal("unknown Exceptions declaration", name, method.Name, method.Desc)
				}
				if _, _, err := callbinding.Descriptor(method.Desc); err != nil {
					t.Fatal(name, method, err)
				}
			}
		}
	}
}

func TestJDKCatalogExactCheckedDeclarationsAndChannels(t *testing.T) {
	for _, release := range []int{8, 11, 17, 21} {
		provider := func(name string) (callbinding.Class, bool) { return jdkInvocationMetadata(name, release) }
		for _, tc := range []struct{ owner, name, desc, exception string }{
			{"java/lang/Object", "<init>", "()V", ""},
			{"java/io/Reader", "read", "()I", "java/io/IOException"},
			{"java/nio/channels/ReadableByteChannel", "read", "(Ljava/nio/ByteBuffer;)I", "java/io/IOException"},
			{"java/lang/reflect/InvocationHandler", "invoke", "(Ljava/lang/Object;Ljava/lang/reflect/Method;[Ljava/lang/Object;)Ljava/lang/Object;", "java/lang/Throwable"},
		} {
			exceptions, known := exactInvocationExceptions(provider, tc.owner, tc.name, tc.desc)
			if !known || tc.exception == "" && len(exceptions) != 0 || tc.exception != "" && (len(exceptions) != 1 || exceptions[0] != tc.exception) {
				t.Fatalf("%d %s.%s%s: known=%v exceptions=%v", release, tc.owner, tc.name, tc.desc, known, exceptions)
			}
		}
		if _, known := exactInvocationExceptions(provider, "java/io/Reader", "read", "(I)I"); known {
			t.Fatal("invented overload exception evidence")
		}
		cls, _ := provider("java/io/Reader")
		for i := range cls.Methods {
			if len(cls.Methods[i].Exceptions) > 0 {
				cls.Methods[i].Exceptions[0] = "corrupted"
			}
		}
		got, _ := exactInvocationExceptions(provider, "java/io/Reader", "read", "()I")
		if len(got) != 1 || got[0] != "java/io/IOException" {
			t.Fatal("request corrupted shared exception slice")
		}
	}
}

func TestJDKInvocationCatalogVersionBounds(t *testing.T) {
	has := func(release int, class, name string) bool {
		c, ok := jdkInvocationMetadata(class, release)
		if !ok {
			return false
		}
		for _, m := range c.Methods {
			if m.Name == name {
				return true
			}
		}
		return false
	}
	if has(8, "java/util/Map", "of") || !has(11, "java/util/Map", "of") {
		t.Fatal("Map.of leaked across profile boundary")
	}
	if has(11, "java/lang/String", "indent") || !has(17, "java/lang/String", "indent") {
		t.Fatal("String.indent leaked across profile boundary")
	}
	for _, release := range []int{8, 11, 17, 21} {
		_, ok := jdkInvocationMetadata("java/lang/Record", release)
		if ok != (release >= 17) {
			t.Fatal("Record profile mismatch", release)
		}
	}
	for _, release := range []int{7, 9, 10, 12, 16, 18, 20, 22, 99} {
		if _, ok := jdkInvocationMetadata("java/util/Map", release); ok {
			t.Fatal("uncataloged release was guessed", release)
		}
	}
	if _, ok := jdkInvocationMetadata("com/example/Missing", 17); ok {
		t.Fatal("unknown class was guessed")
	}
}

func TestJDKFilterInputStreamOriginalNamespace(t *testing.T) {
	// FilterInputStream is a real intermediate superclass, not an alias for
	// InputStream. Its complete original namespace is required before adding
	// a collision-free synthetic method to a subclass.
	for _, release := range []int{8, 11, 17, 21} {
		class, ok := jdkInvocationMetadata("java/io/FilterInputStream", release)
		if !ok || !class.MembersComplete || !class.ParentsComplete || len(class.Parents) != 1 || class.Parents[0] != "java/io/InputStream" {
			t.Fatalf("missing original FilterInputStream declaration in profile %d", release)
		}
		for _, descriptor := range []string{"()I", "([B)I", "([BII)I"} {
			count := 0
			for _, method := range class.Methods {
				if method.Name == "read" && method.Desc == descriptor && method.Public && !method.Static {
					count++
				}
			}
			if count != 1 {
				t.Fatalf("profile %d lost exact read overload %s", release, descriptor)
			}
		}
		provider := func(name string) (callbinding.Class, bool) { return jdkInvocationMetadata(name, release) }
		family, err := callbinding.FamilyOf(callbinding.Witness{Owner: class.Name, Name: "read", Desc: "()I", Kind: callbinding.Virtual}, provider)
		// The zero-argument call has one effective declaration after override
		// resolution; the other two overloads have different arities.
		if err != nil || !family.Complete || family.Target == nil || family.Proof != callbinding.Unique {
			t.Fatalf("profile %d lost complete original zero-argument read family: %+v %v", release, family, err)
		}
	}
}

func TestJDKInvocationCatalogGenericFamiliesAndStringFormal(t *testing.T) {
	for _, release := range []int{8, 11, 17, 21} {
		provider := func(name string) (callbinding.Class, bool) { return jdkInvocationMetadata(name, release) }
		for _, signature := range []struct{ name, desc string }{
			{"get", "(Ljava/lang/Object;)Ljava/lang/Object;"},
			{"merge", "(Ljava/lang/Object;Ljava/lang/Object;Ljava/util/function/BiFunction;)Ljava/lang/Object;"},
		} {
			f, err := callbinding.FamilyOf(callbinding.Witness{Owner: "java/util/Map", Name: signature.name, Desc: signature.desc, Kind: callbinding.Interface}, provider)
			if err != nil || !f.Complete || f.Proof != callbinding.Unique || f.Target == nil || !f.Target.Generic {
				t.Fatalf("release%d %s: %+v err=%v", release, signature.name, f, err)
			}
		}
		if release >= 17 {
			f, err := callbinding.FamilyOf(callbinding.Witness{Owner: "java/lang/Class", Name: "isRecord", Desc: "()Z", Kind: callbinding.Virtual}, provider)
			if err != nil || !f.Complete || f.Proof != callbinding.Unique || f.Target == nil || !f.Target.Public {
				t.Fatalf("Class.isRecord metadata: %+v %v", f, err)
			}
		}

		// Full tables preserve actual competing overloads; metadata never declares
		// every method with a known name uniquely bound.
		f, err := callbinding.FamilyOf(callbinding.Witness{Owner: "java/lang/String", Name: "valueOf", Desc: "(Ljava/lang/Object;)Ljava/lang/String;", Kind: callbinding.Static}, provider)
		if err != nil || !f.Complete || f.Proof != callbinding.Compete {
			t.Fatalf("String.valueOf competitors disappeared: %+v %v", f, err)
		}
		if str, ok := provider("java/lang/String"); !ok || !str.Public {
			t.Fatal("String formal not publicly denotable")
		}
		if !callbinding.Assignable("Ljava/lang/String;", "Ljava/lang/CharSequence;", provider) {
			t.Fatal("String's actual interfaces were lost")
		}
	}
}

func TestJDKInvocationCatalogRequestIsolation(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, ok := jdkInvocationMetadata("java/util/Map", 17)
			if !ok {
				t.Error("missing Map")
				return
			}
			c.Methods[0].Name = "corrupted"
			c.Parents[0] = "corrupted"
		}()
	}
	wg.Wait()
	c, _ := jdkInvocationMetadata("java/util/Map", 17)
	if c.Methods[0].Name == "corrupted" || c.Parents[0] == "corrupted" {
		t.Fatal("request modified shared catalog")
	}
}

func TestJDKInvocationMetadataUsesSourceProfileAndResolver(t *testing.T) {
	raw, err := classes.FS.ReadFile("LongTest.class")
	if err != nil {
		t.Fatal(err)
	}
	obj, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	dumper := NewClassObjectDumper(obj)
	provider := dumper.buildInvocationMetadata()
	if _, ok := provider("java/util/Map"); !ok {
		t.Fatal("class-major Java11 profile not selected")
	}
	dumper.options.TargetSourceVersion = 8
	map8, _ := dumper.buildInvocationMetadata()("java/util/Map")
	for _, method := range map8.Methods {
		if method.Name == "of" {
			t.Fatal("explicit source target ignored")
		}
	}
	dumper.options.TargetSourceVersion = 19
	if _, ok := dumper.buildInvocationMetadata()("java/util/Map"); ok {
		t.Fatal("unknown source target got inferred complete profile")
	}
	dumper.options.TargetSourceVersion = 11
	dumper.foldSiblingResolver = func(name string) ([]byte, bool) {
		if name == "java/util/Map" {
			return []byte("invalid class bytes"), true
		}
		return nil, false
	}
	if _, ok := dumper.buildInvocationMetadata()("java/util/Map"); ok {
		t.Fatal("invalid explicit resolver bytes hidden by catalog fallback")
	}
}

func TestJDKCatalogRetainsExactGenericSignatures(t *testing.T) {
	for _, release := range []int{8, 11, 17, 21} {
		optional, ok := jdkInvocationMetadata("java/util/Optional", release)
		if !ok || optional.Signature != "<T:Ljava/lang/Object;>Ljava/lang/Object;" {
			t.Fatalf("missing Optional class Signature for %d", release)
		}
		found := false
		for _, m := range optional.Methods {
			if m.Name == "of" && m.Desc == "(Ljava/lang/Object;)Ljava/util/Optional;" {
				found = m.Signature == "<T:Ljava/lang/Object;>(TT;)Ljava/util/Optional<TT;>;"
			}
		}
		if !found {
			t.Fatalf("missing exact generic method Signature for %d", release)
		}
	}
}

func TestJDKInvocationCatalogStackErasure(t *testing.T) {
	for _, release := range []int{8, 11, 17, 21} {
		cls, ok := jdkInvocationMetadata("java/util/Stack", release)
		if !ok || cls.Signature != "<E:Ljava/lang/Object;>Ljava/util/Vector<TE;>;" {
			t.Fatal("missing exact Stack declaration", release, cls.Signature)
		}
		found := false
		for _, method := range cls.Methods {
			if method.Name == "push" && method.Desc == "(Ljava/lang/Object;)Ljava/lang/Object;" {
				found = method.Signature == "(TE;)TE;" && method.Public && method.Generic && !method.Static && !method.Varargs && !method.Bridge
			}
		}
		if !found {
			t.Fatal("missing erased push identity", release)
		}
		parent, ok := jdkInvocationMetadata("java/util/Vector", release)
		if !ok || !parent.MembersComplete || !parent.ParentsComplete {
			t.Fatal("missing Stack ancestor", release)
		}
	}
}
