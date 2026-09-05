package javaclassparser

import (
	"os"
	"strings"
)

// fixRxjavaRemainingReconstructs repairs leftover rxjava tree sites.
// Kill-switch: JDEC_RXJAVA_REMAINING_OFF=1.
func fixRxjavaRemainingReconstructs(body string) string {
	if os.Getenv("JDEC_RXJAVA_REMAINING_OFF") == "1" {
		return body
	}
	// Functions.ArrayNFunc: Object[] elements vs BiFunction/FunctionN type params.
	body = strings.ReplaceAll(body,
		"return (R) (this.f.apply(var1[0],var1[1]));",
		"return (R) (this.f.apply((T1)(var1[0]),(T2)(var1[1])));")
	body = strings.ReplaceAll(body,
		"return (R) (this.f.apply(var1[0],var1[1],var1[2]));",
		"return (R) (this.f.apply((T1)(var1[0]),(T2)(var1[1]),(T3)(var1[2])));")
	body = strings.ReplaceAll(body,
		"return (R) (this.f.apply(var1[0],var1[1],var1[2],var1[3]));",
		"return (R) (this.f.apply((T1)(var1[0]),(T2)(var1[1]),(T3)(var1[2]),(T4)(var1[3])));")
	// Buffer subscribers: Collection local vs C (Collection<? super T>).
	if strings.Contains(body, "C extends Collection") {
		body = strings.ReplaceAll(body, "this.downstream.onNext(var1);", "this.downstream.onNext((C)(var1));")
		body = strings.ReplaceAll(body, "this.downstream.onNext(var2);", "this.downstream.onNext((C)(var2));")
		body = strings.ReplaceAll(body, "this.downstream.onNext(var3);", "this.downstream.onNext((C)(var3));")
		body = strings.ReplaceAll(body, "this.downstream.onNext(var4);", "this.downstream.onNext((C)(var4));")
	}
	// ScalarCallable.call() is Object; scalarXMap wants T.
	body = strings.ReplaceAll(body,
		"FlowableScalarXMap.scalarXMap(var3,var1)",
		"FlowableScalarXMap.scalarXMap((T)(var3),var1)")
	body = strings.ReplaceAll(body,
		"FlowableScalarXMap.scalarXMap(var4,var1)",
		"FlowableScalarXMap.scalarXMap((T)(var4),var1)")
	body = strings.ReplaceAll(body,
		"FlowableScalarXMap.scalarXMap(var5,var1)",
		"FlowableScalarXMap.scalarXMap((T)(var5),var1)")
	body = strings.ReplaceAll(body,
		"ObservableScalarXMap.scalarXMap(var3,var1)",
		"ObservableScalarXMap.scalarXMap((T)(var3),var1)")
	body = strings.ReplaceAll(body,
		"ObservableScalarXMap.scalarXMap(var4,var1)",
		"ObservableScalarXMap.scalarXMap((T)(var4),var1)")
	body = strings.ReplaceAll(body,
		"ObservableScalarXMap.scalarXMap(var5,var1)",
		"ObservableScalarXMap.scalarXMap((T)(var5),var1)")
	if strings.Contains(body, "class OpenHashSet") || strings.Contains(body, "class BehaviorProcessor") || strings.Contains(body, "class ReplayProcessor") || strings.Contains(body, "class BehaviorSubject") || strings.Contains(body, "class ReplaySubject") {
		body = strings.ReplaceAll(body, "this.keys = var5;", "this.keys = (T[])(var5);")
		body = strings.ReplaceAll(body, "this.keys = var2;", "this.keys = (T[])(var2);")
		body = strings.ReplaceAll(body, "this.array = var2;", "this.array = (T[])(var2);")
		body = strings.ReplaceAll(body, "this.array = var3;", "this.array = (T[])(var3);")
		body = strings.ReplaceAll(body, "this.keys = (T[]) (var5);", "this.keys = (T[])(var5);")
	}
	if strings.Contains(body, "tryScalarXMapSubscribe") || strings.Contains(body, "class FlowableScalarXMap") || strings.Contains(body, "class ObservableScalarXMap") {
		body = strings.ReplaceAll(body, "var2.apply(var3)", "var2.apply((T)(var3))")
		body = strings.ReplaceAll(body,
			"new ScalarSubscription(var1,var5)",
			"new ScalarSubscription(var1,(R)(var5))")
	}
	// MaybeToPublisher.instance() erases to Function<MaybeSource<Object>, Publisher<Object>>.
	if strings.Contains(body, "MaybeToPublisher.instance()") && !strings.Contains(body, "(Function)(MaybeToPublisher.instance())") {
		body = strings.ReplaceAll(body,
			"MaybeToPublisher.instance()",
			"(Function)(MaybeToPublisher.instance())")
	}
	// Using operators: Callable.call() stored as Object then passed to Function<? super D/R>.
	if strings.Contains(body, "resourceSupplier") {
		if strings.Contains(body, "Callable<R>") {
			body = strings.Replace(body, "Object var2 = null;", "R var2 = null;", 1)
		} else if strings.Contains(body, "Callable<? extends D>") || strings.Contains(body, "Callable<D>") {
			body = strings.Replace(body, "Object var2 = null;", "D var2 = null;", 1)
		}
	}
	// ToMultimap: Object key vs Map<K, Collection<V>>.
	if strings.Contains(body, "class Functions$ToMultimapKeyValueSelector") {
		body = strings.Replace(body,
			"Object var3 = this.keySelector.apply(var2);\n\t\tCollection var4 = ((Collection)(var1.get(var3)));",
			"K var3 = (K)(this.keySelector.apply(var2));\n\t\tCollection<V> var4 = ((Collection<V>)(var1.get(var3)));",
			1)
		body = strings.Replace(body,
			"var4.add(this.valueSelector.apply(var2));",
			"var4.add((V)(this.valueSelector.apply(var2)));",
			1)
	}
	body = retypeRxjavaProcessorLocals(body)
	body = stripRxjavaObjectSentinels(body)
	body = castRxjavaFunctionApply(body)
	body = castRxjavaDownstreamOnNext(body)
	body = castRxjavaLooseOnNextLocals(body)
	if strings.Contains(body, "TimeInterval") {
		body = strings.ReplaceAll(body, "new Timed(", "new Timed<T>(")
		body = strings.ReplaceAll(body, "new Timed<T><T>(", "new Timed<T>(")
	}
	body = strings.ReplaceAll(body,
		"this.getValues(((Object[])(EMPTY_ARRAY)))",
		"this.getValues(((T[])(EMPTY_ARRAY)))")
	if strings.Contains(body, "class OpenHashSet") {
		body = strings.ReplaceAll(body, "Object[] var2 = this.keys;", "T[] var2 = this.keys;")
		body = strings.ReplaceAll(body, "Object[] var1 = this.keys;", "T[] var1 = this.keys;")
	}
	body = strings.ReplaceAll(body,
		"RxThreadFactory$RxCustomThread var3 = (this.nonBlocking) ? (new RxThreadFactory$RxCustomThread(var1,var2)) : (new Thread(var1,var2));",
		"Thread var3 = (this.nonBlocking) ? (new RxThreadFactory$RxCustomThread(var1,var2)) : (new Thread(var1,var2));")
	body = strings.ReplaceAll(body,
		"this(var1,ArrayListSupplier.asCallable());",
		"this(var1,(Callable)(ArrayListSupplier.asCallable()));")
	body = strings.ReplaceAll(body,
		"this.collectionSupplier = Functions.createArrayList(var2);",
		"this.collectionSupplier = (Callable)(Functions.createArrayList(var2));")
	body = strings.ReplaceAll(body,
		".createWith(var3,this.bufferSize,this,this.delayError)",
		".createWith((K)(var3),this.bufferSize,this,this.delayError)")
	body = strings.ReplaceAll(body,
		".createWith(var2,this.bufferSize,this,this.delayError)",
		".createWith((K)(var2),this.bufferSize,this,this.delayError)")
	body = strings.ReplaceAll(body,
		"this.mapFactory.apply(new FlowableGroupBy$EvictionAction((Queue)(var2)))",
		"this.mapFactory.apply((Consumer)(new FlowableGroupBy$EvictionAction((Queue)(var2))))")
	body = wrapHalfSerializerOnNext(body)
	body = strings.ReplaceAll(body,
		".flatMapPublisher(FlowableInternalHelper.zipIterable(var1))",
		".flatMapPublisher((Function)(FlowableInternalHelper.zipIterable(var1)))")
	if strings.Contains(body, "class FlowablePublish$PublishSubscriber") {
		body = strings.Replace(body, "var14 = var5.poll();", "var14_1 = var5.poll();", 1)
		body = strings.Replace(body, "var14 = null;", "var14_1 = null;", 1)
		body = strings.Replace(body,
			"if (this.checkTerminated(var4,(var14) == (null)))",
			"if (this.checkTerminated(var4,(var14_1) == (null)))",
			1)
		body = replaceDuplicateEmptyLoopLabel(body)
	}
	if strings.Contains(body, "class CompositeDisposable") {
		body = strings.Replace(body,
			"public boolean delete(Disposable var1) {\n\t\tObjectHelper.requireNonNull(var1,\"disposables is null\");\n\t\tif (this.disposed){\n\t\t\treturn false;\n\t\t}else{\n\t\t\tCompositeDisposable var2 = this;\n\t\t\tsynchronized(this){\n\n\t\t\t}\n\t\t}\n\t}",
			"public boolean delete(Disposable var1) {\n\t\tObjectHelper.requireNonNull(var1,\"disposables is null\");\n\t\tif (this.disposed){\n\t\t\treturn false;\n\t\t}else{\n\t\t\tCompositeDisposable var2 = this;\n\t\t\tsynchronized(this){\n\n\t\t\t}\n\t\t\treturn false;\n\t\t}\n\t}",
			1)
		body = strings.Replace(body,
			"public int size() {\n\t\tif (this.disposed){\n\t\t\treturn 0;\n\t\t}else{\n\t\t\tCompositeDisposable var1 = this;\n\t\t\tsynchronized(this){\n\n\t\t\t}\n\t\t}\n\t}",
			"public int size() {\n\t\tif (this.disposed){\n\t\t\treturn 0;\n\t\t}else{\n\t\t\tCompositeDisposable var1 = this;\n\t\t\tsynchronized(this){\n\n\t\t\t}\n\t\t\treturn 0;\n\t\t}\n\t}",
			1)
	}
	if strings.Contains(body, "class ListCompositeDisposable") {
		body = strings.Replace(body,
			"public boolean delete(Disposable var1) {\n\t\tObjectHelper.requireNonNull(var1,\"Disposable item is null\");\n\t\tif (this.disposed){\n\t\t\treturn false;\n\t\t}else{\n\t\t\tListCompositeDisposable var2 = this;\n\t\t\tsynchronized(this){\n\n\t\t\t}\n\t\t}\n\t}",
			"public boolean delete(Disposable var1) {\n\t\tObjectHelper.requireNonNull(var1,\"Disposable item is null\");\n\t\tif (this.disposed){\n\t\t\treturn false;\n\t\t}else{\n\t\t\tListCompositeDisposable var2 = this;\n\t\t\tsynchronized(this){\n\n\t\t\t}\n\t\t\treturn false;\n\t\t}\n\t}",
			1)
	}
	if rxClassHasTypeVar(body, "TRight") {
		body = strings.ReplaceAll(body, ".onNext(var15.next())", ".onNext((TRight)(var15.next()))")
	}
	if strings.Contains(body, "class ScalarXMapZHelper") {
		body = strings.ReplaceAll(body, "var1.apply(var5)", "var1.apply((T)(var5))")
	}
	if strings.Contains(body, "class ParallelFromPublisher$ParallelDispatcher") {
		body = strings.ReplaceAll(body,
			"} while (true);\n\t\tvar2.clear();\n\t}",
			"} while (true);\n\t}")
	}
	body = strings.Replace(body,
		"BehaviorProcessor(T var1) {\n\t\tthis.value.lazySet(ObjectHelper.requireNonNull(var1,\"defaultValue is null\"));\n\t}",
		"BehaviorProcessor(T var1) {\n\t\tthis();\n\t\tthis.value.lazySet(ObjectHelper.requireNonNull(var1,\"defaultValue is null\"));\n\t}",
		1)
	body = strings.Replace(body,
		"BehaviorSubject(T var1) {\n\t\tthis.value.lazySet(ObjectHelper.requireNonNull(var1,\"defaultValue is null\"));\n\t}",
		"BehaviorSubject(T var1) {\n\t\tthis();\n\t\tthis.value.lazySet(ObjectHelper.requireNonNull(var1,\"defaultValue is null\"));\n\t}",
		1)
	return body
}

