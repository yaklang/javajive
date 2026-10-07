package javaclassparser

import (
	"context"
	"github.com/yaklang/javajive/internal/workbudget"
	"strings"
	"testing"
)

const enumConstructorMarkerFixture = `enum SwitchBridgeMode {ADD,SUB,XOR}
interface SwitchBridgeOp {long read(SwitchBridgeMode mode,long n);Object token();}
class SwitchBridgeEffects {static boolean fail;static Object published;static long seen;static Object seenOuter;static String trace;static final RuntimeException error=new RuntimeException("identity");}
abstract class SwitchBridgeObserver {SwitchBridgeObserver(long word){SwitchBridgeEffects.trace+="P";SwitchBridgeEffects.published=this;SwitchBridgeEffects.seen=word;SwitchBridgeEffects.seenOuter=enclosing();if(SwitchBridgeEffects.fail)throw SwitchBridgeEffects.error;}abstract Object enclosing();}
class SwitchBridgeOwner {
 private final long offset;SwitchBridgeOwner(long offset){this.offset=offset;}
 private long input(long seed){SwitchBridgeEffects.trace+="A";return seed+offset;}
 final class Cell extends SwitchBridgeObserver {final long word;private Cell(long word){super(word);this.word=word;}Object enclosing(){return SwitchBridgeOwner.this;}}
 long apply(SwitchBridgeMode mode,Cell cell,long n){switch(mode){case ADD:return cell.word+n;case SUB:return cell.word-n;default:return cell.word^n;}}
 SwitchBridgeOp make(final long seed,final Object token){return new SwitchBridgeOp(){public Object token(){return token;}public long read(SwitchBridgeMode mode,long n){Cell cell=new Cell(input(seed));return apply(mode,cell,n);}};}
}
class SwitchBridgeDriver {public static void main(String[]args)throws Exception{int rows=0;Object identity=new Object();for(long offset:new long[]{Long.MIN_VALUE,0,Long.MAX_VALUE})for(long seed:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(long n:new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE})for(Object token:new Object[]{null,identity})for(boolean fail:new boolean[]{false,true})for(SwitchBridgeMode mode:SwitchBridgeMode.values()){SwitchBridgeOwner owner=new SwitchBridgeOwner(offset);SwitchBridgeOp op=owner.make(seed,token);SwitchBridgeEffects.fail=fail;SwitchBridgeEffects.trace="";SwitchBridgeEffects.published=null;long base=java.math.BigInteger.valueOf(seed).add(java.math.BigInteger.valueOf(offset)).longValue();long expected=mode==SwitchBridgeMode.ADD?java.math.BigInteger.valueOf(base).add(java.math.BigInteger.valueOf(n)).longValue():mode==SwitchBridgeMode.SUB?java.math.BigInteger.valueOf(base).subtract(java.math.BigInteger.valueOf(n)).longValue():base^n;try{long got=op.read(mode,n);if(fail||got!=expected)throw new AssertionError("word/overflow");}catch(RuntimeException e){if(!fail||e!=SwitchBridgeEffects.error)throw new AssertionError("failure identity",e);}if(op.token()!=token||SwitchBridgeEffects.published==null||SwitchBridgeEffects.published.getClass()!=SwitchBridgeOwner.Cell.class||SwitchBridgeEffects.seen!=base||SwitchBridgeEffects.seenOuter!=owner||((SwitchBridgeOwner.Cell)SwitchBridgeEffects.published).word!=(fail?0:base)||op.getClass().getEnclosingMethod()==null||!op.getClass().getEnclosingMethod().getName().equals("make")||!SwitchBridgeEffects.trace.equals("AP"))throw new AssertionError("capture/construction/effect/identity");rows++;}SwitchBridgeEffects.fail=false;SwitchBridgeEffects.trace="";SwitchBridgeEffects.published=null;try{new SwitchBridgeOwner(0).make(0,identity).read(null,0);throw new AssertionError("null switch");}catch(NullPointerException expected){}if(!SwitchBridgeEffects.trace.equals("AP")||SwitchBridgeEffects.published==null)throw new AssertionError("null switch changed construction order");System.out.println(rows+":switch:constructor:shared-marker:overflow:identity:effects");}}
`

