
处理阻塞 其实就是gopark的调用的地方，gopark就是让g处于等待状态  
1 channel阻塞  
2 net阻塞  
3 time阻塞  
4 io阻塞  
5 select case channel阻塞  
6 锁阻塞  
这些情况不会阻塞调度循环，而是会把goroutine挂起，  
所谓的挂起，其实是让g先进某个数据结构，待ready后再继续执行  
不会占用线程  
这时候线程会进入schedule，继续消费队列，执行其他g（啥叫线程会进入schedule？mstart1会调schedule）  
我理解是m是被fork出来的，然后就进调度看有没有g可以执行，没有就去p里捞  
至于m怎么被fork，一开始有一个m0，然后就搞出来一个m1(还有一个handoffp也会创建m)  
阻塞的时候g会被打包成sudog，可能是多个，分别挂在不同的队列上  
m的spinning表示如果是true，则m没找到g  
有一个全局调度器  
 


问题1：
程序的入口是什么  
1)  
文件:runtime._rt0_linux_amd64.s  
函数:_rt0_amd64_linux  
```
TEXT _rt0_amd64_linux(SB),NOSPLIT,$-8
	JMP	_rt0_amd64(SB)
```
2)
文件:runtime._rt0_amd64  
函数:_rt0_amd64  
```
TEXT _rt0_amd64(SB),NOSPLIT,$-8
	MOVQ	0(SP), DI	// argc
	LEAQ	8(SP), SI	// argv
	JMP	runtime·rt0_go(SB)
```
3)
文件:runtime.asm_amd64.s  
函数:runtime.rt0_go  
```
TEXT runtime·rt0_go(SB),NOSPLIT,$0
	// copy arguments forward on an even stack
	MOVQ	DI, AX		// argc
	MOVQ	SI, BX		// argv
	SUBQ	$(4*8+7), SP		// 2args 2auto
	ANDQ	$~15, SP
	MOVQ	AX, 16(SP)
	MOVQ	BX, 24(SP)
```
4)
文件:runtime.asm_amd64.s  
函数:runtime.rt0_go  
```
TEXT runtime·rt0_go(SB),NOSPLIT,$0
	// copy arguments forward on an even stack
	// 第一步 argc、argv的相关处理
	MOVQ	DI, AX		// argc
	MOVQ	SI, BX		// argv
	SUBQ	$(4*8+7), SP		// 2args 2auto
	ANDQ	$~15, SP
	MOVQ	AX, 16(SP)
	MOVQ	BX, 24(SP)
	...
	// 第二步 全局m0、g0的初始化，m0和g0互相绑定
	LEAQ	runtime·g0(SB), CX
	MOVQ	CX, g(BX)
	LEAQ	runtime·m0(SB), AX
	// save m->g0 = g0
	MOVQ	CX, m_g0(AX)
	// save m0 to g0->m
	MOVQ	AX, g_m(CX)
	...
	// 第三步 获取cpu信息、做初始化动作
	CALL	runtime·args(SB)
	CALL	runtime·osinit(SB)	// 初始化os，获取cpu个数和HugePages大小
	CALL	runtime·schedinit(SB)	// 在runtime.proc.go文件中，初始化一坨内置数据结构，来初始化调度系统，会进行p的初始化，也会把m0和某个p绑定。
	...
	// 第四步 开始执行用户main goroutine，就是为了执行runtime.main，建好之后插入到m0绑定的p的本地队列
	// create a new goroutine to start program	可以很清楚的看到，启动了一个g
	MOVQ	$runtime·mainPC(SB), AX		// entry	pc是跳转地址，执行用户的主函数
	PUSHQ	AX
	PUSHQ	$0			// arg size
	CALL	runtime·newproc(SB)	// 真正去执行
	POPQ	AX
	POPQ	AX
	...
	// start this M
	CALL	runtime·mstart(SB)	// 启动一个新的m，然后最终会调用schedule函数，从而进入死循环
	CALL	runtime·abort(SB)	// mstart should never return
	RET
```




问题2：
runtime.mstart -> mstart1 -> mstartm0 -> schedule()  
  
大部分m都是在执行一个循环    
也就是说进入调度是从m开始的   
空闲的线程会在一个idle的队列里边管理    
如果需要一个新的m则先去idle的队列找m   
```
func mget() *m {
	mp := sched.midle.ptr()
	if mp != nil {
		sched.midle = mp.schedlink
		sched.nmidle--
	}
	return mp
}
```
本地队列和runnext都是为了解决局部性的问题,最近被调用的会可能再次被调用，所以刚刚创建的g要放到runnext里  

