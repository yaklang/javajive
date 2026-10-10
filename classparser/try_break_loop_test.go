package javaclassparser

import "testing"

func TestAdversarialSuccessfulTryBreakExitsRetryLoopRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "TryBreakRetry", `class RetryMiss extends Exception {}
class RetrySource {
  static int attempts;
  static Integer attempt(int scale,int failures) throws RetryMiss {
    if(++attempts>10) throw new IllegalStateException("lost successful break");
    if(attempts<=failures) throw new RetryMiss();
    return scale;
  }
}
public class TryBreakRetry {
  static String search(int failures) {
    RetrySource.attempts=0;
    Integer found=null;
    for(int scale=4;scale<=16;scale<<=1) {
      try {found=RetrySource.attempt(scale,failures);break;}
      catch(RetryMiss ignored) {}
    }
    return found+":"+RetrySource.attempts;
  }
  public static void main(String[] args) {
    System.out.print(search(0)+";"+search(1)+";"+search(2)+";"+search(3));
  }
}`, Precision, Compatibility, "legacy")
}
