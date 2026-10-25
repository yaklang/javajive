package javaclassparser

import (
	"crypto/sha256"
	"testing"
)

// A returned member comment is not authority to complete its class. Only the
// complete owning source transports the sealed child origin, and a changed or
// conflicting source cannot replace an already activated origin.
func TestPrivateNestPublicationRequiresCompleteReturnedOwningSource(t *testing.T) {
	source := []byte("class Publisher { class Member {} }")
	original := []byte("original member identity")
	origin := sourcePrivateNestOrigin{"Publisher$Member", sha256.Sum256(original)}
	for _, variant := range []string{"exact", "cached exact", "cached wrong owner", "cached changed source", "cached conflicting origin", "child comment", "changed source", "duplicate child", "pending child", "changed active origin", "changed repeat"} {
		t.Run(variant, func(t *testing.T) {
			z := &JarFS{}
			metadata := &sourceLambdaMetadataAuthority{jar: z, publications: map[string]*sourceLambdaPublication{"Publisher": {owner: "Publisher", sourceHash: sha256.Sum256(source), nests: []sourcePrivateNestOrigin{origin}}}}
			z.sourceLambdaMetadata = metadata
			a := &sourceConstructorLoweringAuthority{jar: z}
			z.sourceConstructorLowering = a
			owner, input := "Publisher", source
			if variant == "cached exact" || variant == "cached wrong owner" || variant == "cached changed source" || variant == "cached conflicting origin" {
				cached := metadata.publications[owner]
				metadata.publicationSources = map[sourceLambdaPublicationKey]*sourceLambdaPublication{{owner, cached.sourceHash}: cached}
				metadata.publications[owner] = &sourceLambdaPublication{owner: owner, sourceHash: sha256.Sum256([]byte("speculative alternative"))}
				if a.privateNestSourceReturned(origin.owner, original, nil) {
					t.Fatal("retaining a historical certificate activated it")
				}
			}
			switch variant {
			case "cached wrong owner":
				owner = "UnrelatedPublisher"
			case "cached changed source":
				input = append(append([]byte{}, source...), ' ')
			case "cached conflicting origin":
				a.nestNativeActive = map[string]constructorNativeSourceOrigin{origin.owner: {sha256.Sum256([]byte("other original")), sha256.Sum256(source)}}
			case "child comment":
				owner, input = "Publisher$Member", []byte("// original member body owned by Publisher")
			case "changed source":
				input = append(append([]byte{}, source...), ' ')
			case "duplicate child":
				metadata.publications["Publisher"].nests = append(metadata.publications["Publisher"].nests, origin)
			case "pending child":
				metadata.publications = nil
				a.nestPending = map[string]constructorNativeSourceOrigin{origin.owner: {origin.originalHash, sha256.Sum256(source)}}
			case "changed active origin":
				a.nestNativeActive = map[string]constructorNativeSourceOrigin{origin.owner: {sha256.Sum256([]byte("other original")), sha256.Sum256(source)}}
			}
			err := a.sourceReturned(owner, input)
			if variant == "duplicate child" || variant == "changed active origin" || variant == "cached conflicting origin" {
				if err == nil || len(a.nestActive) != 0 {
					t.Fatal("conflicting publication was activated", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			allowed := variant == "exact" || variant == "changed repeat" || variant == "cached exact"
			if a.privateNestSourceReturned(origin.owner, original, nil) != allowed {
				t.Fatal("owning source boundary changed")
			}
			if !allowed {
				return
			}
			if err = a.sourceReturned(origin.owner, []byte("// member compiled with owner")); err != nil || !a.privateNestSourceReturned(origin.owner, original, nil) {
				t.Fatal("child comment replaced owning origin", err)
			}
			if variant == "changed repeat" {
				next := append(append([]byte{}, source...), ' ')
				metadata.publications["Publisher"] = &sourceLambdaPublication{owner: "Publisher", sourceHash: sha256.Sum256(next), nests: []sourcePrivateNestOrigin{origin}}
				if err = a.sourceReturned("Publisher", next); err == nil || !a.privateNestSourceReturned(origin.owner, original, nil) {
					t.Fatal("changed complete owner replaced active proof", err)
				}
			}
		})
	}
}