问题3：  
怎么循环执行的？并不是for  
  
schedule -> runtime.execute -> runtime.gogo -> runtime.goexit -> schedule  

问题4：
sudog是什么鬼  
sudog 代表在等待列表里的 g，比如向 channel 发送/接收内容时  
之所以需要 sudog 是因为 g 和同步对象之间的关系是多对多的  
一个 g 可能会在多个等待队列中，所以一个 g 可能被打包为多个 sudog  
多个 g 也可以等待在同一个同步对象上  
因此对于一个同步对象就会有很多 sudog 了  
sudog 是从一个特殊的池中进行分配的。用 acquireSudog 和 releaseSudog 来分配和释放 sudog  
  
问题5：  
g是怎么初始化的  
```
go func() {
    // do.....
}()
```
runtime.newproc->runtime.newproc1  
```
	newg := gfget(_p_)	// 先从本地队列拿，拿不到从全局队列拿
	if newg == nil {//各种地方都拿不到，就创建要给
		newg = malg(_StackMin)
		casgstatus(newg, _Gidle, _Gdead)// 状态是dead
		allgadd(newg) 
	}
	...
	runqput(_p_, newg, true)// 扔到哪里？优先runnext，其次localrunq，再次globalrunq
	...
	// 如果有空闲的p 且 m没有处于自旋状态 且 main goroutine已经启动，那么唤醒某个m来执行任务
	if atomic.Load(&sched.npidle) != 0 && atomic.Load(&sched.nmspinning) == 0 && mainStarted {
		wakep()//为了执行g也是拼了，没有idle p的情况下，增加一个p
	}
```


问题6：  
runqput的代码流程  
```
func runqput(_p_ *p, gp *g, next bool) {
	if randomizeScheduler && next && fastrand()%2 == 0 {
		next = false
	}

	if next {
	retryNext:
		oldnext := _p_.runnext	// 优先进入runnext
		if !_p_.runnext.cas(oldnext, guintptr(unsafe.Pointer(gp))) {
			goto retryNext
		}
		if oldnext == 0 {
			return
		}
		// Kick the old runnext out to the regular run queue.
		gp = oldnext.ptr()	// gp现在已是老的next了
	}

retry:	// 将老的runnext放入localrunq
	h := atomic.LoadAcq(&_p_.runqhead) // load-acquire, synchronize with consumers
	t := _p_.runqtail	// 队尾
	if t-h < uint32(len(_p_.runq)) {	// 如果头-尾小于runq长度(256固定值),说明还有空位置
		_p_.runq[t%uint32(len(_p_.runq))].set(gp)// 将gp放入tail的后一个位置
		atomic.StoreRel(&_p_.runqtail, t+1) // store-release, makes the item available for consumption
		return
	}
	// slow的一波骚操作
	if runqputslow(_p_, gp, h, t) {
		return
	}
	// the queue is not full, now the put above must succeed
	goto retry
}
...
// Put g and a batch of work from local runnable queue on global queue.
// Executed only by the owner P.
func runqputslow(_p_ *p, gp *g, h, t uint32) bool {
	var batch [len(_p_.runq)/2 + 1]*g

	// 先从本地队列搞出来前半部分放在batch里，注意n是有效数组长度的一半
	// First, grab a batch from local queue.
	n := t - h
	n = n / 2
	if n != uint32(len(_p_.runq)/2) {
		throw("runqputslow: queue is not full")
	}
	for i := uint32(0); i < n; i++ {
		batch[i] = _p_.runq[(h+i)%uint32(len(_p_.runq))].ptr()
	}
	// 因为前一半已经要被拿走了，所以后一半要往前挪
	if !atomic.CasRel(&_p_.runqhead, h, h+n) { // cas-release, commits consume
		return false
	}
	batch[n] = gp	// 把当前这个放在最后一个，然后统一打乱

	// 重新排序
	if randomizeScheduler {
		for i := uint32(1); i <= n; i++ {
			j := fastrandn(i + 1)
			batch[i], batch[j] = batch[j], batch[i]
		}
	}

	// 这里为啥要连起来？因为global的runq不是一个数组，是一个链表
	// Link the goroutines.
	for i := uint32(0); i < n; i++ {
		batch[i].schedlink.set(batch[i+1])
	}
	var q gQueue
	q.head.set(batch[0])
	q.tail.set(batch[n])

	// 最终放到global里，注意这里有加锁
	// Now put the batch on global queue.
	lock(&sched.lock)
	globrunqputbatch(&q, int32(n+1))
	unlock(&sched.lock)
	return true
}
...
// Put gp on the global runnable queue.
// Sched must be locked.
// May run during STW, so write barriers are not allowed.
//go:nowritebarrierrec
func globrunqput(gp *g) {//就是放到最后一个
	sched.runq.pushBack(gp)
	sched.runqsize++
}
```

