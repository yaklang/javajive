package org.benf.cfr.reader;

public class IfTest {
	void main() {
		int var1 = 1;
		int var2 = 0;
		if ((var1) > (1)){
			var2 = 2;
		}else{
			var2 = 3;
		}
		if ((var2) > (1)){
			var2 = 2;
		}
		if (((var2) <= (1)) && ((var2) <= (0))){

		}else{
			var2 = 2;
		}
		if (((var2) <= (1)) && ((var2) <= (0))){
			var1 = 3;
		}else{
			var1 = 2;
		}
	}
}