package subagent

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GitHub-freshman-X/mewcode01/internal/provider"
)

type TaskStatus string

const (
	TaskRunning   TaskStatus = "running"
	TaskCompleted TaskStatus = "completed"
	TaskFailed    TaskStatus = "failed"
	TaskCancelled TaskStatus = "cancelled"
)

type TaskInfo struct {
	ID          string
	Name        string
	Description string
	Status      TaskStatus
	Result      string
	Failure     string
	StartedAt   time.Time
	EndedAt     time.Time
	Usage       provider.Usage
	ToolCalls   int
	Background  bool
}

type Progress struct {
	Iterations int
	ToolCalls  int
	Usage      provider.Usage
}

type Outcome struct {
	Status    TaskStatus
	Result    string
	Failure   string
	Usage     provider.Usage
	ToolCalls int
}

type Worker func(context.Context, func(Progress)) Outcome

type LaunchRequest struct {
	Name        string
	Description string
	Background  bool
	Worker      Worker
}

type TaskNotification struct{ Task TaskInfo }

type TaskManager struct {
	mu          sync.RWMutex
	clock       func() time.Time
	sequence    atomic.Uint64
	tasks       map[string]*managedTask
	subscribers map[chan TaskNotification]struct{}
}

type managedTask struct {
	info   TaskInfo
	cancel context.CancelFunc
	done   chan struct{}
}

func NewTaskManager() *TaskManager {
	return &TaskManager{clock: time.Now, tasks: make(map[string]*managedTask), subscribers: make(map[chan TaskNotification]struct{})}
}

func (m *TaskManager) Launch(ctx context.Context, request LaunchRequest) (TaskInfo, error) {
	if m == nil || request.Worker == nil {
		return TaskInfo{}, fmt.Errorf("subagent task worker is required")
	}
	started := m.clock()
	id := fmt.Sprintf("subagent-%d", m.sequence.Add(1))
	taskCtx, cancel := context.WithCancel(ctx)
	task := &managedTask{info: TaskInfo{ID: id, Name: request.Name, Description: request.Description, Status: TaskRunning, StartedAt: started, Background: request.Background}, cancel: cancel, done: make(chan struct{})}
	m.mu.Lock()
	m.tasks[id] = task
	initial := cloneTaskInfo(task.info)
	m.mu.Unlock()
	m.publish(initial)
	go func() {
		outcome := request.Worker(taskCtx, func(progress Progress) { m.updateProgress(id, progress) })
		m.finish(id, outcome)
	}()
	return initial, nil
}

func (m *TaskManager) Cancel(id string) bool {
	m.mu.RLock()
	task, ok := m.tasks[id]
	m.mu.RUnlock()
	if !ok || task.info.Status != TaskRunning {
		return false
	}
	task.cancel()
	return true
}

func (m *TaskManager) Get(id string) (TaskInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, ok := m.tasks[id]
	if !ok {
		return TaskInfo{}, false
	}
	return cloneTaskInfo(task.info), true
}

func (m *TaskManager) List() []TaskInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]TaskInfo, 0, len(m.tasks))
	for _, task := range m.tasks {
		out = append(out, cloneTaskInfo(task.info))
	}
	return out
}

// MarkBackground atomically marks a running task as detached from the foreground.
func (m *TaskManager) MarkBackground(id string) bool {
	m.mu.Lock()
	task, ok := m.tasks[id]
	if !ok || task.info.Status != TaskRunning {
		m.mu.Unlock()
		return false
	}
	task.info.Background = true
	info := cloneTaskInfo(task.info)
	m.mu.Unlock()
	m.publish(info)
	return true
}

// RunningBackground returns a stable snapshot of running background tasks only.
func (m *TaskManager) RunningBackground() []TaskInfo {
	m.mu.RLock()
	out := make([]TaskInfo, 0, len(m.tasks))
	for _, task := range m.tasks {
		if task.info.Status == TaskRunning && task.info.Background {
			out = append(out, cloneTaskInfo(task.info))
		}
	}
	m.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].StartedAt.Before(out[j].StartedAt)
	})
	return out
}

func (m *TaskManager) Wait(ctx context.Context, id string) (TaskInfo, error) {
	m.mu.RLock()
	task, ok := m.tasks[id]
	m.mu.RUnlock()
	if !ok {
		return TaskInfo{}, fmt.Errorf("unknown subagent task %q", id)
	}
	select {
	case <-task.done:
		info, _ := m.Get(id)
		return info, nil
	case <-ctx.Done():
		return TaskInfo{}, ctx.Err()
	}
}

func (m *TaskManager) Subscribe() (<-chan TaskNotification, func()) {
	ch := make(chan TaskNotification, 16)
	m.mu.Lock()
	m.subscribers[ch] = struct{}{}
	m.mu.Unlock()
	return ch, func() {
		m.mu.Lock()
		if _, ok := m.subscribers[ch]; ok {
			delete(m.subscribers, ch)
			close(ch)
		}
		m.mu.Unlock()
	}
}

func (m *TaskManager) updateProgress(id string, progress Progress) {
	m.mu.Lock()
	task, ok := m.tasks[id]
	if !ok || task.info.Status != TaskRunning {
		m.mu.Unlock()
		return
	}
	task.info.ToolCalls = progress.ToolCalls
	task.info.Usage = progress.Usage
	info := cloneTaskInfo(task.info)
	m.mu.Unlock()
	m.publish(info)
}

func (m *TaskManager) finish(id string, outcome Outcome) {
	m.mu.Lock()
	task, ok := m.tasks[id]
	if !ok || task.info.Status != TaskRunning {
		m.mu.Unlock()
		return
	}
	status := outcome.Status
	if status != TaskCompleted && status != TaskFailed && status != TaskCancelled {
		status = TaskFailed
		if outcome.Failure == "" {
			outcome.Failure = "subagent ended without a terminal status"
		}
	}
	task.info.Status = status
	task.info.Result = outcome.Result
	task.info.Failure = outcome.Failure
	task.info.Usage = outcome.Usage
	task.info.ToolCalls = outcome.ToolCalls
	task.info.EndedAt = m.clock()
	info := cloneTaskInfo(task.info)
	close(task.done)
	m.mu.Unlock()
	m.publish(info)
}

func (m *TaskManager) publish(info TaskInfo) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for ch := range m.subscribers {
		select {
		case ch <- TaskNotification{Task: cloneTaskInfo(info)}:
		default:
		}
	}
}

func cloneTaskInfo(info TaskInfo) TaskInfo { return info }
