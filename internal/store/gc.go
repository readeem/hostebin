package store

import (
	"errors"
	"time"
)

func (s *Store) SweepExpired() (int, error) {
	var expired []string
	if err := s.walk(func(meta *BundleMeta) error {
		if meta.expired() {
			expired = append(expired, meta.ID)
		}
		return nil
	}); err != nil {
		return 0, err
	}
	for i, id := range expired {
		if err := s.remove(id); err != nil && !errors.Is(err, ErrNotFound) {
			return i, err
		}
	}
	return len(expired), nil
}

func (s *Store) RunGC(stop <-chan struct{}, interval time.Duration, report func(int, error)) {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	run := func() {
		n, err := s.SweepExpired()
		if report != nil {
			report(n, err)
		}
	}
	run()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			run()
		case <-stop:
			return
		}
	}
}