问题7：  
runqget的流程  
```
func runqget(_p_ *p) (gp *g, inheritTime bool) {
	// If there's a runnext, it's the next G to run.
	for {
		next := _p_.runnext		// 优先runnext
		if next == 0 {
			break
		}
		if _p_.runnext.cas(next, 0) {
			return next.ptr(), true
		}
	}

	for {
		h := atomic.LoadAcq(&_p_.runqhead) // load-acquire, synchronize with other consumers
		t := _p_.runqtail
		if t == h {
			return nil, false
		}
		// 这里取的是localrunq里的第一个
		gp := _p_.runq[h%uint32(len(_p_.runq))].ptr()
		if atomic.CasRel(&_p_.runqhead, h, h+1) { // cas-release, commits consume
			return gp, false
		}
	}
}
```

问题8：  
globrunqget流程  
```
// Try get a batch of G's from the global runnable queue.
// Sched must be locked.
// 这里一般是找了个m发现本地队列里没值所致，到global里来找
func globrunqget(_p_ *p, max int32) *g {
	if sched.runqsize == 0 {
		return nil
	}
	// n = min(len(GQ)/GOMAXPROCS+1, len(GQ/2))
	n := sched.runqsize/gomaxprocs + 1
	if n > sched.runqsize {
		n = sched.runqsize
	}
	if max > 0 && n > max {
		n = max
	}
	if n > int32(len(_p_.runq))/2 {
		n = int32(len(_p_.runq)) / 2
	}

	sched.runqsize -= n
	// 拿出来1个
	gp := sched.runq.pop()
	n--
	// 然后把一坨都扔local队列里了
	for ; n > 0; n-- {
		gp1 := sched.runq.pop()
		runqput(_p_, gp1, false)
	}
	return gp
}
```