func TestAdversarialEnumSwitchSharedConstructorMarkerRoundTrip(t *testing.T) {
	testSourceTargetReleaseFamilyFixture(t, enumConstructorMarkerFixture, "SwitchBridgeOwner", "SwitchBridgeDriver", "900:switch:constructor:shared-marker:overflow:identity:effects\n", "8", []int{8, 11})
}

func enumSwitchRootMarkerFixture() string {
	fixture := strings.Replace(nativeRootPrivateConstructorFixture, "Member(Object value)", "private Member(Object value)", 1)
	fixture = "enum RootBridgeMode {A,B,C}\n" + fixture
	fixture = strings.Replace(fixture, "static boolean fail;", "static boolean fail;static Object seen;", 1)
	fixture = strings.Replace(fixture, "\n}\nclass RootBridgeDriver", `
 static int select(RootBridgeMode mode){switch(mode){case A:return 7;case B:return -3;default:return 9;}}
 static Runnable signal(final Object token){return new Runnable(){public void run(){RootBridgeEffects.seen=token;}};}
}
class RootBridgeDriver`, 1)
	fixture = strings.Replace(fixture, "RootBridgeEffects.fail=true;", `for(Object tokenValue:new Object[]{null,token}){RootBridgeEffects.seen=new Object();RootBridgePacket.signal(tokenValue).run();if(RootBridgeEffects.seen!=tokenValue)throw new AssertionError("captured marker object identity");}for(RootBridgeMode mode:RootBridgeMode.values()){int expected=mode==RootBridgeMode.A?7:mode==RootBridgeMode.B?-3:9;if(RootBridgePacket.select(mode)!=expected)throw new AssertionError("enum switch");}try{RootBridgePacket.select(null);throw new AssertionError("null switch");}catch(NullPointerException expected){}RootBridgeEffects.fail=true;`, 1)
	return fixture
}

func TestAdversarialEnumSwitchSharedRootAndMemberConstructorMarkerRoundTrip(t *testing.T) {
	fixture := enumSwitchRootMarkerFixture()
	testSourceTargetReleaseFamilyFixture(t, fixture, "RootBridgePacket", "RootBridgeDriver", "2:private-root:identity:order:owner\n", "8", []int{8, 11})
}

// Numbering admits a shared role only for the original first anonymous unit.
// These controls retain every surrounding packet and alter one certificate.
func TestNativeEnumSwitchSharedMarkerRequiresOriginalAnonymousRole(t *testing.T) {
	originals := nativeCompileClasses(t, enumConstructorMarkerFixture)
	for _, variant := range []string{"original", "empty marker", "missing anonymous plan", "missing marker", "missing marker object", "wrong anonymous owner", "wrong marker ordinal", "foreign marker", "missing child", "wrong table ordinal", "wrong field namespace", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for n, b := range originals {
				files[n] = append([]byte(nil), b...)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(files["SwitchBridgeOwner.class"])
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) {
				t.Fatal("original complete family")
			}
			if len(p.enumSwitchTables) != 1 || len(p.emptyMarkers) != 0 || p.anonymous == nil || len(p.anonymous.children) != 1 {
				t.Fatalf("original topology: tables=%v empty=%v anonymous=%v", p.enumSwitchTables, p.emptyMarkers, p.anonymous)
			}
			child := p.children["SwitchBridgeOwner$Cell"]
			if child == nil || len(child.accessBridges) != 1 {
				t.Fatal("original private member constructor")
			}
			var bridge *nativeConstructorAccessBridge
			for _, b := range child.accessBridges {
				bridge = b
			}
			marker := p.anonymous.children[bridge.marker]
			if marker == nil || marker.ordinal != 1 {
				t.Fatal("original marker is first executable anonymous unit")
			}
			var work *workbudget.Budget
			switch variant {
			case "empty marker":
				p.emptyMarkers = map[string]*ClassObject{bridge.marker: marker.object}
			case "missing anonymous plan":
				p.anonymous = nil
			case "missing marker":
				delete(p.anonymous.children, bridge.marker)
			case "missing marker object":
				marker.object = nil
			case "wrong anonymous owner":
				p.anonymous.owner = "Foreign"
			case "wrong marker ordinal":
				marker.ordinal = 2
			case "foreign marker":
				bridge.marker = "Foreign$1"
			case "missing child":
				p.children["SwitchBridgeOwner$Cell"] = nil
			case "wrong table ordinal":
				for n, table := range p.enumSwitchTables {
					_ = n
					p.enumSwitchTables = map[string]*nativeEnumSwitchTable{"SwitchBridgeOwner$3": table}
					break
				}
			case "wrong field namespace":
				for _, table := range p.enumSwitchTables {
					for name, arr := range table.tables {
						_ = name
						table.tables = map[string]*nativeEnumSwitchArray{"unproved": arr}
						break
					}
				}
			case "budget":
				work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				work = workbudget.New(ctx, workbudget.Limits{})
			}
			if got := nativeEnumSwitchOrdinalClosed(p, work); got != (variant == "original") {
				t.Fatalf("shared numbering role admitted=%v", got)
			}
		})
	}
}

