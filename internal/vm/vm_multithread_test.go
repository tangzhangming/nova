// Package vm 实现了 Sola 编程语言的字节码虚拟机。
//
// 本文件包含多线程 VM 的测试用例。
// 运行时使用 -race 标志检测竞态条件：
//
//	go test -race -v ./internal/vm/...
package vm

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tangzhangming/nova/internal/bytecode"
)

// ============================================================================
// 工作线程池测试
// ============================================================================

func TestWorkerPoolCreation(t *testing.T) {
	vm := New()
	pool := NewWorkerPool(vm, 4)

	if pool.NumWorkers() != 4 {
		t.Errorf("Expected 4 workers, got %d", pool.NumWorkers())
	}

	for i := 0; i < 4; i++ {
		w := pool.GetWorker(i)
		if w == nil {
			t.Errorf("Worker %d should not be nil", i)
		}
		if w.ID() != i {
			t.Errorf("Worker ID should be %d, got %d", i, w.ID())
		}
	}
}

func TestWorkerPoolAutoDetectCPU(t *testing.T) {
	vm := New()
	pool := NewWorkerPool(vm, 0) // 0 = auto detect

	expected := runtime.NumCPU()
	if pool.NumWorkers() != expected {
		t.Errorf("Expected %d workers (NumCPU), got %d", expected, pool.NumWorkers())
	}
}

func TestWorkerPoolStartStop(t *testing.T) {
	vm := New()
	pool := NewWorkerPool(vm, 2)

	if pool.IsRunning() {
		t.Error("Pool should not be running before Start()")
	}

	pool.Start()
	if !pool.IsRunning() {
		t.Error("Pool should be running after Start()")
	}

	// 等待 Workers 启动
	time.Sleep(10 * time.Millisecond)

	pool.Stop()
	if pool.IsRunning() {
		t.Error("Pool should not be running after Stop()")
	}
}

// ============================================================================
// 共享状态测试
// ============================================================================

func TestSharedStateGlobals(t *testing.T) {
	state := NewSharedState()

	// 测试基本操作
	state.SetGlobal("test", bytecode.NewInt(42))

	val, ok := state.GetGlobal("test")
	if !ok {
		t.Error("Global 'test' should exist")
	}
	if val.AsInt() != 42 {
		t.Errorf("Expected 42, got %d", val.AsInt())
	}

	// 测试不存在的键
	_, ok = state.GetGlobal("nonexistent")
	if ok {
		t.Error("Global 'nonexistent' should not exist")
	}

	// 测试删除
	state.DeleteGlobal("test")
	_, ok = state.GetGlobal("test")
	if ok {
		t.Error("Global 'test' should be deleted")
	}
}

func TestSharedStateGlobalsConcurrent(t *testing.T) {
	state := NewSharedState()

	var wg sync.WaitGroup
	numGoroutines := 100
	numOps := 1000

	// 并发写入
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				key := "key_" + string(rune('a'+id%26))
				state.SetGlobal(key, bytecode.NewInt(int64(id*numOps+j)))
			}
		}(i)
	}

	// 并发读取
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				key := "key_" + string(rune('a'+id%26))
				state.GetGlobal(key)
			}
		}(i)
	}

	wg.Wait()
}

func TestSharedStateFreeze(t *testing.T) {
	state := NewSharedState()

	// 在冻结前可以添加类
	class := &bytecode.Class{Name: "TestClass"}
	if !state.DefineClass(class) {
		t.Error("Should be able to define class before freeze")
	}

	// 冻结
	state.Freeze()

	if !state.IsFrozen() {
		t.Error("State should be frozen")
	}

	// 冻结后不能添加类
	class2 := &bytecode.Class{Name: "TestClass2"}
	if state.DefineClass(class2) {
		t.Error("Should not be able to define class after freeze")
	}

	// 但仍然可以读取
	if state.GetClass("TestClass") == nil {
		t.Error("Should be able to read class after freeze")
	}
}

// ============================================================================
// 多线程调度器测试
// ============================================================================

func TestMultiThreadSchedulerSpawn(t *testing.T) {
	vm := New()
	pool := NewWorkerPool(vm, 2)
	sched := pool.GetScheduler()

	// 创建一个简单的闭包
	fn := &bytecode.Function{
		Name:  "test",
		Arity: 0,
		Chunk: bytecode.NewChunk(),
	}
	closure := &bytecode.Closure{Function: fn}

	// Spawn 协程
	g := sched.Spawn(closure, nil)
	if g == nil {
		t.Error("Spawn should return a goroutine")
	}

	if sched.GoroutineCount() != 1 {
		t.Errorf("Expected 1 goroutine, got %d", sched.GoroutineCount())
	}

	// 终止协程
	sched.Kill(g)
	if sched.GoroutineCount() != 0 {
		t.Errorf("Expected 0 goroutines after kill, got %d", sched.GoroutineCount())
	}
}

