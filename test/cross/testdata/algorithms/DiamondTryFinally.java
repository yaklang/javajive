package bench;

// If-diamond then a shared try/finally. Mirrors guava Monitor.enterWhen(Guard, long, TimeUnit):
// unfair tryLock() success must join the post-merge try/finally, not inline the any-handler.
public class DiamondTryFinally {
    static final class Lock {
        int holds;

        boolean tryLock() {
            holds++;
            return holds == 1;
        }

        boolean tryLock(long t) {
            if (t < 0) {
                return false;
            }
            holds++;
            return true;
        }

        void unlock() {
            holds--;
        }

        boolean isHeld() {
            return holds > 0;
        }
    }

    final Lock lock = new Lock();
    final boolean fair;

    DiamondTryFinally(boolean fair) {
        this.fair = fair;
    }

    boolean satisfied;
    boolean awaitOk;
    int signals;

    boolean isSatisfied() {
        return satisfied;
    }

    boolean awaitNanos(long n, boolean held) {
        return awaitOk && n >= 0 && held == lock.isHeld();
    }

    void signal() {
        signals++;
    }

    public boolean enterWhen(long time, boolean interrupt) throws InterruptedException {
        Lock lock = this.lock;
        boolean held = lock.isHeld();
        long start = 0L;
        if (!fair) {
            if (interrupt) {
                throw new InterruptedException();
            }
            if (!lock.tryLock()) {
                start = 1L;
                if (!lock.tryLock(time)) {
                    return false;
                }
            }
        } else {
            start = 1L;
            if (!lock.tryLock(time)) {
                return false;
            }
        }
        boolean ok = false;
        boolean threw = true;
        try {
            ok = isSatisfied() || awaitNanos(start == 0L ? time : time - start, held);
            threw = false;
            return ok;
        } finally {
            if (!ok) {
                try {
                    if (threw && !held) {
                        signal();
                    }
                } finally {
                    lock.unlock();
                }
            }
        }
    }

    static String run(boolean fair, boolean satisfied, boolean awaitOk, long time, boolean interrupt) {
        DiamondTryFinally m = new DiamondTryFinally(fair);
        m.satisfied = satisfied;
        m.awaitOk = awaitOk;
        try {
            boolean r = m.enterWhen(time, interrupt);
            return r + "/" + m.lock.holds + "/" + m.signals;
        } catch (InterruptedException e) {
            return "IE/" + m.lock.holds + "/" + m.signals;
        }
    }

    public static void main(String[] args) {
        System.out.println("u-sat=" + run(false, true, false, 5, false));
        System.out.println("u-await=" + run(false, false, true, 5, false));
        System.out.println("u-fail=" + run(false, false, false, 5, false));
        System.out.println("u-ie=" + run(false, true, false, 5, true));
        System.out.println("f-sat=" + run(true, true, false, 5, false));
        System.out.println("f-timeout=" + run(true, true, false, -1, false));
    }
}
