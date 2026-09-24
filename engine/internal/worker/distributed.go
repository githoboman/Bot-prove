package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"
)

type WorkerStatus int

const (
	Idle WorkerStatus = iota
	Busy
	Draining
	Offline
)

type Worker struct {
	ID              string
	Addr            string
	Status          WorkerStatus
	Load            int
	MaxLoad           int
	LastHeartbeat     int64
	ProofsGenerated   int
	AvgGenMs          int64
	Failures          int
	currentTasks      int
	mu                sync.RWMutex
}

func (w *Worker) setStatus(s WorkerStatus) {
	w.mu.Lock()
	w.Status = s
	w.mu.Unlock()
}

func (w *Worker) getStatus() WorkerStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.Status
}

func (w *Worker) incLoad() {
	w.mu.Lock()
	w.Load++
	w.currentTasks++
	w.mu.Unlock()
}

func (w *Worker) decLoad() {
	w.mu.Lock()
	w.Load--
	if w.currentTasks > 0 {
		w.currentTasks--
	}
	w.mu.Unlock()
}

func (w *Worker) getLoad() (int, int) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.Load, w.MaxLoad
}

type Task struct {
	ID           string
	InputData    []byte
	OutputData   []byte
	ModelData    []byte
	Agent        string
	UseCase      string
	Priority     int
	CreatedAt    int64
	AssignedAt   int64
	CompletedAt  int64
	WorkerID     string
	Status       string
	Result       []byte
	Error        string
	RetryCount   int
}

type TaskResult struct {
	TaskID  string
	ProofID string
	GenMs   int64
	Success bool
	Error   string
}

type Pool struct {
	mu      sync.RWMutex
	workers map[string]*Worker
	tasks   map[string]*Task
	queue   []string
}

func NewPool() *Pool {
	return &Pool{
		workers: make(map[string]*Worker),
		tasks:   make(map[string]*Task),
		queue:   make([]string, 0),
	}
}

func (p *Pool) RegisterWorker(id, addr string, maxLoad int) *Worker {
	w := &Worker{
		ID:        id,
		Addr:      addr,
		Status:    Idle,
		MaxLoad:   maxLoad,
		LastHeartbeat: time.Now().UnixMilli(),
	}
	p.mu.Lock()
	p.workers[id] = w
	p.mu.Unlock()
	return w
}

func (p *Pool) DeregisterWorker(id string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.workers[id]; !ok {
		return fmt.Errorf("worker not found: %s", id)
	}
	delete(p.workers, id)
	return nil
}

func (p *Pool) Heartbeat(workerID string) error {
	p.mu.RLock()
	w, ok := p.workers[workerID]
	p.mu.RUnlock()
	if !ok {
		return fmt.Errorf("worker not found: %s", workerID)
	}
	w.mu.Lock()
	w.LastHeartbeat = time.Now().UnixMilli()
	if w.Status == Offline {
		w.Status = Idle
	}
	w.mu.Unlock()
	return nil
}

func (p *Pool) SubmitTask(agent string, input, output, model []byte, useCase string, priority int) *Task {
	h := sha256.New()
	h.Write(input)
	h.Write([]byte(agent))
	h.Write([]byte(useCase))
	_, _ = fmt.Fprintf(h, "%d", time.Now().UnixNano())
	id := hex.EncodeToString(h.Sum(nil))[:16]

	t := &Task{
		ID:        id,
		InputData: input,
		OutputData: output,
		ModelData: model,
		Agent:     agent,
		UseCase:   useCase,
		Priority:  priority,
		CreatedAt: time.Now().UnixMilli(),
		Status:    "pending",
	}
	p.mu.Lock()
	p.tasks[id] = t
	p.queue = append(p.queue, id)
	p.sortQueue()
	p.mu.Unlock()
	return t
}

func (p *Pool) sortQueue() {
	sort.SliceStable(p.queue, func(i, j int) bool {
		ti := p.tasks[p.queue[i]]
		tj := p.tasks[p.queue[j]]
		return ti.Priority > tj.Priority
	})
}

func (p *Pool) AssignTask(taskID, workerID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	t, ok := p.tasks[taskID]
	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}
	w, ok := p.workers[workerID]
	if !ok {
		return fmt.Errorf("worker not found: %s", workerID)
	}
	if w.getStatus() == Offline || w.getStatus() == Draining {
		return fmt.Errorf("worker unavailable: %s", workerID)
	}
	load, maxLoad := w.getLoad()
	if load >= maxLoad {
		return fmt.Errorf("worker at max load: %s", workerID)
	}
	t.Status = "assigned"
	t.AssignedAt = time.Now().UnixMilli()
	t.WorkerID = workerID
	w.incLoad()
	for i, id := range p.queue {
		if id == taskID {
			p.queue = append(p.queue[:i], p.queue[i+1:]...)
			break
		}
	}
	return nil
}