func TestMultiThreadSchedulerGlobalQueue(t *testing.T) {
	vm := New()
	pool := NewWorkerPool(vm, 2)
	sched := pool.GetScheduler()

	fn := &bytecode.Function{
		Name:  "test",
		Arity: 0,
		Chunk: bytecode.NewChunk(),
	}
	closure := &bytecode.Closure{Function: fn}

	// 提交多个协程
	numGoroutines := 100
	for i := 0; i < numGoroutines; i++ {
		g := sched.Spawn(closure, nil)
		if g == nil {
			t.Errorf("Spawn %d failed", i)
		}
	}

	if sched.GoroutineCount() != int32(numGoroutines) {
		t.Errorf("Expected %d goroutines, got %d", numGoroutines, sched.GoroutineCount())
	}
}

// ============================================================================
// 工作窃取测试
// ============================================================================

func TestWorkerLocalQueue(t *testing.T) {
	vm := New()
	pool := NewWorkerPool(vm, 2)
	worker := pool.GetWorker(0)

	fn := &bytecode.Function{
		Name:  "test",
		Arity: 0,
		Chunk: bytecode.NewChunk(),
	}
	closure := &bytecode.Closure{Function: fn}

	// 创建协程并加入本地队列
	g1 := &Goroutine{ID: 1}
	g1.PushFrame(CallFrame{Closure: closure})

	if !worker.pushLocal(g1) {
		t.Error("pushLocal should succeed")
	}

	if worker.LocalQueueLen() != 1 {
		t.Errorf("Expected local queue len 1, got %d", worker.LocalQueueLen())
	}

	// 弹出
	g := worker.popLocal()
	if g != g1 {
		t.Error("popLocal should return the same goroutine")
	}

	if worker.LocalQueueLen() != 0 {
		t.Errorf("Expected local queue len 0, got %d", worker.LocalQueueLen())
	}
}

func TestWorkerSteal(t *testing.T) {
	vm := New()
	pool := NewWorkerPool(vm, 2)
	worker0 := pool.GetWorker(0)
	worker1 := pool.GetWorker(1)

	// 向 worker0 添加任务
	for i := 0; i < 10; i++ {
		g := &Goroutine{ID: int64(i)}
		worker0.pushLocal(g)
	}

	// worker1 从 worker0 窃取
	stolen := worker1.stealFrom(worker0)
	if stolen == nil {
		t.Error("Should be able to steal from worker0")
	}

	if worker0.LocalQueueLen() != 9 {
		t.Errorf("worker0 should have 9 tasks after steal, got %d", worker0.LocalQueueLen())
	}
}

// ============================================================================
// STW 测试
// ============================================================================

func TestSTW(t *testing.T) {
	vm := New()
	pool := NewWorkerPool(vm, 4)
	gc := NewMultiThreadGC(pool)
	gc.SetScheduler(pool.GetScheduler())

	// 启动线程池
	pool.Start()
	defer pool.Stop()

	// 等待 workers 启动
	time.Sleep(10 * time.Millisecond)

	// 请求 STW
	startTime := time.Now()
	gc.RequestSTW()

	if !gc.IsSTWActive() {
		t.Error("STW should be active after RequestSTW")
	}

	// 释放 STW
	gc.ReleaseSTW()

	if gc.IsSTWActive() {
		t.Error("STW should not be active after ReleaseSTW")
	}

	// 检查 STW 统计
	stats := gc.STWStats()
	if stats.STWCount != 1 {
		t.Errorf("Expected 1 STW, got %d", stats.STWCount)
	}

	duration := time.Since(startTime)
	t.Logf("STW duration: %v", duration)
}

// ============================================================================
// Channel 并发安全测试
// ============================================================================

func TestSafeChannelConcurrent(t *testing.T) {
	ch := NewSafeChannel("int", 10)
	vm := New()
	pool := NewWorkerPool(vm, 4)
	sched := pool.GetScheduler()

	var wg sync.WaitGroup
	numSenders := 10
	numReceivers := 10
	numMessages := 100

	var sentCount atomic.Int64
	var recvCount atomic.Int64

	// 启动发送者
	for i := 0; i < numSenders; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numMessages; j++ {
				g := &Goroutine{ID: int64(id*1000 + j)}
				ok, blocked := ch.SendSafe(bytecode.NewInt(int64(j)), g, sched)
				if ok && !blocked {
					sentCount.Add(1)
				}
			}
		}(i)
	}

	// 启动接收者
	for i := 0; i < numReceivers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numMessages; j++ {
				g := &Goroutine{ID: int64(10000 + id*1000 + j)}
				_, ok, blocked := ch.ReceiveSafe(g, sched)
				if ok && !blocked {
					recvCount.Add(1)
				}
			}
		}(i)
	}

	wg.Wait()

	stats := ch.Stats()
	t.Logf("Channel stats: sent=%d, recv=%d, blocked=%d",
		stats.SendCount, stats.RecvCount, stats.BlockCount)
}

