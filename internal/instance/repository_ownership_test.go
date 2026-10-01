package instance

import (
	"sync"
	"testing"

	bridgepkg "github.com/pinchtab/pinchtab/internal/bridge"
)

// get and running return copies, so the repository is meant to own its
// instances. Add used to store the caller's pointer instead, and the
// orchestrator keeps mutating that struct under its own lock — two unrelated
// mutexes over the same memory.
func TestRepositoryDoesNotAliasAddedInstance(t *testing.T) {
	repo := newRepository()

	inst := &bridgepkg.Instance{ID: "inst_1", Port: "9001", Status: "running"}
	repo.add(inst)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 500 {
			inst.Status = "stopped"
			inst.Status = "running"
		}
	}()
	go func() {
		defer wg.Done()
		for range 500 {
			_ = repo.running()
		}
	}()
	wg.Wait()
}

// A snapshot taken through Add must not change when the caller mutates the
// struct it handed over.
func TestRepositoryAddSnapshotsInstance(t *testing.T) {
	repo := newRepository()

	inst := &bridgepkg.Instance{ID: "inst_1", Port: "9001", Status: "running"}
	repo.add(inst)
	inst.Status = "stopped"

	got, ok := repo.get("inst_1")
	if !ok {
		t.Fatal("instance missing from repository")
	}
	if got.Status != "running" {
		t.Errorf("Status = %q, want running — repository aliased the caller's struct", got.Status)
	}
}
