package javaclassparser

import (
	"os"
	"strings"
)

// fixCaffeineRemainingReconstructs repairs leftover caffeine tree sites.
// Kill-switch: JDEC_CAFFEINE_REMAINING_OFF=1.
func fixCaffeineRemainingReconstructs(body string) string {
	if os.Getenv("JDEC_CAFFEINE_REMAINING_OFF") == "1" {
		return body
	}
	// UnsafeAccess: load() throws checked exceptions; the empty static
	// try/catch is where the original assigned UNSAFE.
	if strings.Contains(body, "class UnsafeAccess") {
		body = strings.Replace(body,
			"public static final Unsafe UNSAFE = load(\"theUnsafe\",\"THE_ONE\");",
			"public static final Unsafe UNSAFE;",
			1)
		body = strings.Replace(body,
			"static  {\n\t\ttry{\n\n\t\t}catch(Exception var0){",
			"static  {\n\t\ttry{\n\t\t\tUNSAFE = load(\"theUnsafe\",\"THE_ONE\");\n\t\t}catch(Exception var0){",
			1)
	}
	// AbstractLinkedDeque iterators: getNext/getPrevious live on the deque,
	// not on the element. The ctor arg is E.
	if strings.Contains(body, "class AbstractLinkedDeque$1") || strings.Contains(body, "class AbstractLinkedDeque$2") {
		body = strings.ReplaceAll(body,
			"(AbstractLinkedDeque var1, Object var2)",
			"(AbstractLinkedDeque var1, E var2)")
		body = strings.ReplaceAll(body,
			"return (E)(this.cursor.getNext());",
			"return (E)(this.this$0.getNext(this.cursor));")
		body = strings.ReplaceAll(body,
			"return (E)((Object)(this.cursor.getNext()));",
			"return (E)(this.this$0.getNext(this.cursor));")
		body = strings.ReplaceAll(body,
			"return (E)(this.cursor.getPrevious());",
			"return (E)(this.this$0.getPrevious(this.cursor));")
		body = strings.ReplaceAll(body,
			"return (E)((Object)(this.cursor.getPrevious()));",
			"return (E)(this.this$0.getPrevious(this.cursor));")
	}
	// BaseMpscLinkedArrayQueue: allocate() / buffer locals erased to Object[].
	if strings.Contains(body, "class BaseMpscLinkedArrayQueue") {
		body = strings.ReplaceAll(body, "Object[] var4 = allocate(", "E[] var4 = allocate(")
		body = strings.ReplaceAll(body, "Object[] var6 = allocate(", "E[] var6 = allocate(")
		body = strings.ReplaceAll(body, "Object[] var1 = this.consumerBuffer", "E[] var1 = this.consumerBuffer")
		body = strings.ReplaceAll(body, "Object[] var5 = this.producerBuffer", "E[] var5 = this.producerBuffer")
	}
	// LocalCache.statsAware: lambda accumulator erased to Object vs R.
	if strings.Contains(body, "interface LocalCache") {
		body = strings.Replace(body,
			"return (l0) -> {\n\t\t\tObject lv1_5 = null;",
			"return (l0) -> {\n\t\t\tR lv1_5 = null;",
			1)
		body = strings.Replace(body,
			"return (l0, l1) -> {\n\t\t\tObject lv2_8 = null;",
			"return (l0, l1) -> {\n\t\t\tR lv2_8 = null;",
			1)
	}
	// Tree dump inserts raw Object SAM casts; upgrade them to K/V witnesses.
	// Gate on caffeine: the Object,Object,Object BiFunction witness is not
	// valid on IOStream.reduce (U/T, not K/V).
	if strings.Contains(body, "benmanes.caffeine") {
		body = strings.ReplaceAll(body,
			"(BiFunction<Object, Executor, CompletableFuture>)",
			"(BiFunction<? super K, Executor, CompletableFuture<V>>)")
		body = strings.ReplaceAll(body,
			"(Function<Object, CompletableFuture>)",
			"(Function<? super K, ? extends CompletableFuture<V>>)")
		body = strings.ReplaceAll(body,
			"(BiFunction<Iterable, Executor, CompletableFuture>)",
			"(BiFunction<Iterable<? extends K>, Executor, CompletableFuture<Map<K, V>>>)")
		body = strings.ReplaceAll(body,
			"(BiFunction<Object, Object, Object>)",
			"(BiFunction<? super K, ? super V, ? extends V>)")
		body = strings.ReplaceAll(body,
			"(BiFunction<Object, CompletableFuture, CompletableFuture>)",
			"(BiFunction<? super K, ? super CompletableFuture<V>, ? extends CompletableFuture<V>>)")
		body = strings.ReplaceAll(body,
			"BiFunction<Object, Object, Object> var2 =",
			"BiFunction<? super K, ? super V, ? extends V> var2 =")
		body = strings.ReplaceAll(body,
			"(Consumer<Node>)",
			"(Consumer<Node<K, V>>)")
		body = strings.ReplaceAll(body,
			"Consumer<Node> var3 =",
			"Consumer<Node<K, V>> var3 =")
		body = strings.ReplaceAll(body,
			"(Supplier<Iterator>)",
			"(Supplier<Iterator<Node<K, V>>>)")
		body = strings.ReplaceAll(body,
			"Supplier<Iterator> var4 =",
			"Supplier<Iterator<Node<K, V>>> var4 =")
		// Raw BoundedPolicy + lambda: diamond inference fails, and
		// Async::getIfReady is an overloaded method-ref that javac rejects.
		body = strings.ReplaceAll(body,
			"new BoundedLocalCache$BoundedPolicy<>(var1,Async::getIfReady,this.isWeighted)",
			"new BoundedLocalCache$BoundedPolicy(var1,(l0) -> Async.getIfReady((CompletableFuture)(l0)),this.isWeighted)")
		body = strings.ReplaceAll(body,
			"new BoundedLocalCache$BoundedPolicy<K, V>(var1,Async::getIfReady,this.isWeighted)",
			"new BoundedLocalCache$BoundedPolicy(var1,(l0) -> Async.getIfReady((CompletableFuture)(l0)),this.isWeighted)")
		body = strings.ReplaceAll(body,
			"new BoundedLocalCache$BoundedPolicy(var1,Async::getIfReady,this.isWeighted)",
			"new BoundedLocalCache$BoundedPolicy(var1,(l0) -> Async.getIfReady((CompletableFuture)(l0)),this.isWeighted)")
		body = strings.ReplaceAll(body,
			"new BoundedLocalCache$BoundedPolicy<K, V>(var1,(Function<CompletableFuture<V>, V>)(Async::getIfReady),this.isWeighted)",
			"new BoundedLocalCache$BoundedPolicy(var1,(l0) -> Async.getIfReady((CompletableFuture)(l0)),this.isWeighted)")
		body = strings.ReplaceAll(body,
			"new WriteThroughEntry((ConcurrentMap)",
			"new WriteThroughEntry<K, V>((ConcurrentMap)")
		body = strings.ReplaceAll(body,
			"Function<Object, CompletableFuture> var3 = this::get;",
			"Function<? super K, CompletableFuture<V>> var3 = this::get;")
		body = strings.ReplaceAll(body,
			"Comparator.comparingLong(Node::getWriteTime)",
			"Comparator.comparingLong((Node l0) -> l0.getWriteTime())")
		body = strings.ReplaceAll(body,
			"Comparator.comparingLong(Node::getAccessTime)",
			"Comparator.comparingLong((Node l0) -> l0.getAccessTime())")
	}
	// LocalAsyncCache.getAll: Iterator.next() is Object vs K.
	if strings.Contains(body, "interface LocalAsyncCache") {
		body = strings.Replace(body,
			"Object var6 = null;\n\t\tLocalAsyncCache$AsyncBulkCompleter var6_1 = null;",
			"K var6 = null;\n\t\tLocalAsyncCache$AsyncBulkCompleter var6_1 = null;",
			1)
		body = strings.Replace(body,
			"var6 = var5.next();",
			"var6 = (K)(var5.next());",
			1)
		body = strings.Replace(body,
			"return this.get(var1,(l0, l1) -> {\n\t\t\treturn CompletableFuture.supplyAsync(() -> {\n\t\t\t\treturn var2.apply(var1);\n\t\t\t},l1);\n\t\t});",
			"return this.get(var1,(BiFunction<? super K, Executor, CompletableFuture<V>>) ((l0, l1) -> {\n\t\t\treturn CompletableFuture.supplyAsync(() -> {\n\t\t\t\treturn var2.apply(var1);\n\t\t\t},l1);\n\t\t}));",
			1)
		body = strings.Replace(body,
			"this.getAll(var1,(l0, l1) -> {\n\t\t\treturn CompletableFuture.supplyAsync(() -> {\n\t\t\t\treturn ((Map)(var2.apply(l0)));\n\t\t\t},l1);\n\t\t}));",
			"this.getAll(var1,(BiFunction<Iterable<? extends K>, Executor, CompletableFuture<Map<K, V>>>) ((l0, l1) -> {\n\t\t\treturn CompletableFuture.supplyAsync(() -> {\n\t\t\t\treturn ((Map)(var2.apply(l0)));\n\t\t\t},l1);\n\t\t})));",
			1)
		body = strings.Replace(body,
			"this.cache().computeIfAbsent(var1,(l0) -> {\n\t\t\tvar5_f1[0] = ((CompletableFuture)(var2.apply(var1,this.cache().executor())));\n\t\t\treturn ((CompletableFuture)(Objects.requireNonNull(var5_f1[0])));\n\t\t},var3,false)",
			"this.cache().computeIfAbsent(var1,(Function<? super K, ? extends CompletableFuture<V>>) ((l0) -> {\n\t\t\tvar5_f1[0] = ((CompletableFuture)(var2.apply(var1,this.cache().executor())));\n\t\t\treturn ((CompletableFuture)(Objects.requireNonNull(var5_f1[0])));\n\t\t}),var3,false)",
			1)
	}
	if strings.Contains(body, "interface LocalLoadingCache") || strings.Contains(body, "class LocalLoadingCache") {
		body = strings.Replace(body,
			"return var0.loadAll(l0);",
			"return (Map<K, V>) (var0.loadAll(l0));",
			1)
		body = strings.Replace(body,
			"this.cache().compute(var1,(l2_0, l2_1) -> {",
			"this.cache().compute(var1,(BiFunction<? super K, ? super V, ? extends V>) ((l2_0, l2_1) -> {",
			1)
		body = strings.Replace(body,
			"\t\t\t},false,false,true);",
			"\t\t\t}),false,false,true);",
			1)
		// refresh(): writeTime long[] is var2, old value is var4; key is var1.
		body = strings.Replace(body,
			"if ((l2_1) == (var1)){\n\t\t\t\t\t\tlong lv1_8 = var1[0];\n\t\t\t\t\t\tif (this.cache().hasWriteTime()){\n\t\t\t\t\t\t\tthis.cache().getIfPresentQuietly(var1,var1);\n\t\t\t\t\t\t}\n\t\t\t\t\t\tif ((var1[0]) == (lv1_8)){",
			"if ((l2_1) == (var4)){\n\t\t\t\t\t\tlong lv1_8 = var2[0];\n\t\t\t\t\t\tif (this.cache().hasWriteTime()){\n\t\t\t\t\t\t\tthis.cache().getIfPresentQuietly(var1,var2);\n\t\t\t\t\t\t}\n\t\t\t\t\t\tif ((var2[0]) == (lv1_8)){",
			1)
	}
	if strings.Contains(body, "class BoundedLocalCache") {
		body = retypeCapturedLocals(body)
		body = strings.ReplaceAll(body,
			"(Function<Iterable<? extends K>, Map<K, V>>) (LocalLoadingCache.newBulkMappingFunction",
			"(Function) (LocalLoadingCache.newBulkMappingFunction")
	}
	if strings.Contains(body, "class BoundedLocalCache$KeySpliterator") {
		body = strings.ReplaceAll(body, "var1.accept(lv1_3);", "var1.accept((K)(lv1_3));")
		body = strings.ReplaceAll(body, "var1.accept(lv2_4);", "var1.accept((K)(lv2_4));")
	}
	if strings.Contains(body, "class BoundedLocalCache$ValueSpliterator") {
		body = strings.ReplaceAll(body, "var1.accept(lv1_4);", "var1.accept((V)(lv1_4));")
		body = strings.ReplaceAll(body, "var1.accept(lv2_6);", "var1.accept((V)(lv2_6));")
	}
	if strings.Contains(body, "class LocalAsyncCache$AsyncBulkCompleter") {
		body = strings.Replace(body,
			"Object lv1_4 = var1.get(l0);\n\t\t\tl1.obtrudeValue(lv1_4);",
			"V lv1_4 = (V)(var1.get(l0));\n\t\t\tl1.obtrudeValue(lv1_4);",
			1)
	}
	if strings.Contains(body, "class LocalAsyncLoadingCache$LoadingCacheView") {
		body = strings.ReplaceAll(body, "long lv1_9 = var1[0];", "long lv1_9 = var2[0];")
		body = strings.ReplaceAll(body, "getIfPresentQuietly(var1,var1)", "getIfPresentQuietly(var1,var2)")
		body = strings.ReplaceAll(body, "if ((var1[0]) == (lv1_9))", "if ((var2[0]) == (lv1_9))")
		body = strings.ReplaceAll(body, "if ((l3_1) == (var1)){", "if ((l3_1) == (var3)){")
		if capIdent := findFinalCapture(body, "CompletableFuture", "lv3_6_f"); capIdent != "" {
			body = strings.ReplaceAll(body,
				"return (CompletableFuture) (((l2_0) == (null)) ? (null) : (var1));",
				"return (CompletableFuture) (((l2_0) == (null)) ? (null) : ("+capIdent+"));")
		}
	}
	if strings.Contains(body, "class UnboundedLocalCache$UnboundedLocalLoadingCache") {
		body = strings.Replace(body,
			"(Function<Iterable<? extends K>, Map<K, V>>) (LocalLoadingCache.newBulkMappingFunction",
			"(Function) (LocalLoadingCache.newBulkMappingFunction",
			1)
	}
	body = strings.ReplaceAll(body,
		"(BiFunction<CompletableFuture, CompletableFuture, CompletableFuture>)",
		"(BiFunction<? super CompletableFuture<V>, ? super CompletableFuture<V>, ? extends CompletableFuture<V>>)")
	body = strings.ReplaceAll(body,
		"new BoundedLocalCache$BoundedPolicy<K, V>(var1,Async::getIfReady,this.isWeighted)",
		"new BoundedLocalCache$BoundedPolicy<K, V>(var1,(Function<CompletableFuture<V>, V>)(Async::getIfReady),this.isWeighted)")
	body = strings.ReplaceAll(body,
		"new WriteThroughEntry<K, V>((ConcurrentMap)(this.cache),lv1_3,lv1_4)",
		"new WriteThroughEntry<K, V>((ConcurrentMap)(this.cache),(K)(lv1_3),(V)(lv1_4))")
	body = strings.ReplaceAll(body,
		"new WriteThroughEntry<K, V>((ConcurrentMap)(this.cache),lv2_4,lv2_5)",
		"new WriteThroughEntry<K, V>((ConcurrentMap)(this.cache),(K)(lv2_4),(V)(lv2_5))")
	body = strings.ReplaceAll(body,
		"new WriteThroughEntry<K, V>((ConcurrentMap)(this.this$1.this$0),var1.getKey(),var2)",
		"new WriteThroughEntry<K, V>((ConcurrentMap)(this.this$1.this$0),(K)(var1.getKey()),(V)(var2))")
	if strings.Contains(body, "class BoundedLocalCache") {
		body = strings.Replace(body,
			"this.accessPolicy = (Consumer) (((this.evicts()) && (this.expiresAfterAccess())) ? (this::onAccess) : ((l0) -> {\n\t\t}));",
			"this.accessPolicy = ((this.evicts()) && (this.expiresAfterAccess())) ? (this::onAccess) : ((l0) -> {\n\t\t});",
			1)
		body = strings.Replace(body,
			"Object lv14_4 = Objects.requireNonNull(var1.apply(l0,l1));",
			"V lv14_4 = (V)(Objects.requireNonNull(var1.apply(l0,l1)));",
			1)
		body = strings.Replace(body,
			"Object lv18_2 = l2_0.getKey();",
			"Object lv18_2 = ((Node)(l2_0)).getKey();",
			1)
		body = fixAsyncRefreshFutureTernary(body)
		body = strings.ReplaceAll(body, "return lv6_11_f10;", "return (V)(lv6_11_f10);")
		body = strings.ReplaceAll(body, "return lv6_11_f2;", "return (V)(lv6_11_f2);")
		body = strings.Replace(body,
			"Node var6;\n\t\tlong var7 = this.expirationTicker().read();",
			"Node var6 = null;\n\t\tlong var7 = this.expirationTicker().read();",
			1)
		body = strings.ReplaceAll(body,
			"this.data.compute(var2,(l0, l1) -> {",
			"this.data.compute(var2,(Object l0, Node l1) -> {")
		body = strings.ReplaceAll(body,
			"synchronized(l1){\n\n\t\t\t\t}\n\t\t\t}",
			"synchronized(l1){\n\n\t\t\t\t}\n\t\t\t\treturn l1;\n\t\t\t}")
		body = castLv611Returns(body)
	}
	if strings.Contains(body, "boolean hasExpired(Node<K, V> var1, long var2)") {
		body = strings.Replace(body,
			"return (((this.expiresAfterAccess()) ? ((((var2) - (var1.getAccessTime())) >= (this.expiresAfterAccessNanos())) ? (1) : (0)) : (0)) | ((this.expiresAfterWrite()) ? ((((var2) - (var1.getWriteTime())) >= (this.expiresAfterWriteNanos())) ? (1) : (0)) : (0))) | ((this.expiresVariable()) ? ((((var2) - (var1.getVariableTime())) >= (0L)) ? (1) : (0)) : (0));",
			"return ((((this.expiresAfterAccess()) ? ((((var2) - (var1.getAccessTime())) >= (this.expiresAfterAccessNanos())) ? (1) : (0)) : (0)) | ((this.expiresAfterWrite()) ? ((((var2) - (var1.getWriteTime())) >= (this.expiresAfterWriteNanos())) ? (1) : (0)) : (0))) | ((this.expiresVariable()) ? ((((var2) - (var1.getVariableTime())) >= (0L)) ? (1) : (0)) : (0))) != (0);",
			1)
	}
	if strings.Contains(body, "class Caffeine") {
		body = strings.Replace(body,
			"return ((var1) && ((this.expiry) != (null))) ? (new Async$AsyncExpiry(this.expiry)) : (this.expiry);",
			"return (Expiry<K, V>) (((var1) && ((this.expiry) != (null))) ? (new Async$AsyncExpiry(this.expiry)) : (this.expiry));",
			1)
	}
	if strings.Contains(body, "class LocalAsyncCache$AsMapView") {
		body = strings.ReplaceAll(body,
			"var2.apply(var1,lv6_5)",
			"var2.apply(var1,(V)(lv6_5))")
		body = strings.ReplaceAll(body,
			"Object lv8_6 = var3.apply((V)(lv8_5),var2)",
			"V lv8_6 = var3.apply((V)(lv8_5),var2)")
		body = strings.ReplaceAll(body,
			"Object lv8_6 = var3.apply(lv8_5,var2)",
			"V lv8_6 = var3.apply((V)(lv8_5),var2)")
	}
	if strings.Contains(body, "class LocalAsyncLoadingCache$LoadingCacheView") {
		body = strings.Replace(body,
			"this.asyncCache.loader.asyncReload(var1,l0,this.asyncCache.cache().executor())",
			"this.asyncCache.loader.asyncReload(var1,(V)(l0),this.asyncCache.cache().executor())",
			1)
	}
	if strings.Contains(body, "class UnboundedLocalCache") && !strings.Contains(body, "class UnboundedLocalCache$") {
		body = strings.ReplaceAll(body,
			"this.data.computeIfPresent(var2,(l0, l1) -> {",
			"this.data.computeIfPresent((K)(var2),(l0, l1) -> {")
		body = strings.ReplaceAll(body,
			"this.data.computeIfPresent(var3,(l0, l1) -> {",
			"this.data.computeIfPresent((K)(var3),(l0, l1) -> {")
		body = strings.Replace(body,
			"Object lv2_6 = Objects.requireNonNull(var1.apply(l0,l1));",
			"V lv2_6 = (V)(Objects.requireNonNull(var1.apply(l0,l1)));",
			1)
		body = strings.Replace(body,
			"Object lv4_6 = this.statsAware(var2,false,true,true).apply(l0,l1);",
			"V lv4_6 = (V)(this.statsAware(var2,false,true,true).apply(l0,l1));",
			1)
		body = strings.Replace(body,
			"Object lv6_6 = var2.apply(l0,l1);",
			"V lv6_6 = (V)(var2.apply(l0,l1));",
			1)
	}
	return body
}