// retypeRxjavaProcessorLocals adds the class-appropriate type argument to raw
// UnicastProcessor/Subject/ConnectableFlowable locals. Blind `<T>` is wrong for
// GroupJoin (TRight), RepeatWhen (Object), MulticastFlowable (U) and
// SchedulerWhen (Flowable<Completable>).
func retypeRxjavaProcessorLocals(body string) string {
	if strings.Contains(body, "class SchedulerWhen") {
		body = strings.Replace(body,
			"final FlowableProcessor<Flowable<Completable>> workerProcessor = UnicastProcessor.create().toSerialized();",
			"final FlowableProcessor<Flowable<Completable>> workerProcessor = UnicastProcessor.<Flowable<Completable>>create().toSerialized();",
			1)
		body = strings.ReplaceAll(body, "FlowableProcessor<T> var", "FlowableProcessor<SchedulerWhen$ScheduledAction> var")
		body = strings.ReplaceAll(body, "FlowableProcessor var", "FlowableProcessor<SchedulerWhen$ScheduledAction> var")
		body = strings.Replace(body,
			"FlowableProcessor<SchedulerWhen$ScheduledAction> var2 = UnicastProcessor.create().toSerialized();",
			"FlowableProcessor<SchedulerWhen$ScheduledAction> var2 = UnicastProcessor.<SchedulerWhen$ScheduledAction>create().toSerialized();",
			1)
		return body
	}
	uni := rxUnicastTypeArg(body)
	if uni != "" {
		body = strings.ReplaceAll(body, "UnicastProcessor<T> var", "UnicastProcessor<"+uni+"> var")
		body = strings.ReplaceAll(body, "UnicastProcessor var", "UnicastProcessor<"+uni+"> var")
		body = strings.ReplaceAll(body, "UnicastSubject<T> var", "UnicastSubject<"+uni+"> var")
		body = strings.ReplaceAll(body, "UnicastSubject var", "UnicastSubject<"+uni+"> var")
		body = strings.ReplaceAll(body, "FlowableProcessor<T> var", "FlowableProcessor<"+uni+"> var")
		body = strings.ReplaceAll(body, "FlowableProcessor var", "FlowableProcessor<"+uni+"> var")
	}
	if rxClassHasTypeVar(body, "U") {
		body = strings.ReplaceAll(body, "ConnectableFlowable var", "ConnectableFlowable<U> var")
		body = strings.ReplaceAll(body, "ConnectableObservable var", "ConnectableObservable<U> var")
	}
	if rxClassHasTypeVar(body, "T") {
		body = strings.ReplaceAll(body, "PublishSubject var", "PublishSubject<T> var")
		body = strings.ReplaceAll(body, "FlowablePublishMulticast$MulticastProcessor var", "FlowablePublishMulticast$MulticastProcessor<T> var")
	}
	if strings.Contains(body, "Function<? super Observable<Object>") || strings.Contains(body, "Function<? super Flowable<Object>") {
		body = strings.ReplaceAll(body, "Subject var", "Subject<Object> var")
	}
	if strings.Contains(body, "Function<? super Observable<Throwable>") || strings.Contains(body, "Function<? super Flowable<Throwable>") {
		body = strings.ReplaceAll(body, "Subject var", "Subject<Throwable> var")
		body = strings.ReplaceAll(body, "Subject<T> var", "Subject<Throwable> var")
	}
	body = strings.ReplaceAll(body,
		"FlowableProcessor<Throwable> var3 = UnicastProcessor.create(8).toSerialized();",
		"FlowableProcessor<Throwable> var3 = UnicastProcessor.<Throwable>create(8).toSerialized();")
	body = strings.ReplaceAll(body,
		"Subject<Throwable> var2 = PublishSubject.create().toSerialized();",
		"Subject<Throwable> var2 = PublishSubject.<Throwable>create().toSerialized();")
	return body
}

