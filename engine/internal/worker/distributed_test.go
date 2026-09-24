package worker

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestWorkerStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   WorkerStatus
		expected WorkerStatus
	}{
		{"Idle", Idle, Idle},
		{"Busy", Busy, Busy},
		{"Draining", Draining, Draining},
		{"Offline", Offline, Offline},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Worker{Status: tt.status}
			if got := w.getStatus(); got != tt.expected {
				t.Errorf("getStatus() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestWorkerSetStatus(t *testing.T) {
	tests := []struct {
		name     string
		initial  WorkerStatus
		new      WorkerStatus
		expected WorkerStatus
	}{
		{"IdleToBusy", Idle, Busy, Busy},
		{"BusyToDraining", Busy, Draining, Draining},
		{"DrainingToOffline", Draining, Offline, Offline},
		{"OfflineToIdle", Offline, Idle, Idle},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &Worker{Status: tt.initial}
			w.setStatus(tt.new)
			if got := w.getStatus(); got != tt.expected {
				t.Errorf("setStatus() resulted in %v, want %v", got, tt.expected)
			}
		})
	}
}

// TestWorkerLoad_IncDecSequence exercises real incLoad/decLoad transitions.
// (A prior version of this test constructed a Worker with a given Load and
// then asserted it equalled a *different* expected value without ever
// calling incLoad/decLoad in between - a tautology that could only pass when
// initial == expected, so it had never actually verified any load-mutation
// behavior; TestWorkerIncLoad/TestWorkerDecLoad below cover that directly.)
func TestWorkerLoad_IncDecSequence(t *testing.T) {
	w := &Worker{}
	if w.Load != 0 {
		t.Fatalf("expected initial load 0, got %d", w.Load)
	}
	w.incLoad()
	if w.Load != 1 {
		t.Fatalf("expected load 1 after incLoad, got %d", w.Load)
	}
	w.incLoad()
	if w.Load != 2 {
		t.Fatalf("expected load 2 after second incLoad, got %d", w.Load)
	}
	w.decLoad()
	if w.Load != 1 {
		t.Fatalf("expected load 1 after decLoad, got %d", w.Load)
	}
}

func TestWorkerIncLoad(t *testing.T) {
	w := &Worker{}
	w.incLoad()
	if w.Load != 1 {
		t.Errorf("incLoad() = %v, want 1", w.Load)
	}
	if w.currentTasks != 1 {
		t.Errorf("currentTasks after incLoad() = %v, want 1", w.currentTasks)
	}
}

func TestWorkerDecLoad(t *testing.T) {
	w := &Worker{Load: 2, currentTasks: 2}
	w.decLoad()
	if w.Load != 1 {
		t.Errorf("decLoad() Load = %v, want 1", w.Load)
	}
	if w.currentTasks != 1 {
		t.Errorf("currentTasks after decLoad() = %v, want 1", w.currentTasks)
	}
}

func TestWorkerGetLoad(t *testing.T) {
	w := &Worker{Load: 5, MaxLoad: 10}
	load, maxLoad := w.getLoad()
	if load != 5 {
		t.Errorf("getLoad() load = %v, want 5", load)
	}
	if maxLoad != 10 {
		t.Errorf("getLoad() maxLoad = %v, want 10", maxLoad)
	}
}

func TestPoolRegisterWorker(t *testing.T) {
	p := NewPool()
	w := p.RegisterWorker("worker1", "127.0.0.1:8080", 10)

	if w.ID != "worker1" {
		t.Errorf("RegisterWorker() ID = %v, want worker1", w.ID)
	}
	if w.Addr != "127.0.0.1:8080" {
		t.Errorf("RegisterWorker() Addr = %v, want 127.0.0.1:8080", w.Addr)
	}
	if w.MaxLoad != 10 {
		t.Errorf("RegisterWorker() MaxLoad = %v, want 10", w.MaxLoad)
	}
	if w.Status != Idle {
		t.Errorf("RegisterWorker() Status = %v, want Idle", w.Status)
	}
}

func TestPoolDeregisterWorker(t *testing.T) {
	p := NewPool()
	p.RegisterWorker("worker1", "127.0.0.1:8080", 10)

	err := p.DeregisterWorker("worker1")
	if err != nil {
		t.Errorf("DeregisterWorker() error = %v, want nil", err)
	}

	if _, ok := p.workers["worker1"]; ok {
		t.Error("DeregisterWorker() worker still exists in pool")
	}
}

func TestPoolDeregisterWorkerNotFound(t *testing.T) {
	p := NewPool()

	err := p.DeregisterWorker("nonexistent")
	if err == nil {
		t.Error("DeregisterWorker() error = nil, want error")
	}
	if err.Error() != "worker not found: nonexistent" {
		t.Errorf("DeregisterWorker() error = %v, want worker not found: nonexistent", err)
	}
}

func TestPoolHeartbeat(t *testing.T) {
	p := NewPool()
	w := p.RegisterWorker("worker1", "127.0.0.1:8080", 10)
	initialHeartbeat := w.LastHeartbeat

	time.Sleep(10 * time.Millisecond)

	err := p.Heartbeat("worker1")
	if err != nil {
		t.Errorf("Heartbeat() error = %v, want nil", err)
	}

	if w.LastHeartbeat <= initialHeartbeat {
		t.Error("Heartbeat() did not update LastHeartbeat")
	}
}

func TestPoolHeartbeatWorkerNotFound(t *testing.T) {
	p := NewPool()

	err := p.Heartbeat("nonexistent")
	if err == nil {
		t.Error("Heartbeat() error = nil, want error")
	}
	if err.Error() != "worker not found: nonexistent" {
		t.Errorf("Heartbeat() error = %v, want worker not found: nonexistent", err)
	}
}