问题9：  
schedule流程和execute流程  
```
//为了找一个g也是煞费苦心
// One round of scheduler: find a runnable goroutine and execute it.
// Never returns.
func schedule() {
	_g_ := getg()

	if _g_.m.locks != 0 {
		throw("schedule: holding locks")
	}

	if _g_.m.lockedg != 0 {
		stoplockedm()
		execute(_g_.m.lockedg.ptr(), false) // Never returns.
	}

	// We should not schedule away from a g that is executing a cgo call,
	// since the cgo call is using the m's g0 stack.
	if _g_.m.incgo {
		throw("schedule: in cgo")
	}

top:
	pp := _g_.m.p.ptr()
	pp.preempt = false
	// 如果当前GC需要停止整个世界（STW), 则调用gcstopm休眠当前的M
	if sched.gcwaiting != 0 {
		gcstopm()
		goto top
	}
	if pp.runSafePointFn != 0 {
		runSafePointFn()
	}

	// Sanity check: if we are spinning, the run queue should be empty.
	// Check this before calling checkTimers, as that might call
	// goready to put a ready goroutine on the local run queue.
	if _g_.m.spinning && (pp.runnext != 0 || pp.runqhead != pp.runqtail) {
		throw("schedule: spinning with local work")
	}

	checkTimers(pp, 0)

	var gp *g
	var inheritTime bool

	// Normal goroutines will check for need to wakeP in ready,
	// but GCworkers and tracereaders will not, so the check must
	// be done here instead.
	tryWakeP := false
	if trace.enabled || trace.shutdown {
		gp = traceReader()
		if gp != nil {
			casgstatus(gp, _Gwaiting, _Grunnable)
			traceGoUnpark(gp, 0)
			tryWakeP = true
		}
	}
	if gp == nil && gcBlackenEnabled != 0 {
		gp = gcController.findRunnableGCWorker(_g_.m.p.ptr())
		tryWakeP = tryWakeP || gp != nil
	}
	if gp == nil {
		// Check the global runnable queue once in a while to ensure fairness.
		// Otherwise two goroutines can completely occupy the local runqueue
		// by constantly respawning each other.
		// 61是个魔数，每61次会去global里找一个item，防止global里一直得不到调度
		if _g_.m.p.ptr().schedtick%61 == 0 && sched.runqsize > 0 {
			lock(&sched.lock)
			// 只去global队列里找1个
			gp = globrunqget(_g_.m.p.ptr(), 1)
			unlock(&sched.lock)
		}
	}
	if gp == nil {
		//从与m关联的p的本地运行队列中获取goroutine
		gp, inheritTime = runqget(_g_.m.p.ptr())
		// We can see gp != nil here even if the M is spinning,
		// if checkTimers added a local goroutine via goready.
	}
	if gp == nil {
		//如果从本地运行队列和全局运行队列都没有找到需要运行的goroutine，
		//则调用findrunnable函数从其它工作线程的运行队列中偷取，如果偷取不到，则当前工作线程(毕竟schedule是mstart进来的)进入睡眠，
		//直到获取到需要运行的goroutine之后findrunnable函数才会返回
		gp, inheritTime = findrunnable() // blocks until work is available
	}

	// This thread is going to run a goroutine and is not spinning anymore,
	// so if it was marked as spinning we need to reset it now and potentially
	// start a new spinning M.
	//偷窃状态的goroutine会进入spinning状态，重置状态，才能让m执行goroutine
	if _g_.m.spinning {
		resetspinning()
	}

	if sched.disable.user && !schedEnabled(gp) {
		// Scheduling of this goroutine is disabled. Put it on
		// the list of pending runnable goroutines for when we
		// re-enable user scheduling and look again.
		lock(&sched.lock)
		if schedEnabled(gp) {
			// Something re-enabled scheduling while we
			// were acquiring the lock.
			unlock(&sched.lock)
		} else {
			sched.disable.runnable.pushBack(gp)
			sched.disable.n++
			unlock(&sched.lock)
			goto top
		}
	}

	// If about to schedule a not-normal goroutine (a GCworker or tracereader),
	// wake a P if there is one.
	if tryWakeP {
		if atomic.Load(&sched.npidle) != 0 && atomic.Load(&sched.nmspinning) == 0 {
			wakep()
		}
	}
	if gp.lockedm != 0 {
		// Hands off own p to the locked m,
		// then blocks waiting for a new p.
		startlockedm(gp)
		goto top
	}
	// 找到了g，那就执行g上的任务函数
	execute(gp, inheritTime)
}
...
func execute(gp *g, inheritTime bool) {
	_g_ := getg()

	// Assign gp.m before entering _Grunning so running Gs have an
	// M.
	_g_.m.curg = gp
	gp.m = _g_.m
	casgstatus(gp, _Grunnable, _Grunning)
	gp.waitsince = 0
	gp.preempt = false
	gp.stackguard0 = gp.stack.lo + _StackGuard
	if !inheritTime {
		_g_.m.p.ptr().schedtick++
	}

	// Check whether the profiler needs to be turned on or off.
	hz := sched.profilehz
	if _g_.m.profilehz != hz {
		setThreadCPUProfiler(hz)
	}

	if trace.enabled {
		// GoSysExit has to happen when we have a P, but before GoStart.
		// So we emit it here.
		if gp.syscallsp != 0 && gp.sysblocktraced {
			traceGoSysExit(gp.sysexitticks)
		}
		traceGoStart()
	}
	//执行go func()中func(),
	//把 g 对象的 gobuf 里的内容搬到寄存器里。
	//然后从 gobuf.pc 寄存器存储的指令位置开始继续向后执行
	// 在proc.go的newproc1里有一句话
	// newg.sched.pc = funcPC(goexit) + sys.PCQuantum
	// goexit放到了pc的栈顶，保证函数执行完后能执行runtime.goexit 
	gogo(&gp.sched)
}
```

