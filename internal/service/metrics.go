package service

import (
	"sort"
	"sync"
)

// CounterMetrics is deliberately small and exports a stable Prometheus text view at the HTTP edge.
type CounterMetrics struct {
	mu     sync.RWMutex
	values map[string]uint64
}

func NewCounterMetrics() *CounterMetrics  { return &CounterMetrics{values: map[string]uint64{}} }
func (m *CounterMetrics) Inc(name string) { m.mu.Lock(); m.values[name]++; m.mu.Unlock() }
func (m *CounterMetrics) Snapshot() map[string]uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]uint64, len(m.values))
	for name, value := range m.values {
		result[name] = value
	}
	return result
}
func (m *CounterMetrics) Names() []string {
	snapshot := m.Snapshot()
	names := make([]string, 0, len(snapshot))
	for name := range snapshot {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
