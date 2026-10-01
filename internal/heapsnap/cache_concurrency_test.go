package heapsnap

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestCacheServesConcurrentPlainAndRetainedLoads(t *testing.T) {
	src, err := os.ReadFile(grownFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	paths := []string{filepath.Join(dir, "heap_a"+Ext), filepath.Join(dir, "heap_b"+Ext)}
	for _, p := range paths {
		if err := os.WriteFile(p, src, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var c Cache
	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := range 32 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			base, err := c.Load(paths[i%2], false)
			if err != nil {
				errs <- err
				return
			}
			head, err := c.Load(paths[(i+1)%2], i%3 == 0)
			if err != nil {
				errs <- err
				return
			}
			if _, err := Compare(base, head, Options{Top: 5, Retained: i%3 == 0}); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}

func TestCacheReportsADeletedSnapshotAsMissing(t *testing.T) {
	src, err := os.ReadFile(grownFixturePath)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "heap_gone"+Ext)
	if err := os.WriteFile(path, src, 0600); err != nil {
		t.Fatal(err)
	}
	var c Cache
	if _, err := c.Load(path, true); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Load(path, false); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("load after delete err = %v, want os.ErrNotExist rather than the cached parse", err)
	}
}
