package scheduler

import (
	"container/heap"
	"fmt"
	"sync"
)

// TaskQueue is an in-memory priority queue with per-agent fairness.
type TaskQueue struct {
	mu          sync.Mutex
	agents      map[string]*agentQueue
	totalCount  int
	maxTotal    int
	maxPerAgent int
}

type agentQueue struct {
	tasks    taskHeap
	inflight int
}

// NewTaskQueue creates a queue with the given global and per-agent limits.
func NewTaskQueue(maxTotal, maxPerAgent int) *TaskQueue {
	return &TaskQueue{
		agents:      make(map[string]*agentQueue),
		maxTotal:    maxTotal,
		maxPerAgent: maxPerAgent,
	}
}

// Enqueue adds a task. Returns the queue position or an error if limits are hit.
func (q *TaskQueue) Enqueue(t *Task) (int, error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.totalCount >= q.maxTotal {
		return 0, fmt.Errorf("global queue full (%d/%d)", q.totalCount, q.maxTotal)
	}

	aq, ok := q.agents[t.AgentID]
	if !ok {
		aq = &agentQueue{}
		heap.Init(&aq.tasks)
		q.agents[t.AgentID] = aq
	}

	if aq.tasks.Len() >= q.maxPerAgent {
		return 0, fmt.Errorf("agent queue full for %q (%d/%d)", t.AgentID, aq.tasks.Len(), q.maxPerAgent)
	}

	heap.Push(&aq.tasks, t)
	q.totalCount++
	return q.totalCount, nil
}

// Dequeue picks the next task using fair round-robin: the agent with the
// fewest in-flight tasks gets served first. Among tasks for that agent,
// the heap ordering (priority then creation time) decides.
func (q *TaskQueue) Dequeue(maxPerAgentInflight, maxGlobalInflight int) *Task {
	q.mu.Lock()
	defer q.mu.Unlock()

	globalInflight := 0
	for _, aq := range q.agents {
		globalInflight += aq.inflight
	}
	if globalInflight >= maxGlobalInflight {
		return nil
	}

	var bestAgent string
	bestInflight := int(^uint(0) >> 1) // max int

	for agentID, aq := range q.agents {
		if aq.tasks.Len() == 0 {
			continue
		}
		if aq.inflight >= maxPerAgentInflight {
			continue
		}
		if aq.inflight < bestInflight {
			bestInflight = aq.inflight
			bestAgent = agentID
		}
	}

	if bestAgent == "" {
		return nil
	}

	aq := q.agents[bestAgent]
	t := heap.Pop(&aq.tasks).(*Task)
	aq.inflight++
	q.totalCount--
	return t
}

// Complete marks a task as no longer in-flight for its agent.
func (q *TaskQueue) Complete(agentID string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if aq, ok := q.agents[agentID]; ok {
		if aq.inflight > 0 {
			aq.inflight--
		}
		if aq.inflight == 0 && aq.tasks.Len() == 0 {
			delete(q.agents, agentID)
		}
	}
}

// Remove removes a specific task from its agent's queue.
// Returns true if the task was found and removed.
func (q *TaskQueue) Remove(taskID, agentID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	aq, ok := q.agents[agentID]
	if !ok {
		return false
	}

	for i, t := range aq.tasks {
		if t.ID == taskID {
			heap.Remove(&aq.tasks, i)
			q.totalCount--
			if aq.tasks.Len() == 0 && aq.inflight == 0 {
				delete(q.agents, agentID)
			}
			return true
		}
	}
	return false
}

// ExpireDeadlined scans all queued tasks and returns those whose deadline
// has passed. The returned tasks are removed from the queue.
func (q *TaskQueue) ExpireDeadlined() []*Task {
	q.mu.Lock()
	defer q.mu.Unlock()

	var expired []*Task
	for agentID, aq := range q.agents {
		var remaining taskHeap
		for _, t := range aq.tasks {
			if !t.Deadline.IsZero() && t.Deadline.Before(timeNow()) {
				expired = append(expired, t)
				q.totalCount--
			} else {
				remaining = append(remaining, t)
			}
		}
		if len(remaining) != len(aq.tasks) {
			aq.tasks = remaining
			heap.Init(&aq.tasks)
		}
		if aq.tasks.Len() == 0 && aq.inflight == 0 {
			delete(q.agents, agentID)
		}
	}
	return expired
}

// Stats returns snapshot queue statistics.
func (q *TaskQueue) Stats() QueueStats {
	q.mu.Lock()
	defer q.mu.Unlock()

	s := QueueStats{
		TotalQueued: q.totalCount,
		Agents:      make(map[string]AgentStats, len(q.agents)),
	}
	for agentID, aq := range q.agents {
		s.TotalInflight += aq.inflight
		s.Agents[agentID] = AgentStats{
			Queued:   aq.tasks.Len(),
			Inflight: aq.inflight,
		}
	}
	return s
}

// QueueStats holds a point-in-time snapshot of the queue.
type QueueStats struct {
	TotalQueued   int                   `json:"totalQueued"`
	TotalInflight int                   `json:"totalInflight"`
	Agents        map[string]AgentStats `json:"agents"`
}

// AgentStats holds per-agent queue metrics.
type AgentStats struct {
	Queued   int `json:"queued"`
	Inflight int `json:"inflight"`
}

// --- heap implementation ---

// taskHeap implements container/heap for priority-then-FIFO ordering.
type taskHeap []*Task

func (h taskHeap) Len() int { return len(h) }

func (h taskHeap) Less(i, j int) bool {
	if h[i].Priority != h[j].Priority {
		return h[i].Priority < h[j].Priority
	}
	return h[i].CreatedAt.Before(h[j].CreatedAt)
}

func (h taskHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *taskHeap) Push(x any) {
	*h = append(*h, x.(*Task))
}

func (h *taskHeap) Pop() any {
	old := *h
	n := len(old)
	t := old[n-1]
	old[n-1] = nil
	*h = old[:n-1]
	return t
}
