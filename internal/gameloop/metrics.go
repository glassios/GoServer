package gameloop

import (
	"sort"
	"time"
)

type TickMetrics struct {
	durations []time.Duration
	maxSize   int
}

func NewTickMetrics(maxSize int) *TickMetrics {
	return &TickMetrics{
		durations: make([]time.Duration, 0, maxSize),
		maxSize:   maxSize,
	}
}

func (m *TickMetrics) Record(duration time.Duration) {
	if len(m.durations) >= m.maxSize {
		m.durations = m.durations[1:]
	}
	m.durations = append(m.durations, duration)
}

func (m *TickMetrics) Average() time.Duration {
	if len(m.durations) == 0 {
		return 0
	}
	var total time.Duration
	for _, d := range m.durations {
		total += d
	}
	return total / time.Duration(len(m.durations))
}

func (m *TickMetrics) P99() time.Duration {
	if len(m.durations) == 0 {
		return 0
	}
	// Copy and sort
	sorted := make([]time.Duration, len(m.durations))
	copy(sorted, m.durations)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i] < sorted[j]
	})

	index := int(float64(len(sorted)) * 0.99)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}
