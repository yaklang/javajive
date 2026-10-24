package javaclassparser

import "testing"

// A switch-only owner is already admitted by ordinary native-member planning.
// The source transaction must use the same original ownership predicate;
// otherwise a helper lookup can publish a source that its owner cannot select.
func TestNativeEnumTransactionOwnerUsesOriginalArtifactAdmission(t *testing.T) {
	for _, prefix := range []string{"Admission", "Alternate"} {
		t.Run(prefix, func(t *testing.T) {
			files := nativeCompileClasses(t, "enum "+prefix+"Mode{A,B} class "+prefix+"Root{static int run("+prefix+"Mode mode,int n){switch(mode){case A:return n+1;case B:return n-1;default:throw new AssertionError();}}}")
			z := nativeArchive(t, files)
			defer z.Close()
			z.targetSourceVersion = 8
			object, e := Parse(files[prefix+"Root.class"])
			if e != nil {
				t.Fatal(e)
			}
			plan := z.nativeMemberReader(object).planNativeMemberFamily()
			if plan == nil || len(plan.enumSwitchTables) != 1 {
				t.Fatal("one original compiler table must already close ordinary ownership")
			}
			owner, known := z.nativeMemberTransactionOwner(object)
			if !known || owner != object.GetClassName() {
				t.Fatalf("transaction lost eligible original owner: %q %v", owner, known)
			}
		})
	}
}
