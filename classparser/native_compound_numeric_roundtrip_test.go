package javaclassparser

import (
	"fmt"
	"strings"
	"testing"
)

// The trusted driver computes separate arithmetic oracles and observes state
// after failure. A RHS resets fields, so moving the original accessor's read
// before that RHS cannot accidentally pass by using equal initial values.
func nativeCompoundNumericSource() string {
	var owner, writer, driver, effects strings.Builder
	owner.WriteString(`class NumericOwner{`)
	effects.WriteString(`class NumericEffects{static String trace="";static int fail;static final RuntimeException failure=new RuntimeException("original");static NumericOwner receiver(NumericOwner owner){trace+="R";if(fail==1)throw failure;return owner;}static void right(NumericOwner owner){trace+="V";if(fail==2)throw failure;if(owner!=null)owner.resetAll();}`)
	driver.WriteString(`class NumericDriver{public static void main(String[]args){NumericOwner owner=new NumericOwner();NumericOwner.Writer writer=owner.writer();int rows=0;`)
	kinds := []struct {
		suffix, typ, rhs, values string
		wide, fp, bool           bool
	}{
		{"Byte", "byte", "int", "new int[]{-257,-129,-1,0,1,127,128,257}", false, false, false},
		{"Short", "short", "int", "new int[]{-65537,-32769,-1,0,1,32767,32768}", false, false, false},
		{"Char", "char", "int", "new int[]{-65537,-1,0,1,65535,65536}", false, false, false},
		{"Int", "int", "int", "new int[]{Integer.MIN_VALUE,-65,-33,-1,0,1,31,32,63,64,Integer.MAX_VALUE}", false, false, false},
		{"Long", "long", "long", "new long[]{Long.MIN_VALUE,-1,0,1,Long.MAX_VALUE}", true, false, false},
		{"Float", "float", "float", "new float[]{-0.0F,0.0F,-1.0F,1.0F,Float.intBitsToFloat(0x7fc00001),Float.POSITIVE_INFINITY}", false, true, false},
		{"Double", "double", "double", "new double[]{-0.0D,0.0D,-1.0D,1.0D,Double.longBitsToDouble(0x7ff8000000000001L),Double.NEGATIVE_INFINITY}", true, true, false},
		{"Boolean", "boolean", "boolean", "new boolean[]{false,true}", false, false, true},
	}
	for _, kind := range kinds {
		field := "value" + kind.suffix
		volatile := ""
		if kind.wide {
			volatile = "volatile "
		}
		fmt.Fprintf(&owner, "private %s%s %s;void reset%s(%s n){%s=n;}", volatile, kind.typ, field, kind.suffix, kind.typ, field)
		fmt.Fprintf(&writer, "%s get%s(){return %s;}", kind.typ, kind.suffix, field)
		fmt.Fprintf(&effects, "static %s right%s(NumericOwner owner,%s n){right(owner);return n;}", kind.rhs, kind.suffix, kind.rhs)
		operators := []string{"+", "-", "*", "/", "%", "&", "|", "^", "<<", ">>", ">>>"}
		if kind.fp {
			operators = operators[:5]
		}
		if kind.bool {
			operators = operators[5:8]
		}
		for opIndex, op := range operators {
			method := fmt.Sprintf("op%s%d", kind.suffix, opIndex)
			rhs, values := kind.rhs, kind.values
			rightMethod := kind.suffix
			if op == "<<" || op == ">>" || op == ">>>" {
				rhs, values, rightMethod = "int", "new int[]{-65,-33,-1,0,1,31,32,63,64}", "Int"
			}
			fmt.Fprintf(&writer, "%s %s(NumericOwner owner,%s n){return NumericEffects.receiver(owner).%s %s=NumericEffects.right%s(owner,n);}", kind.typ, method, rhs, field, op, rightMethod)
			comparison := "result!=expected||writer.get" + kind.suffix + "()!=expected"
			if kind.typ == "float" {
				comparison = "Float.floatToRawIntBits(result)!=Float.floatToRawIntBits(expected)||Float.floatToRawIntBits(writer.getFloat())!=Float.floatToRawIntBits(expected)"
			}
			if kind.typ == "double" {
				comparison = "Double.doubleToRawLongBits(result)!=Double.doubleToRawLongBits(expected)||Double.doubleToRawLongBits(writer.getDouble())!=Double.doubleToRawLongBits(expected)"
			}
			initial, base := "17", "20"
			if kind.typ == "long" {
				initial, base = "17L", "20L"
			}
			if kind.bool {
				initial, base = "false", "true"
			}
			fmt.Fprintf(&driver, "for(%s n:%s){NumericEffects.fail=0;NumericEffects.trace=\"\";owner.reset%s((%s)%s);", rhs, values, kind.suffix, kind.typ, initial)
			if !kind.fp && !kind.bool && (op == "/" || op == "%") {
				fmt.Fprintf(&driver, "if(n==0){try{writer.%s(owner,n);throw new AssertionError(\"missing arithmetic failure\");}catch(ArithmeticException expected){}if(writer.get%s()!=20||!NumericEffects.trace.equals(\"RV\"))throw new AssertionError(\"write before divide failure\");rows++;continue;}", method, kind.suffix)
			}
			fmt.Fprintf(&driver, "%s expected=(%s)(%s %s n),result=writer.%s(owner,n);if(%s||!NumericEffects.trace.equals(\"RV\"))throw new AssertionError(\"%s binding/narrowing/width/late read/once\");rows++;}", kind.typ, kind.typ, base, op, method, comparison, method)
			fmt.Fprintf(&driver, "for(int fail:new int[]{0,1,2}){NumericEffects.fail=fail;NumericEffects.trace=\"\";try{writer.%s(null,(%s)%s);throw new AssertionError(\"missing failure\");}catch(RuntimeException e){if(fail==0?!(e instanceof NullPointerException):e!=NumericEffects.failure)throw new AssertionError(\"failure identity\");}if(!NumericEffects.trace.equals(fail==1?\"R\":\"RV\"))throw new AssertionError(\"null after RHS\");}", method, rhs, base)
		}
		if !kind.bool {
			fmt.Fprintf(&writer, "%s pre%s(NumericOwner owner){return ++NumericEffects.receiver(owner).%s;}%s post%s(NumericOwner owner){return NumericEffects.receiver(owner).%s--;}", kind.typ, kind.suffix, field, kind.typ, kind.suffix, field)
			before := "(" + kind.typ + ")n"
			equals := func(x, y string) string {
				if kind.typ == "float" {
					return "Float.floatToRawIntBits(" + x + ")!=Float.floatToRawIntBits(" + y + ")"
				}
				if kind.typ == "double" {
					return "Double.doubleToRawLongBits(" + x + ")!=Double.doubleToRawLongBits(" + y + ")"
				}
				return x + "!=" + y
			}
			fmt.Fprintf(&driver, "for(%s n:%s){NumericEffects.fail=0;NumericEffects.trace=\"\";%s before=%s,expected=(%s)(before+1);owner.reset%s(before);%s result=writer.pre%s(owner);if(%s||%s||!NumericEffects.trace.equals(\"R\"))throw new AssertionError(\"pre width/overflow\");owner.reset%s(before);NumericEffects.trace=\"\";result=writer.post%s(owner);expected=(%s)(before-1);if(%s||%s||!NumericEffects.trace.equals(\"R\"))throw new AssertionError(\"post old/new width/overflow\");rows++;}", kind.rhs, kind.values, kind.typ, before, kind.typ, kind.suffix, kind.typ, kind.suffix, equals("result", "expected"), equals("writer.get"+kind.suffix+"()", "expected"), kind.suffix, kind.suffix, kind.typ, equals("result", "before"), equals("writer.get"+kind.suffix+"()", "expected"))
		}
	}
	owner.WriteString("void resetAll(){")
	for _, kind := range kinds {
		value := "(" + kind.typ + ")20"
		if kind.bool {
			value = "true"
		}
		fmt.Fprintf(&owner, "value%s=%s;", kind.suffix, value)
	}
	owner.WriteString("}class Writer{")
	owner.WriteString(writer.String())
	owner.WriteString("}Writer writer(){return new Writer();}}")
	effects.WriteString("}")
	driver.WriteString(`System.out.println("numeric:compound:width:narrow:overflow:binding:order:identity");}}`)
	return effects.String() + owner.String() + driver.String()
}

func TestNativeCompoundNumericRoundTrip(t *testing.T) {
	testNativePrivateSetterFixture(t, nativeCompoundNumericSource(), "NumericOwner", "NumericDriver", "numeric:compound:width:narrow:overflow:binding:order:identity\n")
}
func TestNativeCompoundNumericRenamedRoundTrip(t *testing.T) {
	source := strings.ReplaceAll(nativeCompoundNumericSource(), "NumericOwner", "OtherArithmeticScope")
	testNativePrivateSetterFixture(t, source, "OtherArithmeticScope", "NumericDriver", "numeric:compound:width:narrow:overflow:binding:order:identity\n")
}
