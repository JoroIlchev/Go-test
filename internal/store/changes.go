package store

import "sync"

// changes wakes players waiting for new content: every admin action goes
// through Audit and every finished file through MarkMedia*, so those two
// signal it. Waiters re-check what they serve; most changes are not theirs.
type changes struct {
	mu sync.Mutex
	ch chan struct{}
}

// Changed returns a channel that is closed at the next change.
func (s *Store) Changed() <-chan struct{} {
	s.changes.mu.Lock()
	defer s.changes.mu.Unlock()
	if s.changes.ch == nil {
		s.changes.ch = make(chan struct{})
	}
	return s.changes.ch
}

func (s *Store) notify() {
	s.changes.mu.Lock()
	defer s.changes.mu.Unlock()
	if s.changes.ch != nil {
		close(s.changes.ch)
		s.changes.ch = nil
	}
}
