package values

import (
	"github.com/yaklang/javajive/classparser/decompiler/core/values/types"
	"testing"
)

func TestOriginalIntegerTrapRequiresUnchangedProducer(t *testing.T) {
	for _, width := range []string{types.JavaInteger, types.JavaLong} {
		for _, operator := range []string{DIV, REM} {
			for _, change := range []string{"original", "copy", "synthetic", "swapped", "replaced left", "replaced right", "operator", "width", "floating", "missing type", "missing operand", "operand width", "operand float", "operand missing type", "negative PC", "zero PC"} {
				t.Run(width+"/"+operator+"/"+change, func(t *testing.T) {
					typ := types.NewJavaPrimer(width)
					left, right := NewJavaLiteral(17, typ), NewJavaLiteral(3, typ)
					expression := NewOriginalIntegerTrapExpression(left, right, operator, typ, 12)
					switch change {
					case "copy":
						copy := *expression
						expression = &copy
					case "synthetic":
						expression = NewBinaryExpression(left, right, operator, typ)
					case "swapped":
						expression.Values[0], expression.Values[1] = expression.Values[1], expression.Values[0]
					case "replaced left":
						expression.Values[0] = NewJavaLiteral(17, typ)
					case "replaced right":
						expression.Values[1] = NewJavaLiteral(3, typ)
					case "operator":
						expression.Op = ADD
					case "width":
						if width == types.JavaInteger {
							expression.Typ = types.NewJavaPrimer(types.JavaLong)
						} else {
							expression.Typ = types.NewJavaPrimer(types.JavaInteger)
						}
					case "floating":
						expression.Typ = types.NewJavaPrimer(types.JavaDouble)
					case "missing type":
						expression.Typ = nil
					case "operand width":
						if width == types.JavaInteger {
							left.JavaType = types.NewJavaPrimer(types.JavaLong)
						} else {
							left.JavaType = types.NewJavaPrimer(types.JavaInteger)
						}
					case "operand float":
						right.JavaType = types.NewJavaPrimer(types.JavaDouble)
					case "operand missing type":
						left.JavaType = nil
					case "missing operand":
						expression.Values = expression.Values[:1]
					case "negative PC":
						expression = NewOriginalIntegerTrapExpression(left, right, operator, typ, -1)
					case "zero PC":
						expression = NewOriginalIntegerTrapExpression(left, right, operator, typ, 0)
					}
					pc, op, desc, known := expression.OriginalIntegerTrap()
					want := change == "original" || change == "zero PC"
					expectedPC := 12
					if change == "zero PC" {
						expectedPC = 0
					}
					expectedDesc := "I"
					if width == types.JavaLong {
						expectedDesc = "J"
					}
					if known != want || known && (pc != expectedPC || op != operator || desc != expectedDesc) {
						t.Fatalf("origin=%d/%s/%s/%v want=%v", pc, op, desc, known, want)
					}
					if change == "floating" && !expression.HasIntegerTrapOrigin() {
						t.Fatal("damaged witness lost physical integer origin")
					}
				})
			}
		}
	}
}

func TestOriginalIntegerTrapDoesNotInventOriginForInvalidInputs(t *testing.T) {
	typ := types.NewJavaPrimer(types.JavaInteger)
	value := NewJavaLiteral(1, typ)
	for _, expression := range []*JavaExpression{nil, NewOriginalIntegerTrapExpression(nil, value, DIV, typ, 0), NewOriginalIntegerTrapExpression(value, nil, REM, typ, 0), NewOriginalIntegerTrapExpression(value, value, DIV, types.NewJavaPrimer(types.JavaFloat), 0), NewOriginalIntegerTrapExpression(value, value, ADD, typ, 0), NewOriginalIntegerTrapExpression(value, value, DIV, nil, 0), NewOriginalIntegerTrapExpression(value, value, DIV, typ, 65536)} {
		if _, _, _, known := expression.OriginalIntegerTrap(); known {
			t.Fatal("invented trap origin")
		}
	}
}

func TestOriginalIntegerTrapKeepsBinaryNumericPromotion(t *testing.T) {
	for _, width := range []string{types.JavaByte, types.JavaChar, types.JavaShort, types.JavaInteger, types.JavaLong, types.JavaFloat, types.JavaDouble, types.JavaBoolean} {
		t.Run(width, func(t *testing.T) {
			typ := types.NewJavaPrimer(width)
			left := NewJavaLiteral(17, typ)
			if width == types.JavaBoolean {
				left.Data = 1
			}
			expression := NewOriginalIntegerTrapExpression(left, NewJavaLiteral(3, typ), DIV, types.NewJavaPrimer(types.JavaInteger), 4)
			_, _, descriptor, known := expression.OriginalIntegerTrap()
			want := width == types.JavaByte || width == types.JavaChar || width == types.JavaShort || width == types.JavaInteger
			if known != want || known && descriptor != "I" {
				t.Fatalf("operand=%s descriptor=%s known=%v", width, descriptor, known)
			}
		})
	}
}
