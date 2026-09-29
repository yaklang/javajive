package javaclassparser

import "testing"

// The operation remains original bytecode. Only the retrying consumer is
// replaced, so attempts, completion, exception propagation and interrupt state
// expose lost loop exits or catch/finally paths independently of source shape.
func TestAdversarialRetryInterruptCleanupRoundTrip(t *testing.T) {
	t.Parallel()
	roundTripGenericFlow(t, "RetryInterruptCleanup", `import java.io.*;
class RetryOperation {
  static int attempts,failures,outcome,completed;
  static void put() throws InterruptedException {
    if(++attempts>8)throw new AssertionError("retry did not stop");
    if(Thread.interrupted() || attempts<=failures)throw new InterruptedException();
    if(outcome==2)throw new IllegalStateException("fatal");
    completed++;
  }
  static void read() throws IOException {
    if(++attempts>8)throw new AssertionError("retry did not stop");
    if(Thread.currentThread().isInterrupted() || attempts<=failures)throw new InterruptedIOException();
    if(outcome==1)throw new IOException("ordinary");
    if(outcome==2)throw new IllegalStateException("fatal");
    completed++;
  }
}
public class RetryInterruptCleanup {
  static void put() {
    boolean interrupted=false;
    try {
      for(;;) {
        try {RetryOperation.put();break;}
        catch(InterruptedException e) {interrupted=true;}
      }
    } finally {if(interrupted)Thread.currentThread().interrupt();}
  }
  static void read() {
    boolean interrupted=false;
    while(true) {
      try {RetryOperation.read();return;}
      catch(InterruptedIOException e) {Thread.interrupted();interrupted=true;}
      catch(IOException e) {return;}
      finally {if(interrupted)Thread.currentThread().interrupt();}
      // This variant deliberately clears the restored flag before retrying.
      // A finally block must still execute on success, failure and every retry.
      Thread.interrupted();
    }
  }
  static void readFinallyOutside() {
    boolean interrupted=false;
    try {
      while(true) {
        try {RetryOperation.read();return;}
        catch(InterruptedIOException e) {Thread.interrupted();interrupted=true;}
        catch(IOException e) {return;}
      }
    } finally {if(interrupted)Thread.currentThread().interrupt();}
  }
  public static void main(String[] args) {
    for(int failures:new int[]{0,1,3})for(int outcome=0;outcome<3;outcome++)
      for(boolean initial:new boolean[]{false,true})for(int kind=0;kind<3;kind++) {
        Thread.interrupted();if(initial)Thread.currentThread().interrupt();
        RetryOperation.attempts=0;RetryOperation.completed=0;
        RetryOperation.failures=failures;RetryOperation.outcome=outcome;
        String result="ok";
        try {if(kind==0)put();else if(kind==1)read();else readFinallyOutside();}catch(RuntimeException e) {result=e.getClass().getSimpleName();}
        boolean restored=Thread.interrupted();
        System.out.print(RetryOperation.attempts+":"+RetryOperation.completed+":"+restored+":"+result+";");
      }
  }
}`, Precision, Compatibility, "legacy")
}
