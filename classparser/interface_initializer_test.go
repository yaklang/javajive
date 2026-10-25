package javaclassparser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaklang/javajive/classparser/decompiler/core/class_context"
	"github.com/yaklang/javajive/classparser/decompiler/core/statements"
	"github.com/yaklang/javajive/classparser/decompiler/core/utils"
	"github.com/yaklang/javajive/classparser/decompiler/core/values"
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
)

func interfaceInitializerFixture() ([]statements.Statement, []interfaceFieldWrite, []string) {
	arrayType := types.NewJavaArrayType(types.NewJavaClass("java.lang.Object"))
	local := values.NewJavaRef(utils.NewRootVariableId(), nil, arrayType)
	allocation := &values.NewExpression{JavaType: arrayType, Length: []values.JavaValue{values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger))}, HasOriginPC: true, OriginPC: 0}
	assignment := statements.NewAssignStatement(local, allocation, false)
	assignment.OriginPC = 1
	assignment.HasOriginPC = true
	storeElement := statements.NewArrayMemberAssignStatement(values.NewJavaArrayMember(local, values.NewJavaLiteral(0, types.NewJavaPrimer(types.JavaInteger))), values.JavaNull)
	storeElement.HasOriginPC = true
	storeElement.OriginPC = 3
	first := &values.JavaClassMember{Name: "example/Interface", Member: "ARRAY", Description: "[Ljava/lang/Object;", JavaType: arrayType}
	second := &values.JavaClassMember{Name: "example/Interface", Member: "VIEW", Description: "[Ljava/lang/Object;", JavaType: arrayType}
	storeFirst := statements.NewAssignStatement(first, local, false)
	storeFirst.OriginPC = 4
	storeFirst.HasOriginPC = true
	storeSecond := statements.NewAssignStatement(second, first, false)
	storeSecond.OriginPC = 8
	storeSecond.HasOriginPC = true
	ret := statements.NewReturnStatement(nil)
	return []statements.Statement{assignment, storeElement, storeFirst, storeSecond, ret}, []interfaceFieldWrite{{"ARRAY", first.Description, 4, first}, {"VIEW", second.Description, 8, second}}, []string{"ARRAY", "VIEW"}
}

