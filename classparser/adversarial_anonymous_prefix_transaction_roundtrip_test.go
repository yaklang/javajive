package javaclassparser

import (
	"strconv"
	"strings"
	"testing"
)

// A terminal sibling has an original ACC_FINAL anonymous header that javac8
// cannot reproduce as an anonymous expression. The earlier siblings need their
// own complete source/constructor transaction; the tail must retain its existing
// standalone final declaration and actual iterator behavior.
func TestAdversarialAnonymousPrefixTransactionPreservesConstructorsAndIndependentTailRoundTrip(t *testing.T) {
	for _, root := range []string{"PrefixTransactionOwner", "RenamedPrefixTransactionOwner"} {
		for _, tailSuperclass := range []string{"interface", "platform superclass"} {
			for _, privateCopy := range []bool{false, true} {
				for _, named := range []bool{false, true} {
					t.Run("private-copy="+strconv.FormatBool(privateCopy)+"/named="+strconv.FormatBool(named), func(t *testing.T) {
						t.Run(tailSuperclass, func(t *testing.T) {
							t.Run(root, func(t *testing.T) {
								fixture := `class PrefixTransactionOwner {private final int base;PrefixTransactionOwner(int n){base=n;}PrefixTransactionOwner(PrefixTransactionOwner other){PrefixObserved.value=value();base=other.base;}int value(){return base;}PrefixTransactionOwner plus(final int delta){return new PrefixTransactionOwner(this){int value(){return PrefixTransactionOwner.this.base+delta;}};}PrefixTransactionOwner minus(){return new PrefixTransactionOwner(this){int value(){return PrefixTransactionOwner.this.base-7;}};}static Iterable<Integer> pair(final int first,final int second){return new Iterable<Integer>(){public java.util.Iterator<Integer> iterator(){return java.util.Arrays.asList(first,second).iterator();}};}}
class PrefixObserved {static int value;}
class PrefixTransactionDriver {public static void main(String[]args)throws Exception{int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int delta:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){PrefixTransactionOwner original=new PrefixTransactionOwner(n),plus=original.plus(delta);if(PrefixObserved.value!=java.math.BigInteger.valueOf(n).add(java.math.BigInteger.valueOf(delta)).intValue())throw new AssertionError("capture before copy-super callback");PrefixTransactionOwner minus=original.minus();if(PrefixObserved.value!=java.math.BigInteger.valueOf(n).subtract(java.math.BigInteger.valueOf(7)).intValue())throw new AssertionError("second capture before copy-super callback");if(plus.getClass().getEnclosingMethod()==null||!plus.getClass().getEnclosingMethod().getName().equals("plus")||minus.getClass().getEnclosingMethod()==null||!minus.getClass().getEnclosingMethod().getName().equals("minus"))throw new AssertionError("prefix enclosing method");if(plus.value()!=java.math.BigInteger.valueOf(n).add(java.math.BigInteger.valueOf(delta)).intValue()||minus.value()!=java.math.BigInteger.valueOf(n).subtract(java.math.BigInteger.valueOf(7)).intValue())throw new AssertionError("copy constructor/captures");Iterable<Integer> tail=PrefixTransactionOwner.pair(n,delta);if(!java.lang.reflect.Modifier.isFinal(tail.getClass().getModifiers()))throw new AssertionError("terminal original final flag");java.util.Iterator<Integer> it=tail.iterator();if(it.next()!=n||it.next()!=delta||it.hasNext())throw new AssertionError("independent terminal algorithm");try{it.next();throw new AssertionError("exhausted iterator");}catch(java.util.NoSuchElementException expected){}if(tail instanceof java.util.List)for(int bad:new int[]{Integer.MIN_VALUE,-1,2,Integer.MAX_VALUE})try{((java.util.List<?>)tail).get(bad);throw new AssertionError("terminal bad index");}catch(IndexOutOfBoundsException expected){}rows++;}System.out.println(rows+":prefix:constructors:lexical:tail:final");}}`
								if privateCopy {
									fixture = strings.Replace(fixture, "PrefixTransactionOwner(PrefixTransactionOwner other)", "private PrefixTransactionOwner(PrefixTransactionOwner other)", 1)
								}
								if tailSuperclass == "platform superclass" {
									fixture = strings.Replace(fixture, "new Iterable<Integer>(){public java.util.Iterator<Integer> iterator(){return java.util.Arrays.asList(first,second).iterator();}}", "new java.util.AbstractList<Integer>(){public int size(){return 2;}public Integer get(int i){switch(i){case 0:return first;case 1:return second;default:throw new IndexOutOfBoundsException();}}}", 1)
								}
								if named {
									fixture = strings.Replace(fixture, "static Iterable<Integer> pair", "static final class Named{private final PrefixTransactionOwner owner;private Named(PrefixTransactionOwner o){owner=o;}int read(){return owner.base;}}Named named(){return new Named(this);}static Iterable<Integer> pair", 1)
									fixture = strings.Replace(fixture, "Iterable<Integer> tail=", "if(original.named().read()!=n)throw new AssertionError(\"named private field/constructor bridge\");Iterable<Integer> tail=", 1)
								}
								fixture = strings.ReplaceAll(fixture, "PrefixTransactionOwner", root)
								testNativePrivateSetterCompiledFixtureWithShape(t, root, "PrefixTransactionDriver", "25:prefix:constructors:lexical:tail:final\n", func(t *testing.T, debug string) map[string][]byte {
									files := nativeCompileDebugClasses(t, fixture, debug)
									tail := root + "$3"
									for name, raw := range files {
										if !strings.HasPrefix(name, root) || !strings.HasSuffix(name, ".class") {
											continue
										}
										object, e := Parse(raw)
										if e != nil {
											t.Fatal(e)
										}
										if object.GetClassName() == tail {
											object.AccessFlags |= 0x0010
										}
										for _, attribute := range object.Attributes {
											if table, ok := attribute.(*InnerClassesAttribute); ok {
												for _, row := range table.Classes {
													n, known := sourceBridgeClassName(object, row.InnerClassInfoIndex)
													if known && n == tail {
														row.InnerClassAccessFlags |= 0x0010
													}
												}
											}
										}
										files[name] = object.Bytes()
									}
									return files
								}, nativeAnonymousPrefixRepresentationShape)
							})
						})
					})
				}
			}
		}
	}
}