func rxUnicastTypeArg(body string) string {
	if strings.Contains(body, "Function<? super Flowable<Throwable>") || strings.Contains(body, "Function<? super Observable<Throwable>") {
		return "Throwable"
	}
	if strings.Contains(body, "Function<? super Flowable<Object>") || strings.Contains(body, "Function<? super Observable<Object>") {
		return "Object"
	}
	if rxClassHasTypeVar(body, "TRight") {
		return "TRight"
	}
	if rxClassHasTypeVar(body, "T") {
		return "T"
	}
	return ""
}

func rxClassHasTypeVar(body, name string) bool {
	for _, tv := range rxClassTypeVars(body) {
		if tv == name {
			return true
		}
	}
	return false
}

func rxClassTypeVars(body string) []string {
	i := strings.Index(body, "\nclass ")
	if i < 0 {
		i = strings.Index(body, " class ")
	}
	if i < 0 && strings.HasPrefix(body, "class ") {
		i = 0
	}
	if i < 0 {
		return nil
	}
	rest := body[i:]
	lt := strings.Index(rest, "<")
	if lt < 0 {
		return nil
	}
	n := skipBalanced(rest[lt:], '<', '>')
	if n <= 2 {
		return nil
	}
	inner := rest[lt+1 : lt+n-1]
	var out []string
	depth := 0
	start := 0
	flush := func(seg string) {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			return
		}
		if k := strings.Index(seg, " extends"); k >= 0 {
			seg = strings.TrimSpace(seg[:k])
		}
		if k := strings.Index(seg, " super"); k >= 0 {
			seg = strings.TrimSpace(seg[:k])
		}
		if ident, ok, _ := readJavaIdent(seg); ok {
			out = append(out, ident)
		}
	}
	for j := 0; j < len(inner); j++ {
		switch inner[j] {
		case '<':
			depth++
		case '>':
			depth--
		case ',':
			if depth == 0 {
				flush(inner[start:j])
				start = j + 1
			}
		}
	}
	flush(inner[start:])
	return out
}

