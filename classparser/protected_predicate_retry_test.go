package javaclassparser

import "testing"

// The successful boolean store stays inside its predicate's protected range,
// even when the sole retry back edge comes from a switch in the catch. The
// downstream call lies outside that range and must escape on failure.
func TestAdversarialProtectedPredicateRetryRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "ProtectedPredicateRetry", `
enum PredicateAction { RETRY, SKIP, STOP, ERROR }
public class ProtectedPredicateRetry {
  String trace="";
  int attempts,failures;
  boolean done,match,downstreamFailure,handlerFailure;
  PredicateAction action;
  boolean predicate(int value) throws Exception {
    trace+="P";
    if(++attempts>8)throw new AssertionError("retry bound");
    if(attempts<=failures)throw new Exception("predicate");
    return match;
  }
  PredicateAction handle(long count,Throwable primary) {
    trace+="H"+count;
    if(handlerFailure)throw new IllegalArgumentException("handler");
    return action;
  }
  boolean downstream(int value) {
    trace+="D";
    if(downstreamFailure)throw new IllegalStateException("downstream");
    return value>0;
  }
  void cancel() { trace+="C"; }
  void complete() { trace+="F"; }
  void error(Throwable failure) { trace+="E"+failure.getMessage(); }
  boolean accept(int value) {
    if(done)return false;
    long retries=0;
    boolean matches;
    for(;;) {
      try { matches=predicate(value);break; }
      catch(Throwable primary) {
        PredicateAction selected;
        try { selected=handle(++retries,primary); }
        catch(Throwable secondary) {
          cancel();trace+="X"+primary.getMessage()+":"+secondary.getMessage();return false;
        }
        switch(selected) {
          case RETRY:continue;
          case SKIP:return false;
          case STOP:cancel();complete();return false;
          default:cancel();error(primary);return false;
        }
      }
    }
    return matches && downstream(value);
  }
  public static void main(String[] args) { PredicateRetryOracle.main(args); }
}
class PredicateRetryOracle {
  public static void main(String[] args) {
    for(int failures:new int[]{0,1,3})for(PredicateAction action:PredicateAction.values())
      for(int mask=0;mask<16;mask++)for(int value:new int[]{-1,1}) {
        ProtectedPredicateRetry consumer=new ProtectedPredicateRetry();
        consumer.failures=failures;consumer.action=action;
        consumer.done=(mask&1)!=0;consumer.match=(mask&2)!=0;
        consumer.downstreamFailure=(mask&4)!=0;consumer.handlerFailure=(mask&8)!=0;
        String outcome;
        try {outcome=Boolean.toString(consumer.accept(value));}
        catch(Throwable failure) {outcome=failure.getClass().getName()+":"+failure.getMessage();}
        System.out.println(failures+":"+action+":"+mask+":"+value+":"+outcome+":"+consumer.attempts+":"+consumer.trace);
      }
  }
}`, Precision, Compatibility, "legacy")
}