func (p *Pool) CompleteTask(taskID string, result TaskResult) error {
	p.mu.Lock()
	t, ok := p.tasks[taskID]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("task not found: %s", taskID)
	}
	w, ok := p.workers[t.WorkerID]
	p.mu.Unlock()
	if ok {
		w.decLoad()
		w.mu.Lock()
		w.ProofsGenerated++
		if w.AvgGenMs == 0 {
			w.AvgGenMs = result.GenMs
		} else {
			w.AvgGenMs = (w.AvgGenMs + result.GenMs) / 2
		}
		w.mu.Unlock()
	}
	t.Status = "completed"
	t.CompletedAt = time.Now().UnixMilli()
	t.Result = []byte(result.ProofID)
	return nil
}

func (p *Pool) FailTask(taskID, reason string) error {
	p.mu.Lock()
	t, ok := p.tasks[taskID]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("task not found: %s", taskID)
	}
	w, ok := p.workers[t.WorkerID]
	p.mu.Unlock()
	if ok {
		w.decLoad()
		w.mu.Lock()
		w.Failures++
		w.mu.Unlock()
	}
	t.RetryCount++
	if t.RetryCount > 3 {
		t.Status = "failed"
		t.Error = reason
	} else {
		t.Status = "pending"
		t.WorkerID = ""
		p.mu.Lock()
		p.queue = append(p.queue, taskID)
		p.sortQueue()
		p.mu.Unlock()
	}
	return nil
}

func (p *Pool) GetTask(taskID string) (*Task, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	t, ok := p.tasks[taskID]
	if !ok {
		return nil, false
	}
	tt := *t
	return &tt, true
}

func (p *Pool) GetWorkerLoad(workerID string) (int, int) {
	p.mu.RLock()
	w, ok := p.workers[workerID]
	p.mu.RUnlock()
	if !ok {
		return 0, 0
	}
	return w.getLoad()
}

func (p *Pool) SelectWorker() (*Worker, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	var best *Worker
	for _, w := range p.workers {
		st := w.getStatus()
		if st != Idle && st != Busy {
			continue
		}
		load, maxLoad := w.getLoad()
		if load >= maxLoad {
			continue
		}
		if best == nil || load < best.Load {
			best = w
		}
	}
	if best == nil {
		return nil, fmt.Errorf("no available worker")
	}
	return best, nil
}

func (p *Pool) RebalanceTasks() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	count := 0
	for _, t := range p.tasks {
		if t.Status != "assigned" {
			continue
		}
		w, ok := p.workers[t.WorkerID]
		if !ok || w.getStatus() == Offline {
			t.Status = "pending"
			t.WorkerID = ""
			p.queue = append(p.queue, t.ID)
			count++
		}
	}
	if count > 0 {
		p.sortQueue()
	}
	return count
}

func (p *Pool) DrainWorker(workerID string) error {
	p.mu.RLock()
	w, ok := p.workers[workerID]
	p.mu.RUnlock()
	if !ok {
		return fmt.Errorf("worker not found: %s", workerID)
	}
	w.setStatus(Draining)
	return nil
}

func (p *Pool) GetQueueDepth() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.queue)
}

func (p *Pool) GetStats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()
	stats := map[string]interface{}{
		"workers_online":    0,
		"tasks_pending":   0,
		"tasks_completed": 0,
		"tasks_failed":    0,
		"avg_gen_ms":      int64(0),
	}
	var totalGen int64
	var genCount int
	for _, w := range p.workers {
		if w.getStatus() != Offline {
			if n, ok := stats["workers_online"].(int); ok {
				stats["workers_online"] = n + 1
			}
		}
		w.mu.RLock()
		if w.AvgGenMs > 0 {
			totalGen += w.AvgGenMs
			genCount++
		}
		w.mu.RUnlock()
	}
	for _, t := range p.tasks {
		switch t.Status {
		case "pending":
			if n, ok := stats["tasks_pending"].(int); ok {
				stats["tasks_pending"] = n + 1
			}
		case "completed":
			if n, ok := stats["tasks_completed"].(int); ok {
				stats["tasks_completed"] = n + 1
			}
		case "failed":
			if n, ok := stats["tasks_failed"].(int); ok {
				stats["tasks_failed"] = n + 1
			}
		}
	}
	if genCount > 0 {
		stats["avg_gen_ms"] = totalGen / int64(genCount)
	}
	return stats
}

func (p *Pool) PruneStaleTasks(timeoutSec int64) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now().UnixMilli()
	count := 0
	for _, t := range p.tasks {
		if t.Status != "assigned" {
			continue
		}
		if now-t.AssignedAt > timeoutSec*1000 {
			t.Status = "timeout"
			count++
			w, ok := p.workers[t.WorkerID]
			if ok {
				w.decLoad()
			}
		}
	}
	return count
}

func (p *Pool) PruneOfflineWorkers(timeoutSec int64) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now().UnixMilli()
	count := 0
	for _, w := range p.workers {
		w.mu.RLock()
		lastHB := w.LastHeartbeat
		w.mu.RUnlock()
		if now-lastHB > timeoutSec*1000 {
			w.setStatus(Offline)
			count++
		}
	}
	return count
}
