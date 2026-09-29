package values

import "strings"

type staticJDKMethod struct {
	owner, name, descriptor string
}

// These JDK declarations use an unbounded method type variable at the marked
// positions (one bit per argument). An erased Object cast there changes source
// inference without representing a bytecode CHECKCAST. Each entry identifies
// the exact declaration, not just a name or arity. The singleton/nCopies
// families have no competing overload; requireNonNull's two-argument overloads
// differ only at argument 1, whose required overload pin is deliberately kept.
//
// Do not generalize this to all generic factories: Stream.of(T) competes with
// Stream.of(T...), for example, and array arguments can select another method.
var staticJDKInferenceFormals = map[staticJDKMethod]uint8{
	{"java.util.Collections", "singleton", "(Ljava/lang/Object;)Ljava/util/Set;"}:                                  1,
	{"java.util.Collections", "singletonList", "(Ljava/lang/Object;)Ljava/util/List;"}:                             1,
	{"java.util.Collections", "singletonMap", "(Ljava/lang/Object;Ljava/lang/Object;)Ljava/util/Map;"}:             3,
	{"java.util.Collections", "nCopies", "(ILjava/lang/Object;)Ljava/util/List;"}:                                  2,
	{"java.util.Objects", "requireNonNull", "(Ljava/lang/Object;)Ljava/lang/Object;"}:                              1,
	{"java.util.Objects", "requireNonNull", "(Ljava/lang/Object;Ljava/lang/String;)Ljava/lang/Object;"}:            1,
	{"java.util.Objects", "requireNonNull", "(Ljava/lang/Object;Ljava/util/function/Supplier;)Ljava/lang/Object;"}: 1,
}

func (f *FunctionCallExpression) jdkStaticInferenceFormal(i int) bool {
	if f == nil || i < 0 || i >= 8 || (!f.IsStatic && f.Kind != InvokeStatic) {
		return false
	}
	key := staticJDKMethod{strings.ReplaceAll(f.ClassName, "/", "."), f.FunctionName, f.Descriptor}
	return staticJDKInferenceFormals[key]&(1<<uint(i)) != 0
}
