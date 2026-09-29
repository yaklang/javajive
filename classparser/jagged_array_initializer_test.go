package javaclassparser

import "testing"

func TestAdversarialJaggedArrayInitializerKeepsOuterReceiverRoundTrip(t *testing.T) {
	roundTripGenericFlow(t, "com.google.zxing.regression.JaggedInitializer", `package com.google.zxing.regression;
import java.util.Arrays;
public class JaggedInitializer {
  static final int[] PREFIX={1,1,1,1};
  static final int[] SUFFIX={3,1,1};
  static final int[][] MATRIX={{1,1,3,3,1},{3,1,1,1,3},{1,3,1,1,3},{3,3,1,1,1},{1,1,3,1,3},{3,1,3,1,1},{1,3,3,1,1},{1,1,1,3,3},{3,1,1,3,1},{1,3,1,3,1}};
  public static void main(String[] args) {
    System.out.print(Arrays.toString(PREFIX)+":"+Arrays.toString(SUFFIX)+":"+Arrays.deepToString(MATRIX));
    MATRIX[0][0]=99;
    System.out.print(":"+MATRIX[1][0]+":"+PREFIX[0]+":"+new JaggedInitializer().length("abcd"));
  }
  int length(String value) { int count=value.length(); return count+count; }
}`)
}