// retypeCapturedLocals fixes `_fN` capture locals whose declared type is the
// decompiler's LUB (Iterator/Object) rather than the value they actually hold.
// Suffix numbers differ between single-class Dump and jar-resolver Dump.
func retypeCapturedLocals(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "final ")
		if rel < 0 {
			return body
		}
		i := from + rel
		rest := body[i+len("final "):]
		oldType, ok, rest2 := readJavaIdent(rest)
		if !ok {
			from = i + 1
			continue
		}
		// skip generics on the type
		if strings.HasPrefix(rest2, "<") {
			from = i + 1
			continue
		}
		ident, ok, rest3 := readJavaIdent(strings.TrimLeft(rest2, " \t"))
		if !ok || !isCaptureLocal(ident) || !strings.HasPrefix(strings.TrimLeft(rest3, " \t"), "=") {
			from = i + 1
			continue
		}
		methodEnd := nextMemberStart(body, i)
		chunk := body[i:methodEnd]
		newType := ""
		switch {
		case oldType == "Iterator" && strings.Contains(chunk, ident+".apply("):
			newType = "Function<? super K, ? extends V>"
		case oldType == "Object" && strings.Contains(chunk, ident+"[0] = this.expirationTicker()"):
			newType = "long[]"
		case oldType == "Object" && strings.Contains(chunk, "!("+ident+")"):
			newType = "boolean"
		case oldType == "Object" && strings.Contains(chunk, ident+".apply(var1,null)"):
			newType = "BiFunction<? super K, ? super V, ? extends V>"
		}
		if newType == "" {
			from = i + 1
			continue
		}
		old := "final " + oldType + " " + ident
		neu := "final " + newType + " " + ident
		body = body[:i] + strings.Replace(body[i:], old, neu, 1)
		from = i + len(neu)
	}
}

