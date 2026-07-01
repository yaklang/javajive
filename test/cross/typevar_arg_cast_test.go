package cross

// 承重测试:「调用点形参被解析为类型变量 T、实参不可直赋时补 (T) 下行造型」治本 (CODEC_TODO Object→类型变量族).
//
// 两条互补缺口:
//   ① JDK 泛型方法参数表缺 Comparator.compare —— guava Comparators.isInOrder/isInStrictOrder:
//        Iterator it = iterable.iterator();   // RAW(iterator() 的 `? extends T` 命中通配符守卫退化为裸)
//        Object a = it.next(); Object b = it.next();
//        if (comparator.compare(a, b) > 0) ...  // comparator 是 Comparator<T>
//      compare 描述符把两形参都擦除成 Object,实参 a/b 是 Object,javac 按 compare(T,T) 复解析报
//      "Object cannot be converted to T"。把 Comparator.compare 纳入 jdkMethodParamTypeArgIndex 后,形参解析
//      回 T,既有 arg-cast 逻辑补出 compare((T)a,(T)b)。
//   ② 数组实参闸 (typeVarArrayArgCast) —— fastjson2 CSVReaderUTF8/UTF16.readLineObjectAll:
//        Object[] values = readLineValues(false);
//        consumer.accept(values);   // consumer 是 Consumer<T>,accept 形参解析回 T
//      实参是 Object[](数组),其 RawType() 是 *JavaArrayType 而非 *JavaClass,(ok1&&ok2) 类-类造型分支永不触发,
//      裸渲染后 javac 报 "Object[] cannot be converted to T"。新分支对「已解析为作用域内类型变量 T 的形参 + 数组实参」
//      补出 accept((T)values)(数组转类型变量是合法 unchecked cast)。

import "testing"

// TestComparatorCompareTypeVarCastIsLoadBearing pins guava Comparators: a `Comparator<T>.compare(a, b)`
// call whose arguments came from a raw `Iterator.next()` (so they are Object). Resolving compare's
// formals to the receiver's type arg T lets the arg-cast logic re-emit `compare((T)a, (T)b)`; without
// the Comparator entry in the JDK generic-param table javac rejects `Object cannot be converted to T`.
// The umbrella kill-switch JDEC_GENERIC_PARAM_INFER_OFF disables the resolver; removing only the
// Comparator.compare case likewise makes ON nonzero (the resolver no longer maps compare -> T).
func TestComparatorCompareTypeVarCastIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["guava"].relPath)
	if jarPath == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}
	const sw = "JDEC_GENERIC_PARAM_INFER_OFF"
	entries := []string{"com/google/common/collect/Comparators.class"}
	substrs := []string{"Object cannot be converted to T"}

	on := classConvErrCount(t, sw, jarPath, entries, "", substrs, false)  // resolver ON
	off := classConvErrCount(t, sw, jarPath, entries, "", substrs, true)  // resolver OFF (kill-switch)
	t.Logf("Comparators compare typevar errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear all compare typevar errors: ON=%d (want 0)", on)
	}
}

// TestTypeVarArrayArgCastIsLoadBearing pins fastjson2 CSVReaderUTF8/UTF16.readLineObjectAll: a
// `Consumer<T>.accept(values)` call whose argument `values` is the `Object[]` returned by
// readLineValues. The resolver recovers accept's formal as the type variable T, but the class-vs-class
// arg-cast branch cannot fire for an array argument (JavaArrayType.RawType() is not a *JavaClass), so the
// dedicated array-arg branch must synthesize `accept((T) values)`. Disabling it via the kill-switch must
// reintroduce `Object[] cannot be converted to T`.
func TestTypeVarArrayArgCastIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["fastjson2"].relPath)
	if jarPath == "" {
		t.Skip("fastjson2 jar not found under ~/.m2; skipping")
	}
	const sw = "JDEC_TYPEVAR_ARRAY_ARG_CAST_OFF"
	entries := []string{
		"com/alibaba/fastjson2/support/csv/CSVReaderUTF8.class",
		"com/alibaba/fastjson2/support/csv/CSVReaderUTF16.class",
	}
	substrs := []string{"Object[] cannot be converted to T"}

	on := classConvErrCount(t, sw, jarPath, entries, "", substrs, false)  // fix ON
	off := classConvErrCount(t, sw, jarPath, entries, "", substrs, true)  // fix OFF (kill-switch)
	t.Logf("CSVReader accept(Object[]) typevar errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear all accept(Object[]) typevar errors: ON=%d (want 0)", on)
	}
}

// TestTypeVarArrayElementStoreCastIsLoadBearing pins guava type-variable-array ELEMENT stores:
// `this.buffer[i] = objExpr` / `this.keys[i] = objExpr` / `this.values[r][c] = objExpr` where the
// fields are declared `T[]` / `K[]` / `V[][]` but the stored value is the erased Object. The aastore
// erased the source's `(T)` cast to a no-op; the dumper must re-emit it from the field's recorded
// generic Signature (FieldTypeVar), else javac rejects `Object cannot be converted to T/K/V/E`.
// Disabling it via the kill-switch must reintroduce those element-store conversion errors.
func TestTypeVarArrayElementStoreCastIsLoadBearing(t *testing.T) {
	lookJavac(t)
	jarPath := resolveJar(jarSpecs["guava"].relPath)
	if jarPath == "" {
		t.Skip("guava jar not found under ~/.m2; skipping")
	}
	const sw = "JDEC_TYPEVAR_ARRAY_ELEM_STORE_CAST_OFF"
	entries := []string{
		"com/google/common/collect/TopKSelector.class",
		"com/google/common/collect/HashBiMap.class",
		"com/google/common/collect/DenseImmutableTable.class",
		"com/google/common/collect/ImmutableSortedMultiset$SerializedForm.class",
	}
	// Each substring is the exact element type variable of one type-var-array field store.
	substrs := []string{
		"Object cannot be converted to T",
		"Object cannot be converted to K",
		"Object cannot be converted to V",
		"Object cannot be converted to E",
	}

	on := classConvErrCount(t, sw, jarPath, entries, "", substrs, false)  // fix ON
	off := classConvErrCount(t, sw, jarPath, entries, "", substrs, true)  // fix OFF (kill-switch)
	t.Logf("type-var-array element-store errors: ON=%d OFF=%d", on, off)

	if off == 0 {
		t.Fatalf("kill-switch did not reproduce the defect: OFF=%d (expected > 0)", off)
	}
	if on >= off {
		t.Errorf("fix is NOT load-bearing: ON=%d OFF=%d (ON must be strictly fewer)", on, off)
	}
	if on != 0 {
		t.Errorf("fix did not clear all element-store typevar errors: ON=%d (want 0)", on)
	}
}
