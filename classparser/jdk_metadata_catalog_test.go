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
	if len(document.Profiles) != 6 {
		t.Fatal("expected exact profiles 8/9/11/16/17/21")
	}
	expected := map[int]bool{8: true, 9: true, 11: true, 16: true, 17: true, 21: true}
	for _, profile := range document.Profiles {
		if !expected[profile.Release] {
			t.Fatal("unexpected or duplicate profile", profile.Release)
		}
		delete(expected, profile.Release)
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

func TestJDKCatalogRawBitIntrinsicsHaveExactCompleteDeclarations(t *testing.T) {
	for _, release := range []int{8, 9, 11, 16, 17, 21} {
		for _, target := range []struct{ owner, member, descriptor string }{
			{"java/lang/Float", "intBitsToFloat", "(I)F"},
			{"java/lang/Double", "longBitsToDouble", "(J)D"},
		} {
			cls, known := jdkInvocationMetadata(target.owner, release)
			if !known || cls.Name != target.owner || !cls.Public || cls.IsInterface || !cls.MembersComplete || !cls.ParentsComplete {
				t.Fatalf("incomplete raw-bit owner %d/%s", release, target.owner)
			}
			matches := 0
			for _, method := range cls.Methods {
				if method.Name == target.member && method.Desc == target.descriptor {
					if !method.Public || !method.Static || method.Bridge || !method.ExceptionsKnown || len(method.Exceptions) != 0 {
						t.Fatalf("invalid intrinsic declaration %d/%s: %+v", release, target.owner, method)
					}
					matches++
				}
			}
			if matches != 1 {
				t.Fatalf("missing or duplicate exact intrinsic %d/%s %d", release, target.owner, matches)
			}
		}
	}
}

func TestJDKCatalogExactCheckedDeclarationsAndChannels(t *testing.T) {
	for _, release := range []int{8, 9, 11, 16, 17, 21} {
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

func TestJDKCatalogModularNamespaceAndCheckedDeclarations(t *testing.T) {
	for _, release := range []int{8, 9, 11, 16, 17, 21} {
		provider := func(name string) (callbinding.Class, bool) { return jdkInvocationMetadata(name, release) }
		for _, tc := range []struct{ owner, name, desc, exception string }{
			{"org/xml/sax/helpers/DefaultHandler", "startDocument", "()V", "org/xml/sax/SAXException"},
			{"org/xml/sax/ext/LexicalHandler", "comment", "([CII)V", "org/xml/sax/SAXException"},
			{"java/beans/PropertyEditorSupport", "setValue", "(Ljava/lang/Object;)V", ""},
			{"javax/sql/DataSource", "getConnection", "()Ljava/sql/Connection;", "java/sql/SQLException"},
			{"javax/naming/spi/ObjectFactory", "getObjectInstance", "(Ljava/lang/Object;Ljavax/naming/Name;Ljavax/naming/Context;Ljava/util/Hashtable;)Ljava/lang/Object;", "java/lang/Exception"},
			{"java/security/PrivilegedAction", "run", "()Ljava/lang/Object;", ""},
		} {
			exceptions, known := exactInvocationExceptions(provider, tc.owner, tc.name, tc.desc)
			if !known || tc.exception == "" && len(exceptions) != 0 || tc.exception != "" && (len(exceptions) != 1 || exceptions[0] != tc.exception) {
				t.Fatalf("%d %s.%s%s known=%v exceptions=%v", release, tc.owner, tc.name, tc.desc, known, exceptions)
			}
		}
		xml, known := provider("org/xml/sax/XMLReader")
		if !known || !xml.IsInterface || !xml.MembersComplete || !xml.ParentsComplete {
			t.Fatalf("%d incomplete original XMLReader", release)
		}
		// The enclosing source helper still needs a complete ancestor namespace.
		// Unavailable third-party parents must not become guessed platform tables.
		if _, known := provider("org/eclipse/jetty/alpn/ALPN$ClientProvider"); known {
			t.Fatal("invented third-party namespace")
		}
	}
	var document struct {
		Profiles []struct {
			Release    int               `json:"release"`
			Archives   map[string]string `json:"archives"`
			Provenance map[string]struct {
				Archive string `json:"archive"`
			} `json:"provenance"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(jdkInvocationCatalogJSON, &document); err != nil {
		t.Fatal(err)
	}
	for _, profile := range document.Profiles {
		for _, tc := range []struct{ name, module string }{{"org/xml/sax/helpers/DefaultHandler", "java.xml.jmod"}, {"java/beans/PropertyEditorSupport", "java.desktop.jmod"}, {"javax/naming/spi/ObjectFactory", "java.naming.jmod"}, {"javax/sql/DataSource", "java.sql.jmod"}} {
			module := tc.module
			if profile.Release == 8 {
				module = "rt.jar"
			}
			evidence, known := profile.Provenance[tc.name]
			digest, err := hex.DecodeString(profile.Archives[module])
			if !known || evidence.Archive != module || err != nil || len(digest) != 32 {
				t.Fatalf("%d %s invalid original archive provenance", profile.Release, tc.name)
			}
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
	if has(8, "java/util/Map", "of") || !has(9, "java/util/Map", "of") || !has(11, "java/util/Map", "of") {
		t.Fatal("Map.of leaked across profile boundary")
	}
	if has(11, "java/lang/String", "indent") || !has(16, "java/lang/String", "indent") || !has(17, "java/lang/String", "indent") {
		t.Fatal("String.indent leaked across profile boundary")
	}
	for _, release := range []int{8, 9, 11, 16, 17, 21} {
		_, ok := jdkInvocationMetadata("java/lang/Record", release)
		if ok != (release >= 16) {
			t.Fatal("Record profile mismatch", release)
		}
	}
	for _, release := range []int{7, 10, 12, 18, 20, 22, 99} {
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
	for _, release := range []int{8, 9, 11, 16, 17, 21} {
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
	for _, release := range []int{8, 9, 11, 16, 17, 21} {
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
		if release >= 16 {
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
	for _, release := range []int{8, 9, 11, 16, 17, 21} {
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
	for _, release := range []int{8, 9, 11, 16, 17, 21} {
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

func TestInvocationMetadataReferenceCatalogRequiresOriginalClosedProfilesAndNeverClaimsMembers(t *testing.T) {
	var document struct {
		Schema   int `json:"schema"`
		Profiles []struct {
			Release    int                 `json:"release"`
			Parents    map[string][]string `json:"reference_hierarchy"`
			Provenance map[string]struct {
				SHA256  string `json:"sha256"`
				Major   int    `json:"major"`
				Archive string `json:"archive"`
				Entry   string `json:"entry"`
			} `json:"reference_provenance"`
		} `json:"profiles"`
	}
	if e := json.Unmarshal(jdkInvocationCatalogJSON, &document); e != nil {
		t.Fatal(e)
	}
	if document.Schema != 3 || len(document.Profiles) != 6 {
		t.Fatal("exact hierarchy profiles required")
	}
	for _, profile := range document.Profiles {
		if len(profile.Parents) != len(profile.Provenance) || len(profile.Parents) == 0 {
			t.Fatal("missing original class provenance")
		}
		for n, parents := range profile.Parents {
			origin, known := profile.Provenance[n]
			digest, e := hex.DecodeString(origin.SHA256)
			if !known || e != nil || len(digest) != 32 || origin.Major < 45 || origin.Major > profile.Release+44 || origin.Archive == "" || origin.Entry == "" {
				t.Fatal("invalid original provenance", n)
			}
			got, known := jdkReferenceSupertypes(n, profile.Release)
			if !known || len(got) != len(parents) {
				t.Fatal("missing exact platform declaration", n)
			}
			for i, parent := range parents {
				if got[i] != parent {
					t.Fatal("parent identity changed", n)
				}
				if _, known := profile.Parents[parent]; !known {
					t.Fatal("missing closed original ancestor", n, parent)
				}
			}
			if len(got) > 0 {
				got[0] = "mutated"
				again, _ := jdkReferenceSupertypes(n, profile.Release)
				if again[0] != parents[0] {
					t.Fatal("shared hierarchy exposed to request mutation")
				}
			}
		}
		p, known := jdkReferenceSupertypes("java/security/SecureRandom", profile.Release)
		if !known || len(p) != 1 || p[0] != "java/util/Random" {
			t.Fatal("original platform inheritance lost")
		}
		if _, known := jdkInvocationMetadata("java/security/SecureRandom", profile.Release); known {
			t.Fatal("ancestry evidence was promoted to a complete member table")
		}
	}
	for _, release := range []int{0, 7, 10, 15, 18, 22} {
		if _, known := jdkReferenceSupertypes("java/security/SecureRandom", release); known {
			t.Fatal("substituted a different platform profile")
		}
	}
	if _, known := jdkReferenceSupertypes("java/security/UnknownOriginal", 8); known {
		t.Fatal("invented a platform declaration")
	}
}
