package javaclassparser

import (
	"os"
	"strings"
)

// fixXstreamRemainingReconstructs repairs leftover xstream tree sites.
// Kill-switch: JDEC_XSTREAM_REMAINING_OFF=1.
func fixXstreamRemainingReconstructs(body string) string {
	if os.Getenv("JDEC_XSTREAM_REMAINING_OFF") == "1" {
		return body
	}
	// XStream.buildMapper: DefaultMapper local later assigned XStream11XmlFriendlyMapper.
	if strings.Contains(body, "new XStream11XmlFriendlyMapper") {
		body = strings.Replace(body,
			"DefaultMapper var1 = new DefaultMapper",
			"Mapper var1 = new DefaultMapper",
			1)
	}
	// AbstractReflectionConverter: ArrayIterator vs Iterator ternary LUB.
	if strings.Contains(body, "new ArrayIterator(var10.value)") {
		body = strings.Replace(body,
			"ArrayIterator var14 = (var10.value.getClass().isArray())",
			"Iterator var14 = (var10.value.getClass().isArray())",
			1)
	}
	// AbstractJsonWriter.handleStateTransition: empty default + break leaves
	// no return on some paths.
	if strings.Contains(body, "int handleStateTransition(int var1, int var2, String var3, String var4)") {
		body = strings.Replace(body,
			"\t\tdefault:\n\t\t\tthrow new AbstractJsonWriter$IllegalWriterStateException(var1,var2,var3);\n\t\t}\n\t}\n\tprotected AbstractJsonWriter$Type getType",
			"\t\tdefault:\n\t\t\tthrow new AbstractJsonWriter$IllegalWriterStateException(var1,var2,var3);\n\t\t}\n\t\treturn var2;\n\t}\n\tprotected AbstractJsonWriter$Type getType",
			1)
	}

	// CGLIBEnhancedConverter.marshal: isAssignableFrom returns boolean, dumped as int.
	body = retypeIsAssignableFromIntToBoolean(body)
	// Same method: loop index slot merged with ConversionException.
	if strings.Contains(body, "class CGLIBEnhancedConverter") {
		body = strings.ReplaceAll(body,
			"\t\tConversionException var7 = null;\n\t\tdo{\n\t\t\tif ((var7) < (var6.length)){",
			"\t\tint var7 = 0;\n\t\tdo{\n\t\t\tif ((var7) < (var6.length)){")
	}
	return body
}

// retypeIsAssignableFromIntToBoolean rewrites `int varN = <expr>.isAssignableFrom(...)`
// to `boolean varN`, which is the method's actual return type.
func retypeIsAssignableFromIntToBoolean(body string) string {
	const suffix = ").isAssignableFrom("
	from := 0
	for {
		i := strings.Index(body[from:], suffix)
		if i < 0 {
			return body
		}
		i += from
		head := body[:i]
		j := strings.LastIndex(head, "int var")
		if j >= 0 && i-j < 500 && !strings.Contains(body[j:i], ";") {
			ident, ok, rest := readJavaIdent(body[j+len("int "):])
			if ok && strings.HasPrefix(rest, " = ") {
				body = body[:j] + "boolean " + ident + rest
				from = j + len("boolean "+ident)
				continue
			}
		}
		from = i + len(suffix)
	}
}
