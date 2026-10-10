package org.benf.cfr.reader;

import java.util.HashMap;
import java.util.function.BiFunction;

public class LongTest {
	void main() {
		HashMap var1 = new HashMap();
		var1.merge(Long.valueOf(1L),Long.valueOf(10L),(BiFunction)(((BiFunction<Long, Long, Long>)(Long::sum))));
		var1.merge(Long.valueOf(1L),Long.valueOf(5L),(BiFunction)(((BiFunction<Long, Long, Long>)(Long::sum))));
		System.out.println(var1.get(Long.valueOf(1L)));
	}
}