问题10：findrunnable流程  
```
// 找到一个可执行的 goroutine 来 execute
// 会尝试从其它的 P 那里偷 g，从全局队列中拿，或者 network 中 poll
func findrunnable() (gp *g, inheritTime bool) {
	_g_ := getg()

	// The conditions here and in handoffp must agree: if
	// findrunnable would return a G to run, handoffp must start
	// an M.

top:
	_p_ := _g_.m.p.ptr()
	if sched.gcwaiting != 0 {
		gcstopm()
		goto top
	}
	if _p_.runSafePointFn != 0 {
		runSafePointFn()
	}
	if fingwait && fingwake {
		if gp := wakefing(); gp != nil {
			ready(gp, 0, true)
		}
	}
	if *cgo_yield != nil {
		asmcgocall(*cgo_yield, nil)
	}

	// 从本地队列中获取
	if gp, inheritTime := runqget(_p_); gp != nil {
		return gp, inheritTime
	}

	// 从全局队列中获取
	if sched.runqsize != 0 {
		lock(&sched.lock)
		gp := globrunqget(_p_, 0)
		unlock(&sched.lock)
		if gp != nil {
			return gp, false
		}
	}

	// Poll network.
	// This netpoll is only an optimization before we resort to stealing.
	// We can safely skip it if there are no waiters or a thread is blocked
	// in netpoll already. If there is any kind of logical race with that
	// blocked thread (e.g. it has already returned from netpoll, but does
	// not set lastpoll yet), this thread will do blocking netpoll below
	// anyway.
	// 从网络IO轮询器中找到就绪的G，把这个G变为可运行的G
	if netpollinited() && atomic.Load(&netpollWaiters) > 0 && atomic.Load64(&sched.lastpoll) != 0 {
		if list := netpoll(false); !list.empty() { // non-blocking
			gp := list.pop()
			injectglist(&list)
			casgstatus(gp, _Gwaiting, _Grunnable)
			if trace.enabled {
				traceGoUnpark(gp, 0)
			}
			return gp, false
		}
	}

	// Steal work from other P's.
	procs := uint32(gomaxprocs)
	// 如果其他P都是空闲的，就不从其他P哪里偷取G了
	if atomic.Load(&sched.npidle) == procs-1 {
		// Either GOMAXPROCS=1 or everybody, except for us, is idle already.
		// New work can appear from returning syscall/cgocall, network or timers.
		// Neither of that submits to local run queues, so no point in stealing.
		goto stop
	}
	// If number of spinning M's >= number of busy P's, block.
	// This is necessary to prevent excessive CPU consumption
	// when GOMAXPROCS>>1 but the program parallelism is low.
	// 如果当前的M没在自旋 且 空闲P的数目小于正在自旋的M个数的2倍，那么让该M进入自旋状态(自旋也是要消耗cpu的，不能有太多的自旋)
	// 自旋线程是抢占g的，不是抢占p的，比如阻塞场景，g和m被休眠，p找的是休眠m，而不是自旋m
	// 所以这里忙的p如果很多，那就尽量让p去占用cpu资源，不要自旋查找g。
	if !_g_.m.spinning && 2*atomic.Load(&sched.nmspinning) >= procs-atomic.Load(&sched.npidle) {
		goto stop
	}
	// 如果M为非自旋，那么设置为自旋状态
	if !_g_.m.spinning {
		_g_.m.spinning = true
		atomic.Xadd(&sched.nmspinning, 1)
	}
	// 尝试4次从别的P偷,注意是随机选一个
	for i := 0; i < 4; i++ {
		for enum := stealOrder.start(fastrand()); !enum.done(); enum.next() {
			if sched.gcwaiting != 0 {
				goto top
			}
			stealRunNextG := i > 2 // first look for ready queues with more than 1 g
			// 在这里开始针对P进行偷取操作，从allp[enum.position()]偷去一半的G，并返回其中的一个
			if gp := runqsteal(_p_, allp[enum.position()], stealRunNextG); gp != nil {
				return gp, false
			}
		}
	}

stop:

	// We have nothing to do. If we're in the GC mark phase, can
	// safely scan and blacken objects, and have work to do, run
	// idle-time marking rather than give up the P.
	// 当前的M找不到G来运行。如果此时P处于 GC mark 阶段,就进行垃圾回收的标记工作；
	if gcBlackenEnabled != 0 && _p_.gcBgMarkWorker != 0 && gcMarkWorkAvailable(_p_) {
		_p_.gcMarkWorkerMode = gcMarkWorkerIdleMode
		gp := _p_.gcBgMarkWorker.ptr()
		casgstatus(gp, _Gwaiting, _Grunnable)
		if trace.enabled {
			traceGoUnpark(gp, 0)
		}
		return gp, false
	}

	// wasm only:
	// If a callback returned and no other goroutine is awake,
	// then pause execution until a callback was triggered.
	if beforeIdle() {
		// At least one goroutine got woken.
		goto top
	}

	// Before we drop our P, make a snapshot of the allp slice,
	// which can change underfoot once we no longer block
	// safe-points. We don't need to snapshot the contents because
	// everything up to cap(allp) is immutable.
	allpSnapshot := allp

	// return P and block
	lock(&sched.lock)
	if sched.gcwaiting != 0 || _p_.runSafePointFn != 0 {
		unlock(&sched.lock)
		goto top
	}
	// 再次从全局队列中获取G
	if sched.runqsize != 0 {
		gp := globrunqget(_p_, 0)
		unlock(&sched.lock)
		return gp, false
	}
	// 将当前对M和P解绑
	if releasep() != _p_ {
		throw("findrunnable: wrong p")
	}
	// 将p放入p空闲链表
	pidleput(_p_)
	unlock(&sched.lock)

	// Delicate dance: thread transitions from spinning to non-spinning state,
	// potentially concurrently with submission of new goroutines. We must
	// drop nmspinning first and then check all per-P queues again (with
	// #StoreLoad memory barrier in between). If we do it the other way around,
	// another thread can submit a goroutine after we've checked all run queues
	// but before we drop nmspinning; as the result nobody will unpark a thread
	// to run the goroutine.
	// If we discover new work below, we need to restore m.spinning as a signal
	// for resetspinning to unpark a new worker thread (because there can be more
	// than one starving goroutine). However, if after discovering new work
	// we also observe no idle Ps, it is OK to just park the current thread:
	// the system is fully loaded so no spinning threads are required.
	// Also see "Worker thread parking/unparking" comment at the top of the file.
	wasSpinning := _g_.m.spinning
	// M取消自旋状态
	if _g_.m.spinning {
		_g_.m.spinning = false
		if int32(atomic.Xadd(&sched.nmspinning, -1)) < 0 {
			throw("findrunnable: negative nmspinning")
		}
	}

	// check all runqueues once again
	// 再次检查所有的P，有没有可以运行的G
	for _, _p_ := range allpSnapshot {
		// 如果p的本地队列有G
		if !runqempty(_p_) {
			lock(&sched.lock)
			// 获取另外一个空闲P
			_p_ = pidleget()
			unlock(&sched.lock)
			if _p_ != nil {
				// 如果P不是nil，将M绑定P
				acquirep(_p_)
				// 如果是自旋，设置M为自旋
				if wasSpinning {
					_g_.m.spinning = true
					atomic.Xadd(&sched.nmspinning, 1)
				}
				// 返回到函数开头，从本地p获取G
				goto top
			}
			break
		}
	}

	// Check for idle-priority GC work again.
	if gcBlackenEnabled != 0 && gcMarkWorkAvailable(nil) {
		lock(&sched.lock)
		_p_ = pidleget()
		if _p_ != nil && _p_.gcBgMarkWorker == 0 {
			pidleput(_p_)
			_p_ = nil
		}
		unlock(&sched.lock)
		if _p_ != nil {
			acquirep(_p_)
			if wasSpinning {
				_g_.m.spinning = true
				atomic.Xadd(&sched.nmspinning, 1)
			}
			// Go back to idle GC check.
			goto stop
		}
	}

	// poll network
	// 再次检查netpoll
	if netpollinited() && atomic.Load(&netpollWaiters) > 0 && atomic.Xchg64(&sched.lastpoll, 0) != 0 {
		if _g_.m.p != 0 {
			throw("findrunnable: netpoll with p")
		}
		if _g_.m.spinning {
			throw("findrunnable: netpoll with spinning")
		}
		list := netpoll(true) // block until new work is available
		atomic.Store64(&sched.lastpoll, uint64(nanotime()))
		if !list.empty() {
			lock(&sched.lock)
			_p_ = pidleget()
			unlock(&sched.lock)
			if _p_ != nil {
				acquirep(_p_)
				gp := list.pop()
				injectglist(&list)
				casgstatus(gp, _Gwaiting, _Grunnable)
				if trace.enabled {
					traceGoUnpark(gp, 0)
				}
				return gp, false
			}
			injectglist(&list)
		}
	}
	// 实在找不到G，那就休眠吧
	// 且此时的M一定不是自旋状态
	stopm()
	goto top
}
```
调用runqget()从当前P的队列中取G（和schedule()中的调用相同），获取到就return  
获取不到就全局队列中获取，获取到了就return  
尝试从poll中获取，获取得到也return  
获取不到就尝试从其他的p中获取，找数量超过1个的，找得到就返回，runqsteal  
如果处于垃圾回收标记阶段，就进行垃圾回收的标记工作；  
再次调用globrunqget()从全局队列中取可执行的G，获取的到就返回 return  
再次检查所有的runqueues，如果有返回到最开始的top  
没有就做gc方面的工作，然次从poll获取，获取的到就return，获取不到就然后调用stopm  
stopm的核心是调用mput把m结构体对象放入sched的midle空闲队列，然后通过notesleep(&m.park)函数让自己进入睡眠状态。  
唤醒后在再次跳转到top  