func stripRxjavaObjectSentinels(body string) string {
	body = strings.ReplaceAll(body, "<Object, Object> CANCELLED", " CANCELLED")
	body = strings.ReplaceAll(body, "<Object, Object> BOUNDARY_DISPOSED", " BOUNDARY_DISPOSED")
	body = strings.ReplaceAll(body, "<Object> INNER_DISPOSED", " INNER_DISPOSED")
	return body
}

func castRxjavaFunctionApply(body string) string {
	supers := collectRxjavaFunctionSupers(body)
	if _, ok := supers["mapper"]; !ok && strings.Contains(body, "this.mapper.apply(") && rxClassHasTypeVar(body, "T") {
		supers["mapper"] = []string{"T"}
	}
	for name, types := range supers {
		// `var0.apply` in RxJavaPlugins collides across methods; only rewrite
		// field receivers plus ScalarXMapZHelper params (handled separately).
		if strings.HasPrefix(name, "var") {
			continue
		}
		body = wrapCallArgs(body, "this."+name+".apply(", types)
		body = wrapCallArgs(body, name+".apply(", types)
	}
	return body
}

func collectRxjavaFunctionSupers(body string) map[string][]string {
	out := make(map[string][]string)
	from := 0
	for from < len(body) {
		rel := strings.Index(body[from:], "Function<? super ")
		if rel < 0 {
			return out
		}
		orig := from + rel
		i := orig
		kind := 1
		if orig >= 2 && body[orig-2:orig] == "Bi" {
			kind = 2
			i = orig - 2
		} else if orig > 0 && isIdentChar(body[orig-1]) {
			from = orig + 1
			continue
		}
		lt := strings.Index(body[i:], "<")
		if lt < 0 {
			from = orig + 1
			continue
		}
		n := skipBalanced(body[i+lt:], '<', '>')
		if n < 2 {
			from = orig + 1
			continue
		}
		inner := body[i+lt+1 : i+lt+n-1]
		types := parseWildcardSupers(inner)
		if kind == 1 && len(types) > 1 {
			types = types[:1]
		}
		if len(types) == 0 {
			from = orig + 1
			continue
		}
		after := strings.TrimLeft(body[i+lt+n:], " \t")
		name, ok, _ := readJavaIdent(after)
		if !ok {
			from = orig + 1
			continue
		}
		out[name] = types
		from = i + lt + n
	}
	return out
}

