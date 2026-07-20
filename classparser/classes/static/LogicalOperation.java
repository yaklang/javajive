package org.benf.cfr.reader;

public class LogicalOperation {
	boolean main() {
		int var1 = 1;
		int var2 = ((var1) != (3)) ? (((var1) == (5)) ? (1) : (0)) : (1);
		var2 = ((var1) == (3)) ? (((var1) == (5)) ? (1) : (0)) : (0);
		return ((var1) == (3)) || (((var1) == (3)) && ((var1) == (5)));
	}
}