问题11：  
sysmon的流程  
```
// Always runs without a P, so write barriers are not allowed.
//没有绑定p的，wirte barriers都是不被允许的，和gc机制相关
//go:nowritebarrierrec

func sysmon() {
	lock(&sched.lock)
	sched.nmsys++
	// 判断程序是否死锁
	checkdead()
	unlock(&sched.lock)

	// If a heap span goes unused for 5 minutes after a garbage collection,
	// we hand it back to the operating system.
	scavengelimit := int64(5 * 60 * 1e9)

	if debug.scavenge > 0 {
		// Scavenge-a-lot for testing.
		forcegcperiod = 10 * 1e6
		scavengelimit = 20 * 1e6
	}

	lastscavenge := nanotime()
	nscavenge := 0

	lasttrace := int64(0)
	idle := 0 // how many cycles in succession we had not wokeup somebody
	delay := uint32(0)
	for {
		if idle == 0 { // 初始化时 20us sleep.
			delay = 20
		} else if idle > 50 { // start doubling the sleep after 1ms...
			delay *= 2 //执行一次后等待的时间会翻倍
		}
		if delay > 10*1000 { //最长10ms
			delay = 10 * 1000
		}
		usleep(delay)
		if debug.schedtrace <= 0 && (sched.gcwaiting != 0 || atomic.Load(&sched.npidle) == uint32(gomaxprocs)) {
			lock(&sched.lock)
			if atomic.Load(&sched.gcwaiting) != 0 || atomic.Load(&sched.npidle) == uint32(gomaxprocs) {
				atomic.Store(&sched.sysmonwait, 1)
				unlock(&sched.lock)
				// Make wake-up period small enough
				// for the sampling to be correct.
				maxsleep := forcegcperiod / 2
				if scavengelimit < forcegcperiod {
					maxsleep = scavengelimit / 2
				}
				shouldRelax := true
				if osRelaxMinNS > 0 {
					next := timeSleepUntil()
					now := nanotime()
					if next-now < osRelaxMinNS {
						shouldRelax = false
					}
				}
				if shouldRelax {
					osRelax(true)
				}
				notetsleep(&sched.sysmonnote, maxsleep)
				if shouldRelax {
					osRelax(false)
				}
				lock(&sched.lock)
				atomic.Store(&sched.sysmonwait, 0)
				noteclear(&sched.sysmonnote)
				idle = 0
				delay = 20
			}
			unlock(&sched.lock)
		}
		// trigger libc interceptors if needed
		if *cgo_yield != nil {
			asmcgocall(*cgo_yield, nil)
		}
	   // 如果 10ms 没有 poll 过 network，那么就 netpoll 一次
		lastpoll := int64(atomic.Load64(&sched.lastpoll))
		now := nanotime()
		if netpollinited() && lastpoll != 0 && lastpoll+10*1000*1000 < now {
			atomic.Cas64(&sched.lastpoll, uint64(lastpoll), uint64(now))
			//netpoll中会执行epollWait，epollWait返回可读写的fd
			//netpoll返回可读写的fd关联的协程
			list := netpoll(false) // non-blocking - returns list of goroutines
			if !list.empty() {
				// Need to decrement number of idle locked M's
				// (pretending that one more is running) before injectglist.
				// Otherwise it can lead to the following situation:
				// injectglist grabs all P's but before it starts M's to run the P's,
				// another M returns from syscall, finishes running its G,
				// observes that there is no work to do and no other running M's
				// and reports deadlock.
				incidlelocked(-1)
				//将可读写fd关联的协程状态设置为ready
				injectglist(&list)
				incidlelocked(1)
			}
		}
		// retake P's blocked in syscalls
		// and preempt long running G's
		// 抢夺阻塞时间长的syscall的G
		if retake(now) != 0 {
			idle = 0
		} else {
			idle++
		}
		// 检查是否需要强制gc，两分钟一次
		if t := (gcTrigger{kind: gcTriggerTime, now: now}); t.test() && atomic.Load(&forcegc.idle) != 0 {
			lock(&forcegc.lock)
			forcegc.idle = 0
			var list gList
			list.push(forcegc.g)
			injectglist(&list)
			unlock(&forcegc.lock)
		}
		// 清理内存
		if lastscavenge+scavengelimit/2 < now {
			mheap_.scavenge(int32(nscavenge), uint64(now), uint64(scavengelimit))
			lastscavenge = now
			nscavenge++
		}
		if debug.schedtrace > 0 && lasttrace+int64(debug.schedtrace)*1000000 <= now {
			lasttrace = now
			schedtrace(debug.scheddetail > 0)
		}
	}
}
```
检查checkdead ，检查goroutine锁死，启动时检查一次。  
处理netpoll返回，injectglist将协程的状态设置为ready状态  
强制gc  
收回因为 syscall 而长时间阻塞的 p，同时抢占那些执行时间过长的 g，retake(now)  
如果 堆 内存闲置超过 5min，那么释放掉，mheap_.scavenge(int32(nscavenge), uint64(now), uint64(scavengelimit))  
不能接管的阻塞就得靠sysmon来剥离搞定  