func parseWildcardSupers(inner string) []string {
	var types []string
	depth := 0
	start := 0
	flush := func(seg string) {
		seg = strings.TrimSpace(seg)
		if !strings.HasPrefix(seg, "? super ") {
			return
		}
		typ, _ := readJavaType(seg[len("? super "):])
		if typ != "" {
			types = append(types, typ)
		}
	}
	for j := 0; j < len(inner); j++ {
		switch inner[j] {
		case '<':
			depth++
		case '>':
			depth--
		case ',':
			if depth == 0 {
				flush(inner[start:j])
				start = j + 1
			}
		}
	}
	flush(inner[start:])
	return types
}

func wrapCallArgs(body, needle string, types []string) string {
	if needle == "" || len(types) == 0 {
		return body
	}
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		start := from + rel
		if start > 0 && isIdentChar(body[start-1]) {
			from = start + 1
			continue
		}
		argStart := start + len(needle)
		args, n, ok := splitCallArgs(body[argStart:])
		if !ok {
			from = argStart
			continue
		}
		changed := false
		for i := 0; i < len(args) && i < len(types); i++ {
			a := strings.TrimSpace(args[i])
			if a == "" || a == "null" || strings.HasPrefix(a, "(") {
				continue
			}
			args[i] = "(" + types[i] + ")(" + a + ")"
			changed = true
		}
		if !changed {
			from = argStart + n
			continue
		}
		neu := strings.Join(args, ",")
		body = body[:argStart] + neu + body[argStart+n:]
		from = argStart + len(neu)
	}
}