func TestInterfaceInitializerArrayTemporaryRequiresClosedAdjacentFixedType(t *testing.T) {
	fixture := func() (*interfaceInitializerSegment, *CodeAttribute, *values.NewExpression, *values.JavaRef) {
		typ := types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger))
		local := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
		array := &values.NewExpression{JavaType: typ, HasOriginPC: true, OriginPC: 1, HasEvaluationEndPC: true, EvaluationEndPC: 5, Initializer: []values.JavaValue{values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger))}}
		return &interfaceInitializerSegment{write: interfaceFieldWrite{pc: 6}, prefix: []statements.Statement{statements.NewAssignStatement(local, array, false)}, value: local}, &CodeAttribute{Code: make([]byte, 10)}, array, local
	}
	cases := []struct {
		name   string
		change func(*interfaceInitializerSegment, *CodeAttribute, *values.NewExpression, *values.JavaRef)
	}{
		{"intervening effect", func(p *interfaceInitializerSegment, _ *CodeAttribute, _ *values.NewExpression, _ *values.JavaRef) {
			p.prefix = append(p.prefix, statements.NewExpressionStatement(values.JavaNull))
		}},
		{"different use", func(p *interfaceInitializerSegment, _ *CodeAttribute, a *values.NewExpression, _ *values.JavaRef) {
			p.value = values.NewJavaRef(utils.NewRootVariableId(), nil, a.Type())
		}},
		{"self reference", func(_ *interfaceInitializerSegment, _ *CodeAttribute, a *values.NewExpression, r *values.JavaRef) {
			a.Initializer[0] = r
		}},
		{"unbound dependency", func(_ *interfaceInitializerSegment, _ *CodeAttribute, a *values.NewExpression, _ *values.JavaRef) {
			a.Initializer[0] = values.NewJavaRef(utils.NewRootVariableId(), nil, a.Type())
		}},
		{"different assignment type", func(_ *interfaceInitializerSegment, _ *CodeAttribute, _ *values.NewExpression, r *values.JavaRef) {
			r.ResetVarType(types.NewJavaArrayType(types.NewJavaPrimer(types.JavaLong)))
		}},
		{"unknown allocation origin", func(_ *interfaceInitializerSegment, _ *CodeAttribute, a *values.NewExpression, _ *values.JavaRef) {
			a.HasOriginPC = false
		}},
		{"incomplete element witness", func(_ *interfaceInitializerSegment, _ *CodeAttribute, a *values.NewExpression, _ *values.JavaRef) {
			a.HasEvaluationEndPC = false
		}},
		{"elements after field store", func(p *interfaceInitializerSegment, _ *CodeAttribute, a *values.NewExpression, _ *values.JavaRef) {
			a.EvaluationEndPC = p.write.pc
		}},
		{"allocation after field store", func(p *interfaceInitializerSegment, _ *CodeAttribute, a *values.NewExpression, _ *values.JavaRef) {
			a.OriginPC = p.write.pc
		}},
		{"handler boundary", func(_ *interfaceInitializerSegment, c *CodeAttribute, _ *values.NewExpression, _ *values.JavaRef) {
			c.ExceptionTable = []*ExceptionTableEntry{{StartPc: 0, EndPc: 5, HandlerPc: 8}}
		}},
		{"opaque initializer", func(_ *interfaceInitializerSegment, _ *CodeAttribute, a *values.NewExpression, _ *values.JavaRef) {
			a.Initializer[0] = &values.CustomValue{}
		}},
		{"opaque allocation rendering", func(_ *interfaceInitializerSegment, _ *CodeAttribute, a *values.NewExpression, _ *values.JavaRef) {
			a.ArgumentsGetter = func() string { return "" }
		}},
	}
	p, code, array, _ := fixture()
	if result, proved := interfaceInitializerArrayExpression(p, code); !proved || result != array || len(p.prefix) != 1 {
		t.Fatal("closed adjacent array must retain its allocation tree without mutating the plan")
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			p, code, array, local := fixture()
			tt.change(p, code, array, local)
			if _, proved := interfaceInitializerArrayExpression(p, code); proved {
				t.Fatal("unproved temporary must remain in its original initializer segment")
			}
		})
	}
}

