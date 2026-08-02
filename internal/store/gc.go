package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (s *Store) SweepExpired() (int, error) {
	entries, err := os.ReadDir(s.bundlesDir)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		_, err := s.Get(entry.Name())
		if errors.Is(err, ErrExpired) {
			trash := filepath.Join(s.bundlesDir, ".trash-"+entry.Name())
			if renameErr := os.Rename(s.bundleDir(entry.Name()), trash); renameErr != nil {
				return removed, renameErr
			}
			if removeErr := os.RemoveAll(trash); removeErr != nil {
				return removed, removeErr
			}
			removed++
		} else if err != nil && !errors.Is(err, ErrNotFound) {
			return removed, err
		}
	}
	return removed, nil
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