func TestSafeChannelClose(t *testing.T) {
	ch := NewSafeChannel("int", 0) // 无缓冲通道
	vm := New()
	pool := NewWorkerPool(vm, 2)
	sched := pool.GetScheduler()

	// 添加等待接收的协程
	g1 := &Goroutine{ID: 1}
	ch.RecvQueue = append(ch.RecvQueue, g1)

	// 关闭通道
	ch.CloseSafe(sched)

	if !ch.IsClosedSafe() {
		t.Error("Channel should be closed")
	}

	// 等待队列应该被清空
	if len(ch.RecvQueue) != 0 {
		t.Error("RecvQueue should be empty after close")
	}
}

// ============================================================================
// 压力测试
// ============================================================================

func TestWorkerPoolStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	vm := New()
	pool := NewWorkerPool(vm, runtime.NumCPU())
	sched := pool.GetScheduler()

	pool.Start()
	defer pool.Stop()

	fn := &bytecode.Function{
		Name:  "stress",
		Arity: 0,
		Chunk: bytecode.NewChunk(),
	}
	closure := &bytecode.Closure{Function: fn}

	// 创建大量协程
	numGoroutines := 10000
	var created atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g := sched.Spawn(closure, nil)
			if g != nil {
				created.Add(1)
			}
		}()
	}

	wg.Wait()

	t.Logf("Created %d goroutines", created.Load())

	stats := sched.Stats()
	t.Logf("Scheduler stats: goroutines=%d, spawned=%d, completed=%d",
		stats.GoroutineCount, stats.TotalSpawned, stats.TotalCompleted)
}

func TestGCStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	vm := New()
	pool := NewWorkerPool(vm, 4)
	gc := NewMultiThreadGC(pool)
	gc.SetScheduler(pool.GetScheduler())

	pool.Start()
	defer pool.Stop()

	// 并发执行 GC 和对象分配
	var wg sync.WaitGroup
	numWorkers := 4
	numAllocs := 1000

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numAllocs; j++ {
				// 分配对象
				arr := make([]bytecode.Value, 10)
				for k := range arr {
					arr[k] = bytecode.NewInt(int64(k))
				}

				// 偶尔触发 GC
				if j%100 == 0 {
					gc.TryCollect()
				}
			}
		}()
	}

	wg.Wait()

	stwStats := gc.STWStats()
	t.Logf("STW stats: count=%d, total=%dns, avg=%dns, max=%dns",
		stwStats.STWCount, stwStats.TotalSTWTimeNs, stwStats.AvgSTWTimeNs, stwStats.MaxSTWTimeNs)
}

// ============================================================================
// 死锁检测测试
// ============================================================================

func TestNoDeadlock(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping deadlock test in short mode")
	}

	vm := New()
	pool := NewWorkerPool(vm, 4)
	gc := NewMultiThreadGC(pool)
	gc.SetScheduler(pool.GetScheduler())

	pool.Start()

	// 设置超时
	done := make(chan bool)
	go func() {
		// 执行一系列可能导致死锁的操作
		for i := 0; i < 100; i++ {
			// STW
			gc.RequestSTW()
			time.Sleep(time.Millisecond)
			gc.ReleaseSTW()

			// 创建 Channel 并发送/接收
			ch := NewSafeChannel("int", 1)
			g := &Goroutine{ID: int64(i)}
			ch.SendSafe(bytecode.NewInt(int64(i)), g, pool.GetScheduler())
		}
		done <- true
	}()

	select {
	case <-done:
		// 成功完成
	case <-time.After(10 * time.Second):
		t.Error("Potential deadlock detected")
	}

	pool.Stop()
}

// ============================================================================
// 基准测试
// ============================================================================

func BenchmarkWorkerPoolSubmit(b *testing.B) {
	vm := New()
	pool := NewWorkerPool(vm, runtime.NumCPU())
	pool.Start()
	defer pool.Stop()

	fn := &bytecode.Function{
		Name:  "bench",
		Arity: 0,
		Chunk: bytecode.NewChunk(),
	}
	closure := &bytecode.Closure{Function: fn}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pool.GetScheduler().Spawn(closure, nil)
	}
}

func BenchmarkSharedStateGlobalAccess(b *testing.B) {
	state := NewSharedState()
	state.SetGlobal("test", bytecode.NewInt(42))

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			state.GetGlobal("test")
		}
	})
}

func BenchmarkSafeChannelSendRecv(b *testing.B) {
	ch := NewSafeChannel("int", 100)
	vm := New()
	pool := NewWorkerPool(vm, 4)
	sched := pool.GetScheduler()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		g := &Goroutine{ID: 1}
		for pb.Next() {
			if i%2 == 0 {
				ch.SendSafe(bytecode.NewInt(int64(i)), g, sched)
			} else {
				ch.ReceiveSafe(g, sched)
			}
			i++
		}
	})
}

func BenchmarkSTW(b *testing.B) {
	vm := New()
	pool := NewWorkerPool(vm, runtime.NumCPU())
	gc := NewMultiThreadGC(pool)
	gc.SetScheduler(pool.GetScheduler())

	pool.Start()
	defer pool.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gc.RequestSTW()
		gc.ReleaseSTW()
	}
}
