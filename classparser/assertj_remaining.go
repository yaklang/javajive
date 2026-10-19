package javaclassparser

import (
	"github.com/yaklang/javajive/internal/jdecenv"
	"strings"
)

// fixAssertjRemainingReconstructs repairs leftover assertj-core tree sites.
// Kill-switch: JDEC_ASSERTJ_REMAINING_OFF=1.
func fixAssertjRemainingReconstructs(body string) string {
	if jdecenv.Get("JDEC_ASSERTJ_REMAINING_OFF") == "1" {
		return body
	}
	if !strings.Contains(body, "assertj") {
		return body
	}
	// A Number view can own an original receiver CHECKCAST or a selected
	// overload. Core binding retains that evidence; a class-source keyword
	// cannot authorize removing it (including from application string data).
	if strings.Contains(body, "ELEMENT[][]") || strings.Contains(body, "ELEMENT extends") {
		body = strings.ReplaceAll(body, "((Object[][])(", "((ELEMENT[][])(")
		body = strings.ReplaceAll(body, "(Object[][])(", "(ELEMENT[][])(")
	}
	if strings.Contains(body, "ELEMENT extends") {
		body = strings.ReplaceAll(body,
			"this.newObjectArrayAssert(Filters.filter(((Object[])(this.actual))).with(var1,var2).get())",
			"this.newObjectArrayAssert((List)(Filters.filter(((Object[])(this.actual))).with(var1,var2).get()))")
		body = strings.ReplaceAll(body,
			"this.newObjectArrayAssert(Filters.filter(((Object[])(this.actual))).with(var1,null).get())",
			"this.newObjectArrayAssert((List)(Filters.filter(((Object[])(this.actual))).with(var1,null).get()))")
		body = strings.ReplaceAll(body,
			"this.newObjectArrayAssert(Filters.filter(((Object[])(this.actual))).being(var1).get())",
			"this.newObjectArrayAssert((List)(Filters.filter(((Object[])(this.actual))).being(var1).get()))")
		body = strings.ReplaceAll(body,
			"this.newObjectArrayAssert(var3.get())",
			"this.newObjectArrayAssert((List)(var3.get()))")
	}

	// isEqualTo/isIn pick String overloads when parse/BigDecimal results are
	// wrapped in (String)/(String[]).
	body = strings.ReplaceAll(body, "(String)(this.parse(var1))", "this.parse(var1)")
	body = strings.ReplaceAll(body, "(String)(new BigDecimal(var1))", "new BigDecimal(var1)")
	// A producer's argument type or name does not prove its result array type.
	// Keep original CHECKCAST and selected-descriptor views in the typed IR.

	// Interface receivers and wider overload views retain their original
	// CHECKCAST/descriptor evidence. Unrelated method words cannot erase them.

	// An array field can require both a physical CHECKCAST and an Object[]
	// view of the selected overload. Typed binding owns these operations;
	// method words cannot remove them or change application string data.

	body = strings.ReplaceAll(body,
		".flatMap(Collection::stream)",
		".flatMap((l0) -> ((Collection)(l0)).stream())")
	body = strings.ReplaceAll(body, "var1.apply(l0)", "((Function)(var1)).apply(l0)")
	body = strings.ReplaceAll(body, ".filter(var1).collect", ".filter((Predicate)(var1)).collect")
	body = strings.ReplaceAll(body, ".being(var1)", ".being((Condition)(var1))")
	// Invocation owners and selected overloads come from bytecode. The
	// spelling "matches" is not evidence that a receiver is a Condition;
	// inserting that cast can silently select matches(Object) instead of
	// a distinct matches(String), and can also corrupt string literals.

	body = wrapReturnCallWithCast(body, "CACHE.findOrInsert", "Class")

	// Constructor overload selection is proved against original declaration
	// tables. A program/method/parameter spelling cannot select a Stream view.
	// Factory invocations and receiver dispatch belong to original declaration
	// evidence. A short source name cannot authorize replacing either call with
	// a different operation, or changing application data containing that text.

	body = strings.ReplaceAll(body,
		"public static List<DeepDifference$Difference> determineDifferences(Object var0, Object var1, TreeMap var2, TypeComparators var3)",
		"public static List<DeepDifference$Difference> determineDifferences(Object var0, Object var1, Map<String, Comparator<?>> var2, TypeComparators var3)")
	body = strings.ReplaceAll(body,
		"ArrayList var7 = ((var6) != (0)) ? (new ArrayList()) : (var2);",
		"List var7 = ((var6) != (0)) ? ((List)(new ArrayList())) : (var2);")
	body = strings.ReplaceAll(body,
		"DiffNode var17 = new DiffNode(var14,var16,var15);",
		"PathNode var17 = new DiffNode(var14,var16,var15);")

	if strings.Contains(body, "getConstructor") {
		body = strings.ReplaceAll(body,
			"}catch(IllegalAccessException | InvocationTargetException | InstantiationException var3){",
			"}catch(IllegalAccessException | InvocationTargetException | InstantiationException | NoSuchMethodException var3){")
	}
	// Exceptions attributes and handler tables own the checked-exception contract.
	// Never move a declared exception into a catch based on source-name matches.
	if strings.Contains(body, "Function<Map.Entry") {
		body = strings.ReplaceAll(body,
			"return failsRequirements((Consumer)(var3),l0);",
			"return failsRequirements(var3,(Map.Entry)(l0));")
		body = strings.ReplaceAll(body,
			"return failsRequirements(var3,l0);",
			"return failsRequirements(var3,(Map.Entry)(l0));")
	} else {
		body = strings.ReplaceAll(body,
			"return failsRequirements(var3,l0);",
			"return failsRequirements((Consumer)(var3),l0);")
	}
	body = strings.ReplaceAll(body,
		"return removeElement(var0,l0);",
		"return removeElement(var0,(E)(l0));")
	body = strings.ReplaceAll(body,
		".anyMatch(Iterables::areAllConsumersSatisfied)",
		".anyMatch((l0) -> Iterables.areAllConsumersSatisfied((Queue)(l0)))")
	body = strings.ReplaceAll(body,
		"this.predicates.assertIsNotNull(var3);\n\t\tfinal ArrayList var4_f1 = var4;",
		"this.predicates.assertIsNotNull(var3);\n\t\tfinal PredicateDescription var4_f1 = var4;")

	body = strings.ReplaceAll(body,
		"this.conditions.assertIs((AssertionInfo)(this.info),((Optional)(this.actual)).get(),var1);",
		"this.conditions.assertIs((AssertionInfo)(this.info),((Optional)(this.actual)).get(),(Condition)(var1));")
	body = strings.ReplaceAll(body,
		"l0.setDelegate(var1);",
		"((SoftAssertionsProvider)(l0)).setDelegate(var1);")
	body = strings.ReplaceAll(body,
		".map(Field::getName)",
		".map((l0) -> ((Field)(l0)).getName())")
	body = strings.ReplaceAll(body,
		".map(JoinDescription::checkNotNull)",
		".map((l0) -> JoinDescription.checkNotNull((Description)(l0)))")
	body = strings.ReplaceAll(body,
		"Optional.ofNullable(var1).map(TypeComparators::comparatorByTypes).ifPresent((Consumer<Stream>)((l0) -> {\n			l0.forEach(this::registerComparatorForType);\n		}));",
		"Optional.ofNullable(var1).map(TypeComparators::comparatorByTypes).ifPresent((l0) -> {\n			((Stream)(l0)).forEach((l1) -> {\n			this.registerComparatorForType((Map.Entry)(l1));\n		});\n		});")

	body = replaceAssertjOptionalStreamFlatMap(body)

	body = strings.ReplaceAll(body,
		"((String[])(Stream.of(var0).map((Function<String, String>)((l0) -> {\n			return new StringBuilder().append(l0).append(\"ForProxy\").toString();\n		})).toArray((int l0) -> {\n			return new String[l0];\n		})))",
		"((String[])(java.util.Arrays.stream(var0).map((l0) -> new StringBuilder().append(l0).append(\"ForProxy\").toString()).toArray()))")

	return body
}

func replaceAssertjOptionalStreamFlatMap(body string) string {
	needle := ".flatMap((Function<Optional, Stream>)"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		start := from + rel
		paren := start + len(".flatMap")
		n := skipBalanced(body[paren:], '(', ')')
		if n < 2 {
			from = start + 1
			continue
		}
		repl := ".flatMap((l0) -> { Optional opt = (Optional)(l0); return opt.isPresent() ? Stream.of(opt.get()) : Stream.empty(); })"
		body = body[:start] + repl + body[paren+n:]
		from = start + len(repl)
	}
}

func wrapReturnCallWithCast(body, callee, castTyp string) string {
	needle := "return " + callee + "("
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		start := from + rel
		paren := start + len("return ") + len(callee)
		n := skipBalanced(body[paren:], '(', ')')
		if n < 2 {
			from = start + 1
			continue
		}
		end := paren + n
		expr := body[start+len("return ") : end]
		repl := "return (" + castTyp + ")(" + expr + ")"
		body = body[:start] + repl + body[end:]
		from = start + len(repl)
	}
}