附加：
无锁队列就是用atomic来确保并发安全  
加锁一定会阻塞  
atomic是for循环，理论上也会阻塞，但要轻  
为什么有了p就不需要加锁？因为本质并发是多线程，一个m只能操作一个p  
handoffp是为了m执行g，里边因为调用cgo或者syscall之类的阻塞很久，能够把p从m上剥离掉  
另外就是发信号干掉g，让g在某个地方等待  
创建 goroutine 结构体时，如果可用的 g 和 sudog 结构体能够复用，会优先进行复用。每个p都有一个gFree的结构，以及全局gFree  
全局有一个allgs的结构  
每次遇到阻塞，如果没有可用的m，就会新启动一个m  
LockOSThread是一个比较特殊的接口，可以让一个g绑定一个thread来跑，一旦没有unlock则thread会退出  


m0 表示进程启动的第一个线程，也叫主线程。  
它和其他的m没有什么区别，要说区别的话，它是进程启动通过汇编直接复制给m0的，  
m0是个全局变量，而其他的m都是runtime内自己创建的。  
m0 的赋值过程，可以看前面 runtime/asm_amd64.s 的代码。一个go进程只有一个m0。  

g0  
首先要明确的是每个m都有一个g0，因为每个线程有一个系统堆栈，  
g0 虽然也是g的结构，但和普通的g还是有差别的，最重要的差别就是栈的差别。  
g0 上的栈是系统分配的栈，在linux上栈大小默认固定8MB，不能扩展，也不能缩小。  
而普通g一开始只有2KB大小，可扩展。  
在 g0 上也没有任何任务函数，也没有任何状态，并且它不能被调度程序抢占。因为调度就是在g0上跑的。  