func TestInterfaceWrappedArrayRequiresSingleOrderedClosedUse(t *testing.T) {
	for _, variant := range []string{"control", "later effect", "earlier effect", "duplicate", "conditional", "instance call", "missing origin", "consumer before elements", "outer before inner", "field before consumer", "receiver effect", "unbound operand", "opaque operand", "cycle", "too deep"} {
		t.Run(variant, func(t *testing.T) {
			typ := types.NewJavaArrayType(types.NewJavaPrimer(types.JavaInteger))
			local := values.NewJavaRef(utils.NewRootVariableId(), nil, typ)
			array := &values.NewExpression{JavaType: typ, HasOriginPC: true, OriginPC: 1, HasEvaluationEndPC: true, EvaluationEndPC: 5, Initializer: []values.JavaValue{values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger))}}
			call := func(pc int, args ...values.JavaValue) *values.FunctionCallExpression {
				return &values.FunctionCallExpression{Kind: values.InvokeStatic, IsStatic: true, HasOriginPC: true, OriginPC: pc, ClassName: "example.Factories", FunctionName: "wrap", Descriptor: "([I)Ljava/lang/Object;", FuncType: &types.JavaFuncType{ReturnType: types.NewJavaClass("java.lang.Object")}, Arguments: args}
			}
			inner := call(6, local)
			cast := &values.CastExpression{Value: inner, TargetType: types.NewJavaClass("java.lang.Object")}
			outer := call(8, values.NewJavaLiteral(7, types.NewJavaPrimer(types.JavaInteger)), cast)
			plan := &interfaceInitializerSegment{write: interfaceFieldWrite{pc: 10}, prefix: []statements.Statement{statements.NewAssignStatement(local, array, false)}, value: outer}
			switch variant {
			case "later effect":
				outer.Arguments = append(outer.Arguments, call(7))
			case "earlier effect":
				outer.Arguments[0] = call(0)
			case "duplicate":
				outer.Arguments = append(outer.Arguments, local)
			case "conditional":
				outer.Arguments[1] = &values.TernaryExpression{Condition: values.NewJavaLiteral(true, types.NewJavaPrimer(types.JavaBoolean)), TrueValue: local, FalseValue: values.JavaNull}
			case "instance call":
				inner.Kind = values.InvokeVirtual
			case "missing origin":
				inner.HasOriginPC = false
			case "consumer before elements":
				inner.OriginPC = array.EvaluationEndPC
			case "outer before inner":
				outer.OriginPC = inner.OriginPC
			case "field before consumer":
				plan.write.pc = outer.OriginPC
			case "receiver effect":
				outer.Object = call(0)
			case "unbound operand":
				outer.Arguments = append(outer.Arguments, values.NewJavaRef(utils.NewRootVariableId(), nil, typ))
			case "opaque operand":
				outer.Arguments = append(outer.Arguments, &values.CustomValue{})
			case "cycle":
				inner.Arguments = []values.JavaValue{outer}
			case "too deep":
				for i := 0; i < 40; i++ {
					plan.value = &values.CastExpression{Value: plan.value, TargetType: typ}
				}
			}
			result, proved := interfaceInitializerArrayExpression(plan, &CodeAttribute{Code: make([]byte, 12)})
			if variant == "control" || variant == "later effect" {
				if !proved || result == plan.value {
					t.Fatal("closed call path should replace only the private local")
				}
				copy := result.(*values.FunctionCallExpression)
				replaced := copy.Arguments[1].(*values.CastExpression).Value.(*values.FunctionCallExpression)
				if copy.Witness() != outer.Witness() || replaced.Witness() != inner.Witness() || replaced.Arguments[0] != array || inner.Arguments[0] != local || plan.value != outer {
					t.Fatal("changed binding, array identity or original expression tree")
				}
			} else if proved {
				t.Fatal("unproved expression cannot borrow allocation movement")
			}
		})
	}
}