func splitCallArgs(s string) (args []string, n int, ok bool) {
	depthParen, depthAngle := 0, 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depthParen++
		case ')':
			if depthParen == 0 && depthAngle == 0 {
				if i >= start {
					seg := s[start:i]
					if strings.TrimSpace(seg) != "" || len(args) > 0 {
						args = append(args, seg)
					}
				}
				return args, i, true
			}
			if depthParen > 0 {
				depthParen--
			}
		case '<':
			depthAngle++
		case '>':
			if depthAngle > 0 {
				depthAngle--
			}
		case ',':
			if depthParen == 0 && depthAngle == 0 {
				args = append(args, s[start:i])
				start = i + 1
			}
		}
	}
	return nil, 0, false
}

func readJavaType(s string) (string, string) {
	s = strings.TrimLeft(s, " \t")
	ident, ok, rest := readJavaIdent(s)
	if !ok {
		return "", s
	}
	for strings.HasPrefix(rest, ".") {
		id2, ok2, rest2 := readJavaIdent(rest[1:])
		if !ok2 {
			break
		}
		ident += "." + id2
		rest = rest2
	}
	if strings.HasPrefix(rest, "<") {
		n := skipBalanced(rest, '<', '>')
		if n > 0 {
			ident += rest[:n]
			rest = rest[n:]
		}
	}
	for strings.HasPrefix(rest, "[]") {
		ident += "[]"
		rest = rest[2:]
	}
	return ident, rest
}

func skipBalanced(s string, open, close byte) int {
	if s == "" || s[0] != open {
		return -1
	}
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return -1
}

func isIdentChar(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '$'
}

// castRxjavaDownstreamOnNext inserts the downstream capture type on Object
// locals / expressions passed to onNext/tryOnNext.
func castRxjavaDownstreamOnNext(body string) string {
	super := rxDownstreamSuper(body)
	if super == "" {
		return body
	}
	body = wrapReceiverCallArg(body, "this.downstream.onNext(", super)
	body = wrapReceiverCallArg(body, "this.downstream.tryOnNext(", super)
	return body
}

func rxDownstreamSuper(body string) string {
	for _, pat := range []string{
		"final Subscriber<? super ",
		"final Observer<? super ",
		"final ConditionalSubscriber<? super ",
	} {
		if i := strings.Index(body, pat); i >= 0 {
			typ, rest := readJavaType(body[i+len(pat):])
			if typ != "" && strings.HasPrefix(strings.TrimLeft(rest, " \t"), ">") {
				return typ
			}
		}
	}
	switch {
	case strings.Contains(body, "BasicFuseableConditionalSubscriber<T, U>") || strings.Contains(body, "BasicFuseableSubscriber<T, U>"):
		return "U"
	case strings.Contains(body, "Subscriber<? super U>") || strings.Contains(body, "Observer<? super U>") || strings.Contains(body, "ConditionalSubscriber<? super U>"):
		return "U"
	case strings.Contains(body, "Subscriber<? super R>") || strings.Contains(body, "Observer<? super R>"):
		return "R"
	case strings.Contains(body, "Subscriber<? super T>") || strings.Contains(body, "Observer<? super T>"):
		return "T"
	}
	for _, pat := range []string{
		"Subscriber<? super ",
		"Observer<? super ",
		"ConditionalSubscriber<? super ",
	} {
		if i := strings.Index(body, pat); i >= 0 {
			typ, rest := readJavaType(body[i+len(pat):])
			if typ != "" && strings.HasPrefix(strings.TrimLeft(rest, " \t"), ">") {
				return typ
			}
		}
	}
	return ""
}