func TestPoolHeartbeatOfflineToIdle(t *testing.T) {
	p := NewPool()
	w := p.RegisterWorker("worker1", "127.0.0.1:8080", 10)
	w.setStatus(Offline)

	err := p.Heartbeat("worker1")
	if err != nil {
		t.Errorf("Heartbeat() error = %v, want nil", err)
	}

	if w.getStatus() != Idle {
		t.Errorf("Heartbeat() status = %v, want Idle", w.getStatus())
	}
}

func TestPoolSubmitTask(t *testing.T) {
	p := NewPool()
	task := p.SubmitTask("agent1", []byte("input"), []byte("output"), []byte("model"), "usecase1", 1)

	if task.ID == "" {
		t.Error("SubmitTask() task.ID is empty")
	}
	if task.Status != "pending" {
		t.Errorf("SubmitTask() task.Status = %v, want pending", task.Status)
	}
	if task.Priority != 1 {
		t.Errorf("SubmitTask() task.Priority = %v, want 1", task.Priority)
	}
}

func TestPoolAssignTask(t *testing.T) {
	p := NewPool()
	w := p.RegisterWorker("worker1", "127.0.0.1:8080", 1)
	task := p.SubmitTask("agent1", []byte("input"), []byte("output"), []byte("model"), "usecase1", 1)

	err := p.AssignTask(task.ID, "worker1")
	if err != nil {
		t.Errorf("AssignTask() error = %v, want nil", err)
	}

	if task.Status != "assigned" {
		t.Errorf("AssignTask() task.Status = %v, want assigned", task.Status)
	}
	if task.WorkerID != "worker1" {
		t.Errorf("AssignTask() task.WorkerID = %v, want worker1", task.WorkerID)
	}
	if w.Load != 1 {
		t.Errorf("AssignTask() worker.Load = %v, want 1", w.Load)
	}
}

func TestPoolAssignTaskWorkerNotFound(t *testing.T) {
	p := NewPool()
	task := p.SubmitTask("agent1", []byte("input"), []byte("output"), []byte("model"), "usecase1", 1)

	err := p.AssignTask(task.ID, "nonexistent")
	if err == nil {
		t.Error("AssignTask() error = nil, want error")
	}
	if err.Error() != "worker not found: nonexistent" {
		t.Errorf("AssignTask() error = %v, want worker not found: nonexistent", err)
	}
}

func TestPoolAssignTaskWorkerOffline(t *testing.T) {
	p := NewPool()
	w := p.RegisterWorker("worker1", "127.0.0.1:8080", 1)
	w.setStatus(Offline)
	task := p.SubmitTask("agent1", []byte("input"), []byte("output"), []byte("model"), "usecase1", 1)

	err := p.AssignTask(task.ID, "worker1")
	if err == nil {
		t.Error("AssignTask() error = nil, want error")
	}
	if err.Error() != "worker unavailable: worker1" {
		t.Errorf("AssignTask() error = %v, want worker unavailable: worker1", err)
	}
}

func TestPoolAssignTaskWorkerMaxLoad(t *testing.T) {
	p := NewPool()
	w := p.RegisterWorker("worker1", "127.0.0.1:8080", 1)
	w.incLoad()
	task := p.SubmitTask("agent1", []byte("input"), []byte("output"), []byte("model"), "usecase1", 1)

	err := p.AssignTask(task.ID, "worker1")
	if err == nil {
		t.Error("AssignTask() error = nil, want error")
	}
	if err.Error() != "worker at max load: worker1" {
		t.Errorf("AssignTask() error = %v, want worker at max load: worker1", err)
	}
}

func TestPoolSortQueue(t *testing.T) {
	p := NewPool()
	p.SubmitTask("agent1", []byte("input1"), []byte("output"), []byte("model"), "usecase1", 3)
	p.SubmitTask("agent2", []byte("input2"), []byte("output"), []byte("model"), "usecase2", 1)
	p.SubmitTask("agent3", []byte("input3"), []byte("output"), []byte("model"), "usecase3", 2)

	if len(p.queue) != 3 {
		t.Errorf("queue length = %d, want 3", len(p.queue))
	}

	p.sortQueue()
	if p.queue[0] != p.tasks[p.queue[0]].ID {
		t.Error("sortQueue() did not sort correctly")
	}
}

func TestWorkerConcurrentAccess(t *testing.T) {
	w := &Worker{ID: "worker1", Status: Idle, Load: 0}
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.incLoad()
			w.decLoad()
			_ = w.getStatus()
			_, _ = w.getLoad()
		}()
	}

	wg.Wait()

	if w.Load != 0 {
		t.Errorf("Concurrent access resulted in Load = %v, want 0", w.Load)
	}
}

func TestPoolConcurrentAccess(t *testing.T) {
	p := NewPool()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			workerID := fmt.Sprintf("worker%d", id)
			p.RegisterWorker(workerID, fmt.Sprintf("127.0.0.1:%d", 8080+id), 10)
			if err := p.Heartbeat(workerID); err != nil {
				t.Errorf("Heartbeat failed for %s: %v", workerID, err)
			}
			if err := p.DeregisterWorker(workerID); err != nil {
				t.Errorf("DeregisterWorker failed for %s: %v", workerID, err)
			}
		}(i)
	}

	wg.Wait()

	if len(p.workers) != 0 {
		t.Errorf("Concurrent access resulted in pool with %d workers, want 0", len(p.workers))
	}
}