mstart 只是简单的对 mstart1 的封装，接着看 mstart1 。  
1. 调用 _g_ := getg() 获取 g ，然后检查该 g 是不是 g0 ，如果不是 g0 ，直接抛出异常。  
2. 初始化m，主要是设置线程的备用信号堆栈和信号掩码  
3. 判断 g 绑定的 m 是不是 m0，如果是，需要一些特殊处理，主要是设置系统信号量的处理函数  
4. 检查 m 是否有起始任务函数？若有，执行它。  
5. 获取一个p，并和m绑定  
6. 进入调度程序  

m0代表主线程、g0代表了线程的堆栈。调度都是在系统堆栈上跑的，也就是一定要跑在 g0 上，所以 mstart1 函数才检查是不是在g0上， 因为接下来就要执行调度程序了。  

g0的目的是用于调度其他g，m在切换其他g的时候，需要先把g0拿过来，g0提供上下文的环境，帮忙调度到其他的g当中  
比如上一个g是g1，g1退出时需要调用goexit()，然后m加载g0，通过g0加载环境信息，负责协程切换schedule()函数，然后通过g0把g2(下一个g)调度过来  

唤醒一个m后如果本地队列没有可执行的g，则m会自旋一段时间，然后去全局队列里取可用的g  

没g了就调用g0,来找别的g  



长期休眠的m，最终会被gc销毁(存疑https://github.com/golang/go/issues/14592)  




![Image](https://github.com/stubbornTanzhe/images/blob/main/gmp.JPG)
			  