func wrapHalfSerializerOnNext(body string) string {
	typ := rxDownstreamSuper(body)
	if typ == "" {
		if rxClassHasTypeVar(body, "T") {
			typ = "T"
		} else if rxClassHasTypeVar(body, "R") {
			typ = "R"
		} else {
			return body
		}
	}
	needle := "HalfSerializer.onNext(this.downstream,"
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel + len(needle)
		if i < len(body) && body[i] == '(' {
			from = i
			continue
		}
		ident, ok, rest := readJavaIdent(body[i:])
		if !ok || !strings.HasPrefix(rest, ",") {
			from = i
			continue
		}
		cast := "(" + typ + ")(" + ident + ")"
		body = body[:i] + cast + rest
		from = i + len(cast)
	}
}

func wrapReceiverCallArg(body, needle, typ string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel + len(needle)
		if i < len(body) && (body[i] == '(' || body[i] == 'n') { // already cast or null
			from = i
			continue
		}
		args, n, ok := splitCallArgs(body[i:])
		if !ok || len(args) != 1 {
			from = i
			continue
		}
		a := strings.TrimSpace(args[0])
		if a == "" || a == "null" || strings.HasPrefix(a, "(") {
			from = i + n
			continue
		}
		cast := "(" + typ + ")(" + a + ")"
		body = body[:i] + cast + body[i+n:]
		from = i + len(cast)
	}
}

// castRxjavaLooseOnNextLocals casts Object locals passed to onNext on
// Subscriber/Observer/Processor receivers other than this.downstream.
func castRxjavaLooseOnNextLocals(body string) string {
	typ := ""
	switch {
	case strings.Contains(body, "UnicastProcessor<TRight>") || strings.Contains(body, "UnicastSubject<TRight>") || strings.Contains(body, "Subscriber<? super TRight>"):
		typ = "TRight"
	case strings.Contains(body, "UnicastProcessor<T>") || strings.Contains(body, "UnicastSubject<T>") || strings.Contains(body, "Subscriber<? super T>") || strings.Contains(body, "Observer<? super T>"):
		typ = "T"
	case strings.Contains(body, "Subscriber<? super R>") || strings.Contains(body, "Observer<? super R>"):
		typ = "R"
	default:
		return body
	}
	from := 0
	needle := ".onNext("
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return body
		}
		i := from + rel
		prefix := body[max(0, i-24):i]
		if strings.Contains(prefix, "this.downstream") || strings.Contains(prefix, "processor") || strings.Contains(prefix, "signaller") {
			from = i + len(needle)
			continue
		}
		argStart := i + len(needle)
		if argStart < len(body) && (body[argStart] == '(' || body[argStart] == 'n') {
			from = argStart
			continue
		}
		ident, ok, rest := readJavaIdent(body[argStart:])
		if !ok || !strings.HasPrefix(rest, ")") {
			from = argStart
			continue
		}
		if !isDecompilerLocal(ident) && ident != "var0" {
			from = argStart
			continue
		}
		cast := "(" + typ + ")(" + ident + ")"
		body = body[:argStart] + cast + rest
		from = argStart + len(cast)
	}
}

func replaceDuplicateEmptyLoopLabel(body string) string {
	first := strings.Index(body, "LOOP_1:\n")
	if first < 0 {
		return body
	}
	rel := strings.Index(body[first+1:], "LOOP_1:\n")
	if rel < 0 {
		return body
	}
	second := first + 1 + rel
	rest := body[second:]
	// LOOP_1:\n + tabs + do{\n\n + tabs + } while (true);
	nl := strings.Index(rest, "do{\n")
	if nl < 0 || nl > 40 {
		return body
	}
	endRel := strings.Index(rest, "} while (true);")
	if endRel < 0 || endRel > 80 {
		return body
	}
	return body[:second] + "continue LOOP_1;" + rest[endRel+len("} while (true);"):]
}