func TestInterfaceInitializerPartitionRequiresOriginalStoresAndPrivateLocals(t *testing.T) {
	cases := []struct {
		name   string
		change func([]statements.Statement, []interfaceFieldWrite, []string) ([]statements.Statement, []interfaceFieldWrite, []string)
	}{
		{"proved", func(b []statements.Statement, w []interfaceFieldWrite, o []string) ([]statements.Statement, []interfaceFieldWrite, []string) {
			return b, w, o
		}},
		{"wrong store PC", func(b []statements.Statement, w []interfaceFieldWrite, o []string) ([]statements.Statement, []interfaceFieldWrite, []string) {
			w[0].pc++
			return b, w, o
		}},
		{"unknown store PC", func(b []statements.Statement, w []interfaceFieldWrite, o []string) ([]statements.Statement, []interfaceFieldWrite, []string) {
			b[2].(*statements.AssignStatement).HasOriginPC = false
			return b, w, o
		}},
		{"different descriptor", func(b []statements.Statement, w []interfaceFieldWrite, o []string) ([]statements.Statement, []interfaceFieldWrite, []string) {
			w[0].descriptor = "Ljava/lang/Object;"
			return b, w, o
		}},
		{"different owner", func(b []statements.Statement, w []interfaceFieldWrite, o []string) ([]statements.Statement, []interfaceFieldWrite, []string) {
			b[2].(*statements.AssignStatement).LeftValue.(*values.JavaClassMember).Name = "example/Other"
			return b, w, o
		}},
		{"repeated field", func(b []statements.Statement, w []interfaceFieldWrite, o []string) ([]statements.Statement, []interfaceFieldWrite, []string) {
			w[1].name = w[0].name
			o[1] = o[0]
			return b, w, o
		}},
		{"different declaration order", func(b []statements.Statement, w []interfaceFieldWrite, o []string) ([]statements.Statement, []interfaceFieldWrite, []string) {
			o[0], o[1] = o[1], o[0]
			return b, w, o
		}},
		{"missing destination", func(b []statements.Statement, w []interfaceFieldWrite, o []string) ([]statements.Statement, []interfaceFieldWrite, []string) {
			return b, w[:1], o[:1]
		}},
		{"cross-segment local", func(b []statements.Statement, w []interfaceFieldWrite, o []string) ([]statements.Statement, []interfaceFieldWrite, []string) {
			b[3].(*statements.AssignStatement).JavaValue = b[2].(*statements.AssignStatement).JavaValue
			return b, w, o
		}},
		{"opaque operand", func(b []statements.Statement, w []interfaceFieldWrite, o []string) ([]statements.Statement, []interfaceFieldWrite, []string) {
			b[2].(*statements.AssignStatement).JavaValue = &values.CustomValue{}
			return b, w, o
		}},
		{"effects after final write", func(b []statements.Statement, w []interfaceFieldWrite, o []string) ([]statements.Statement, []interfaceFieldWrite, []string) {
			b = append(b[:4], append([]statements.Statement{statements.NewExpressionStatement(values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)))}, b[4:]...)...)
			return b, w, o
		}},
		{"unknown control marker", func(b []statements.Statement, w []interfaceFieldWrite, o []string) ([]statements.Statement, []interfaceFieldWrite, []string) {
			return append([]statements.Statement{&statements.MiddleStatement{Flag: "unknown"}}, b...), w, o
		}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			body, writes, order := interfaceInitializerFixture()
			body, writes, order = tt.change(body, writes, order)
			plans, err := planInterfaceInitializers(body, "example/Interface", writes, order)
			if (err == nil) != (tt.name == "proved") {
				t.Fatalf("plans=%d err=%v", len(plans), err)
			}
			if err == nil && (len(plans) != 2 || len(plans[0].prefix) != 2 || len(plans[1].prefix) != 0) {
				t.Fatal("must retain each original prefix once")
			}
		})
	}
}

func TestAdversarialAnnotationInitializerRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlowUnits(t, "AnnotationInitReview", `
import java.util.*;
@interface ReviewedInitAnnotation {List<String> VALUES=Collections.unmodifiableList(Arrays.asList(new String[]{"first","second"}));String value()default "kept";}
public class AnnotationInitReview {public static void main(String[]args){System.out.println(ReviewedInitAnnotation.VALUES);System.out.println(ReviewedInitAnnotation.VALUES==ReviewedInitAnnotation.VALUES);try{ReviewedInitAnnotation.VALUES.add("x");}catch(Throwable e){System.out.println(e.getClass().getName());}}}
`, nil, []string{"ReviewedInitAnnotation"}, Precision, Compatibility, "legacy")
}

func TestAdversarialAnnotationInitializerDollarNameCollisionRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlowUnits(t, "AnnotationDollarCollisionReview", `
import java.util.*;
@interface ReviewedDollarAnnotation {List<String> VALUES=Collections.unmodifiableList(Arrays.asList(new String[]{"kept","once"}));class jdec$init$0 {public int marker(){return 37;}}}
class OriginalDollarProbe {static int marker(){return new ReviewedDollarAnnotation.jdec$init$0().marker();}}
public class AnnotationDollarCollisionReview {public static void main(String[]args){System.out.println(ReviewedDollarAnnotation.VALUES);System.out.println(ReviewedDollarAnnotation.VALUES==ReviewedDollarAnnotation.VALUES);System.out.println(OriginalDollarProbe.marker());}}
`, nil, []string{"ReviewedDollarAnnotation"}, Precision, Compatibility, "legacy")
}

