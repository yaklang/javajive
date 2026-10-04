package org.benf.cfr.reader;

public class TernaryExpressionTest {
	int getVar() {
		return 1;
	}
	void main() {
		int var1 = 1;
		int var2 = ((var1) == (2)) ? (this.getVar()) : (((var1) == (1)) ? (1) : (2));
		System.out.println(var2);
		String var3 = "s";
		String var4 = (((var3) == (null)) ? (var3 = "a") : (var3 = "b")).toString();
	}
}