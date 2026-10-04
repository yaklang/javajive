package org.benf.cfr.reader;

public class VarArgs {
	void main(String... var1) {
		System.out.println(var1[0]);
	}
	void invoke() {
		String var1 = "a";
		String[] var2 = new String[]{"a"};
		this.main(var2);
	}
}