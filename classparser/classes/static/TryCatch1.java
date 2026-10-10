package org.benf.cfr.reader;

import java.io.File;
import java.io.FileInputStream;

public class TryCatch1 {
	public static void main(String[] var0) {
try {
		System.out.println(2);
		try{
			FileInputStream var1 = new FileInputStream(new File(""));
			try{
				System.out.println(1);
			}catch(Throwable var2){
				try{
					var1.close();
				}catch(Throwable var3){
					var2.addSuppressed(var3);
				}
				throw var2;
			}
			var1.close();
		}catch(Exception var1_1){
			var1_1.printStackTrace();
		}

} catch (java.lang.Throwable jdec$escape$0) {
throw jdec$rethrow$0(jdec$escape$0);
}
	}
	private static <E extends java.lang.Throwable> java.lang.RuntimeException jdec$rethrow$0(java.lang.Throwable failure) throws E {throw (E) failure;}
}