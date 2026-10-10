package org.benf.cfr.reader;

import java.util.ArrayList;
import java.util.function.Consumer;

public class LambdaTest {
	// Fields
	 int a;

	public LambdaTest() {
		this.a = (this.a) + (1);
	}
	void main() {
		ArrayList var1 = new ArrayList();
		Consumer<Object> var2 = (l0) -> {
			int lv1_1 = 1;
		};
		var1.forEach(var2);
		int var3 = 1;
		ArrayList var4 = new ArrayList();
		var4.add(Integer.valueOf(1));
		Consumer<Object> var5 = (l0) -> {
			System.out.println(l0);
		};
		var4.forEach(var5);
	}
}