// An independent terminal anonymous binary cannot retain EnclosingMethod or
// synthetic capture flags when represented by a named Java declaration. These
// representational differences are explicit; finality, source completeness,
// actual methods/fields, external ABI and independently executed behavior remain
// mandatory. Prefix EnclosingMethod is checked by the untouched original driver.
func nativeAnonymousPrefixRepresentationShape(t *testing.T, name string, original, rebuilt []byte) {
	t.Helper()
	old, e := Parse(original)
	if e != nil {
		t.Fatal(e)
	}
	fresh, e := Parse(rebuilt)
	if e != nil {
		t.Fatal(e)
	}
	if old.GetClassName() != fresh.GetClassName() || old.GetSupperClassName() != fresh.GetSupperClassName() || old.AccessFlags&(0x0010|0x0200|0x0400|0x4000) != fresh.AccessFlags&(0x0010|0x0200|0x0400|0x4000) {
		t.Fatalf("representation lost original class contract %s", name)
	}
	check := func(before, after []*MemberInfo) {
		actual := map[string]*MemberInfo{}
		for _, m := range after {
			n, nk := sourceBridgeUTF8(fresh, m.NameIndex)
			d, dk := sourceBridgeUTF8(fresh, m.DescriptorIndex)
			if !nk || !dk {
				t.Fatal("rebuilt declaration")
			}
			actual[n+d] = m
		}
		for _, m := range before {
			n, nk := sourceBridgeUTF8(old, m.NameIndex)
			d, dk := sourceBridgeUTF8(old, m.DescriptorIndex)
			if !nk || !dk {
				t.Fatal("original declaration")
			}
			if m.AccessFlags&0x1000 != 0 {
				continue
			}
			got := actual[n+d]
			if got == nil || m.AccessFlags&(0x0001|0x0004|0x0008|0x0010|0x0400) != got.AccessFlags&(0x0001|0x0004|0x0008|0x0010|0x0400) {
				t.Fatalf("representation lost original member contract %s %s%s", name, n, d)
			}
		}
	}
	check(old.Fields, fresh.Fields)
	check(old.Methods, fresh.Methods)
}

// A private nonstatic parent consumes its physical enclosing-instance word in
// addition to its ordinary source arguments. Its constructor callback must see
// anonymous captures already initialized; removing a marker is not sufficient.
func TestAdversarialAnonymousPrivateMemberSuperDelegationKeepsCapturesRoundTrip(t *testing.T) {
	for _, owner := range []string{"PrivateMemberPacketOwner", "RenamedPrivateMemberPacketOwner"} {
		t.Run(owner, func(t *testing.T) {
			fixture := `class PrivateMemberPacketOwner {
 final int base;
 PrivateMemberPacketOwner(int n){base=n;}
 PrivateMemberPacketOwner(PrivateMemberPacketOwner other){MemberPacketObserved.value=value();base=other.base;}
 int value(){return base;}
 class Parent extends PrivateMemberPacketOwner{private Parent(PrivateMemberPacketOwner other){super(other);}}
 PrivateMemberPacketOwner make(final int delta){return new Parent(this){int value(){return PrivateMemberPacketOwner.this.base+delta;}};}
}
class MemberPacketObserved{static int value;}
class PrivateMemberPacketDriver{public static void main(String[]args){int rows=0;for(int n:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE})for(int delta:new int[]{Integer.MIN_VALUE,-1,0,1,Integer.MAX_VALUE}){PrivateMemberPacketOwner owner=new PrivateMemberPacketOwner(n);PrivateMemberPacketOwner child=owner.make(delta);int expected=java.math.BigInteger.valueOf(n).add(java.math.BigInteger.valueOf(delta)).intValue();if(MemberPacketObserved.value!=expected||child.value()!=expected)throw new AssertionError("private member SUPER callback/capture order");if(child.getClass().getEnclosingMethod()==null||!child.getClass().getEnclosingMethod().getName().equals("make"))throw new AssertionError("original anonymous scope");rows++;}System.out.println(rows+":private:member:super:captures");}}
`
			fixture = strings.ReplaceAll(fixture, "PrivateMemberPacketOwner", owner)
			testNativePrivateSetterFixture(t, fixture, owner, "PrivateMemberPacketDriver", "25:private:member:super:captures\n")
		})
	}
}