func TestAdversarialEnumSwitchSharedPrivateRootAllocationAndMemberConstructorMarkerRoundTrip(t *testing.T) {
	fixture := strings.Replace(enumConstructorMarkerFixture, "SwitchBridgeOwner(long offset)", "private SwitchBridgeOwner(long offset)", 1)
	fixture = strings.Replace(fixture, "private final long offset;", "static class Factory {static SwitchBridgeOwner make(long offset){return new SwitchBridgeOwner(offset);}} private final long offset;", 1)
	fixture = strings.ReplaceAll(fixture, "new SwitchBridgeOwner(offset)", "SwitchBridgeOwner.Factory.make(offset)")
	// The factory itself must retain its original NEW/private-constructor edge.
	fixture = strings.Replace(fixture, "return SwitchBridgeOwner.Factory.make(offset);", "return new SwitchBridgeOwner(offset);", 1)
	fixture = strings.ReplaceAll(fixture, "new SwitchBridgeOwner(0)", "SwitchBridgeOwner.Factory.make(0)")
	testSourceTargetReleaseFamilyFixture(t, fixture, "SwitchBridgeOwner", "SwitchBridgeDriver", "900:switch:constructor:shared-marker:overflow:identity:effects\n", "8", []int{8, 11})
}

func TestNativeAnonymousRootConstructorMarkerRequiresJointDeclaration(t *testing.T) {
	originals := nativeCompileClasses(t, enumSwitchRootMarkerFixture())
	for _, variant := range []string{"original", "missing member plan", "missing root bridge", "wrong marker", "wrong target", "wrong original declaration", "missing original delegate", "budget", "canceled"} {
		t.Run(variant, func(t *testing.T) {
			files := map[string][]byte{}
			for n, b := range originals {
				files[n] = append([]byte(nil), b...)
			}
			z := nativeArchive(t, files)
			defer z.Close()
			root, e := Parse(files["RootBridgePacket.class"])
			if e != nil {
				t.Fatal(e)
			}
			d := z.nativeMemberReader(root)
			p := d.planNativeMemberFamily()
			if p == nil || !d.planNativeMemberAnonymousScopes(p) || p.anonymous == nil {
				t.Fatal("complete original root constructor/anonymous scope")
			}
			group := p.anonymous
			forest := p.anonymousForest
			var bridge *nativeConstructorAccessBridge
			for _, b := range p.rootAccessBridges {
				bridge = b
			}
			if bridge == nil {
				t.Fatal("actual original root bridge")
			}
			switch variant {
			case "missing member plan":
				p = nil
			case "missing root bridge":
				p.rootAccessBridges = nil
			case "wrong marker":
				bridge.marker = "Foreign$1"
			case "wrong target":
				bridge.target = "()V"
			case "wrong original declaration":
				for _, m := range root.Methods {
					desc, _ := sourceBridgeUTF8(root, m.DescriptorIndex)
					if desc == bridge.descriptor {
						m.AccessFlags &^= 0x1000
					}
				}
			case "missing original delegate":
				for i, m := range root.Methods {
					desc, _ := sourceBridgeUTF8(root, m.DescriptorIndex)
					if desc == bridge.target {
						root.Methods = append(root.Methods[:i], root.Methods[i+1:]...)
						break
					}
				}
			case "budget":
				d.Work = workbudget.New(nil, workbudget.Limits{MaxGraphScans: 1})
			case "canceled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				d.Work = workbudget.New(ctx, workbudget.Limits{})
			}
			got := d.validateNativeAnonymousGroup(group, p, forest) != nil
			if got != (variant == "original") {
				t.Fatalf("joint original declaration closure=%v", got)
			}
		})
	}
}