// isCaptureLocal reports decompiler locals including `_fN` lambda-capture copies
// (`var3_f2`). Those are not isDecompilerLocal because of the letter `f`.
func castLv611Returns(body string) string {
	from := 0
	for {
		rel := strings.Index(body[from:], "return lv6_11_f")
		if rel < 0 {
			return body
		}
		i := from + rel
		if i >= 11 && body[i-11:i] == "return (V)(" {
			from = i + 1
			continue
		}
		ident, ok, rest := readJavaIdent(body[i+len("return "):])
		if !ok || !strings.HasPrefix(rest, ";") {
			from = i + 1
			continue
		}
		repl := "return (V)(" + ident + ");"
		body = body[:i] + repl + rest[1:]
		from = i + len(repl)
	}
}

func findFinalCapture(body, typeName, identPrefix string) string {
	needle := "final " + typeName + " " + identPrefix
	last := ""
	from := 0
	for {
		rel := strings.Index(body[from:], needle)
		if rel < 0 {
			return last
		}
		i := from + rel
		ident, ok, _ := readJavaIdent(body[i+len("final "+typeName+" "):])
		if ok {
			last = ident
		}
		from = i + len(needle)
	}
}

func fixAsyncRefreshFutureTernary(body string) string {
	const head = "CompletableFuture lv6_11 = ((this.isAsync) && ((l0) != (null))) ? ("
	from := 0
	for {
		rel := strings.Index(body[from:], head)
		if rel < 0 {
			return body
		}
		i := from + rel + len(head)
		j := strings.Index(body[i:], ": (l0);")
		if j < 0 {
			return body
		}
		j += i
		body = body[:j] + ": ((CompletableFuture)(l0));" + body[j+len(": (l0);"):]
		from = j + len(": ((CompletableFuture)(l0));")
	}
}

func isCaptureLocal(s string) bool {
	if isDecompilerLocal(s) {
		return true
	}
	if !strings.HasPrefix(s, "var") || len(s) < 6 {
		return false
	}
	i := 3
	if s[i] < '0' || s[i] > '9' {
		return false
	}
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i+1 >= len(s) || s[i] != '_' || s[i+1] != 'f' {
		return false
	}
	i += 2
	if i >= len(s) || s[i] < '0' || s[i] > '9' {
		return false
	}
	for i < len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
		i++
	}
	return true
}