func TestAdversarialInterfaceInitializerHelperCollisionRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlowUnits(t, "InterfaceCollisionReview", `
interface CollisionTable { Object VALUE=new Object[]{"once",Integer.valueOf(7)};int jdec$init$1=3;static int jdec$init$0(){return 29;}class jdec$init$2 {} }
public class InterfaceCollisionReview {public static void main(String[]args){System.out.println(java.util.Arrays.toString((Object[])CollisionTable.VALUE));System.out.println(CollisionTable.VALUE==CollisionTable.VALUE);System.out.println(CollisionTable.jdec$init$0()+CollisionTable.jdec$init$1);}}
`, nil, []string{"CollisionTable"}, Precision, Compatibility, "legacy")
}

func TestInterfaceInitializerStoreCoverageAndFallbackOwnership(t *testing.T) {
	javac, _ := t04Tools(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "InterfaceWitness.java")
	if err := os.WriteFile(file, []byte(`interface InterfaceWitness {Object VALUE=new Object();Object UNUSED=null;}`), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(javac, "-proc:none", "--release", "8", "-g:none", "-d", dir, file).CombinedOutput(); err != nil {
		t.Fatalf("compile witness: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "InterfaceWitness.class"))
	if err != nil {
		t.Fatal(err)
	}
	object, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	dumper := NewClassObjectDumper(object)
	dumper.FuncCtx = &class_context.ClassContext{}
	dumper.report = &DecompileResult{}
	var code *CodeAttribute
	for _, method := range object.Methods {
		name, _ := object.getUtf8(method.NameIndex)
		if name == "<clinit>" {
			for _, attribute := range method.Attributes {
				if attr, ok := attribute.(*CodeAttribute); ok {
					code = attr
				}
			}
		}
	}
	writes, err := dumper.interfaceWrites(code)
	if err != nil || len(writes) != 2 {
		t.Fatalf("original own stores: %v %v", writes, err)
	}
	pc := uint16(writes[0].pc)
	for _, tt := range []struct {
		name  string
		entry *ExceptionTableEntry
		want  bool
	}{
		{"protected store", &ExceptionTableEntry{StartPc: pc, EndPc: pc + 3}, false},
		{"store on exclusive end", &ExceptionTableEntry{StartPc: 0, EndPc: pc}, true},
		{"malformed coverage", &ExceptionTableEntry{StartPc: pc, EndPc: pc}, false},
		{"missing entry", nil, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			code.ExceptionTable = []*ExceptionTableEntry{tt.entry}
			_, err := dumper.interfaceWrites(code)
			if (err == nil) != tt.want {
				t.Fatalf("coverage proof err=%v", err)
			}
		})
	}
	code.ExceptionTable = nil
	// Mutate only the first original Fieldref owner, retaining the exact
	// name and descriptor: fallback must not attribute that foreign store
	// to this interface merely because the member name matches.
	firstIndex := int(code.Code[writes[0].pc+1])<<8 | int(code.Code[writes[0].pc+2])
	first, ok := object.ConstantPool[firstIndex-1].(*ConstantFieldrefInfo)
	if !ok {
		t.Fatal("expected first destination Fieldref")
	}
	ownerIndex := len(object.ConstantPool) + 1
	object.ConstantPool = append(object.ConstantPool, &ConstantUtf8Info{Value: "ForeignWitness"}, &ConstantClassInfo{NameIndex: uint16(ownerIndex)})
	first.ClassIndex = uint16(ownerIndex + 1)
	dumper.ConstantPool = object.ConstantPool
	if _, err := dumper.interfaceWrites(code); err == nil {
		t.Fatal("foreign write cannot be partitioned as an own initializer")
	}
	helpers := dumper.interfaceInitializerStubs(fmt.Errorf("foreign initializer effects"))
	if len(helpers) != 1 || dumper.fieldDefaultValue["VALUE"] != "" || !strings.Contains(dumper.fieldDefaultValue["UNUSED"], "jdec$init$") {
		t.Fatalf("fallback ownership: helpers=%d initializers=%v", len(helpers), dumper.fieldDefaultValue)
	}
	if len(dumper.report.StubMethods) != 1 || len(dumper.report.Diagnostics) != 1 || !strings.Contains(helpers[0].code, "unproven interface initializer") {
		t.Fatal("fallback must explicitly report incomplete semantics")
	}
}
