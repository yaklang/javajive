package org.benf.cfr.reader;

import java.util.ArrayList;

public class LambdaTest {
	// Fields
	 int a;

	public LambdaTest() {
		this.a = (this.a) + (1);
	}
	void main() {
		new ArrayList<>().forEach((l0) -> {
			int lv1_1 = 1;
		});
		int var1 = 1;
		ArrayList var2 = new ArrayList();
		var2.add(Integer.valueOf(1));
		final ArrayList var2_f1 = var2;
		var2.forEach((l0) -> {
			System.out.println(l0);
		});
	}
}