package javaclassparser

import (
	"strings"
	"testing"
)

const anonymousMemberAllocationFixture = `class AllocationEffects {static String trace="";static boolean fail;static final RuntimeException error=new RuntimeException("argument");static int arg(int n){trace+="A";if(fail)throw error;return n;}}
class AllocationOwner<T> {
 final T token;AllocationOwner(T token){this.token=token;}
 class Entry {final int word;final Object owner;Entry(int n){AllocationEffects.trace+="E";word=n;owner=AllocationOwner.this;}}
 abstract class Reader {private Reader(){if(enclosing()!=AllocationOwner.this)throw new AssertionError("capture before superclass callback");}abstract Object enclosing();abstract Entry read(int n);}
 Reader make(){return new Reader(){Object enclosing(){return AllocationOwner.this;}Entry read(int n){return new Entry(AllocationEffects.arg(n));}};}
}
class AllocationDriver {public static void main(String[]args){int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(boolean fail:new boolean[]{false,true}){Object token=new Object();AllocationOwner<Object> owner=new AllocationOwner<Object>(token);AllocationOwner<Object>.Reader reader=owner.make();AllocationEffects.trace="";AllocationEffects.fail=fail;try{AllocationOwner<Object>.Entry entry=reader.read(n);if(fail||entry.word!=n||entry.owner!=owner||owner.token!=token||!AllocationEffects.trace.equals("AE"))throw new AssertionError("member allocation identity/order");}catch(RuntimeException e){if(!fail||e!=AllocationEffects.error||!AllocationEffects.trace.equals("A"))throw new AssertionError("argument failure identity/order",e);}rows++;}System.out.println(rows+":anonymous:member:allocation:effects:identity");}}`

func TestAdversarialAnonymousLexicalMemberAllocationRoundTrip(t *testing.T) {
	testSourceTargetReleaseFamilyFixture(t, anonymousMemberAllocationFixture, "AllocationOwner", "AllocationDriver", "10:anonymous:member:allocation:effects:identity\n", "8", []int{8, 11, 16})
}

func TestAdversarialAnonymousNestedLexicalMemberAllocationRoundTrip(t *testing.T) {
	fixture := anonymousNestedMemberAllocationFixture()
	testSourceTargetReleaseFamilyFixture(t, fixture, "AllocationOwner", "AllocationDriver", "10:anonymous:member:allocation:effects:identity\n", "8", []int{8, 11, 16})
}
func TestAdversarialAnonymousExplicitLexicalMemberAllocationRoundTrip(t *testing.T) {
	fixture := strings.Replace(anonymousMemberAllocationFixture, "return new Entry(AllocationEffects.arg(n));", "return AllocationOwner.this.new Entry(AllocationEffects.arg(n));", 1)
	testSourceTargetReleaseFamilyFixture(t, fixture, "AllocationOwner", "AllocationDriver", "10:anonymous:member:allocation:effects:identity\n", "8", []int{8, 11, 16})
}

func anonymousNestedMemberAllocationFixture() string {
	return strings.Replace("interface AllocationThunk{Object take(int n);}"+anonymousMemberAllocationFixture, "Entry read(int n){return new Entry(AllocationEffects.arg(n));}", "Entry read(int n){AllocationThunk thunk=new AllocationThunk(){public Object take(int m){return new Entry(AllocationEffects.arg(m));}};return (Entry)thunk.take(n);}", 1)
}
