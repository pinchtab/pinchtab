package bridgeregistry

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

const registryDir = "bridges"

// Record is the durable, non-secret identity of one standalone bridge.
type Record struct {
	PID                    int       `json:"pid"`
	ProcessStartUnixMillis int64     `json:"processStartUnixMillis"`
	Address                string    `json:"address"`
	Port                   string    `json:"port"`
	CDPIdentity            string    `json:"cdpIdentity"`
	BrowserType            string    `json:"browserType"`
	BrowserLabel           string    `json:"browserLabel"`
	RegisteredAt           time.Time `json:"registeredAt"`
}

// State adds current process and listener evidence to a durable record.
type State struct {
	Record
	Status    string `json:"status"`
	PIDStatus string `json:"pidStatus"`
	Reachable bool   `json:"reachable"`
	Live      bool   `json:"live"`
	Stale     bool   `json:"stale"`
	Pruned    bool   `json:"pruned,omitempty"`
}

// Registration removes only its own record when closed.
type Registration struct {
	root *os.Root
	name string
	once sync.Once
	err  error
}

// Register atomically creates a private record under stateDir/bridges.
func Register(stateDir string, rec Record) (*Registration, error) {
	root, err := openRegistry(stateDir, true)
	if err != nil {
		return nil, err
	}

	rec.PID = os.Getpid()
	rec.ProcessStartUnixMillis, err = processStart(rec.PID)
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("read bridge process identity: %w", err)
	}
	rec.Address = strings.TrimSpace(rec.Address)
	rec.Port = strings.TrimSpace(rec.Port)
	rec.CDPIdentity = SafeCDPIdentity(rec.CDPIdentity)
	rec.BrowserType = strings.TrimSpace(rec.BrowserType)
	rec.BrowserLabel = strings.TrimSpace(rec.BrowserLabel)
	rec.RegisteredAt = time.Now().UTC()
	if err := validateRecord(rec); err != nil {
		_ = root.Close()
		return nil, err
	}

	id, err := randomID()
	if err != nil {
		_ = root.Close()
		return nil, fmt.Errorf("generate bridge record id: %w", err)
	}
	name := fmt.Sprintf("bridge-%d-%s.json", rec.PID, id)
	if err := writeAtomic(root, name, rec, id); err != nil {
		_ = root.Close()
		return nil, err
	}
	return &Registration{root: root, name: name}, nil
}

// Close removes the registration file without signaling or otherwise touching
// the bridge process.
func (r *Registration) Close() error {
	if r == nil || r.root == nil {
		return nil
	}
	r.once.Do(func() {
		if err := r.root.Remove(r.name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			r.err = err
		}
		if err := r.root.Close(); r.err == nil {
			r.err = err
		}
	})
	return r.err
}

// List returns every bridge record with fresh liveness evidence. When prune is
// true it removes only records whose original PID is conclusively dead or has
// been reused; it never signals a process.
func List(stateDir string, prune bool) ([]State, error) {
	root, err := openRegistry(stateDir, false)
	if errors.Is(err, fs.ErrNotExist) {
		return []State{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, fmt.Errorf("read bridge registry: %w", err)
	}
	states := make([]State, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.Type()&fs.ModeSymlink != 0 || !strings.HasPrefix(name, "bridge-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		info, err := root.Lstat(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect bridge record %q: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		data, err := root.ReadFile(name)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read bridge record %q: %w", name, err)
		}
		var rec Record
		if err := json.Unmarshal(data, &rec); err != nil {
			return nil, fmt.Errorf("decode bridge record %q: %w", name, err)
		}
		state := inspect(rec)
		if prune && (state.PIDStatus == "dead" || state.PIDStatus == "reused") {
			if err := root.Remove(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return nil, fmt.Errorf("prune bridge record %q: %w", name, err)
			}
			state.Pruned = true
		}
		states = append(states, state)
	}

	sort.Slice(states, func(i, j int) bool {
		pi, _ := strconv.Atoi(states[i].Port)
		pj, _ := strconv.Atoi(states[j].Port)
		if pi != pj {
			return pi < pj
		}
		if states[i].PID != states[j].PID {
			return states[i].PID < states[j].PID
		}
		return states[i].RegisteredAt.Before(states[j].RegisteredAt)
	})
	return states, nil
}

// SafeCDPIdentity preserves enough identity to spot bridges attached to the
// same browser while dropping credentials, query strings, fragments, and the
// raw browser GUID.
func SafeCDPIdentity(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Hostname() == "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if port := u.Port(); port != "" {
		host = net.JoinHostPort(host, port)
	} else if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	identity := strings.ToLower(u.Scheme) + "://" + host
	const marker = "/devtools/browser/"
	if idx := strings.Index(u.Path, marker); idx >= 0 {
		browserID := strings.Split(strings.TrimPrefix(u.Path[idx:], marker), "/")[0]
		if browserID != "" {
			sum := sha256.Sum256([]byte(browserID))
			identity += marker + hex.EncodeToString(sum[:6])
		}
	}
	return identity
}

func openRegistry(stateDir string, create bool) (*os.Root, error) {
	stateDir = strings.TrimSpace(stateDir)
	if stateDir == "" {
		return nil, fmt.Errorf("bridge registry requires a state directory")
	}
	if create {
		if err := os.MkdirAll(stateDir, 0o700); err != nil {
			return nil, fmt.Errorf("create state directory: %w", err)
		}
	}
	stateRoot, err := os.OpenRoot(stateDir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stateRoot.Close() }()
	if create {
		if err := stateRoot.MkdirAll(registryDir, 0o700); err != nil {
			return nil, fmt.Errorf("create bridge registry: %w", err)
		}
		info, err := stateRoot.Lstat(registryDir)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("bridge registry is not a real directory")
		}
		if err := stateRoot.Chmod(registryDir, 0o700); err != nil {
			return nil, fmt.Errorf("secure bridge registry: %w", err)
		}
	}
	root, err := stateRoot.OpenRoot(registryDir)
	if err != nil {
		return nil, err
	}
	return root, nil
}

func writeAtomic(root *os.Root, name string, rec Record, id string) error {
	data, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("encode bridge record: %w", err)
	}
	tmp := ".bridge-" + id + ".tmp"
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create bridge record: %w", err)
	}
	ok := false
	defer func() {
		_ = f.Close()
		if !ok {
			_ = root.Remove(tmp)
		}
	}()
	if err := f.Chmod(0o600); err != nil {
		return fmt.Errorf("secure bridge record: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write bridge record: %w", err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("sync bridge record: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close bridge record: %w", err)
	}
	if err := root.Rename(tmp, name); err != nil {
		return fmt.Errorf("publish bridge record: %w", err)
	}
	ok = true
	return nil
}

func validateRecord(rec Record) error {
	if rec.Address == "" {
		return fmt.Errorf("bridge registry requires an address")
	}
	port, err := strconv.Atoi(rec.Port)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("bridge registry has invalid port %q", rec.Port)
	}
	if rec.BrowserType == "" {
		return fmt.Errorf("bridge registry requires a browser type")
	}
	return nil
}

func inspect(rec Record) State {
	pidStatus := processStatus(rec)
	reachable := listenerReachable(rec.Address, rec.Port)
	state := State{Record: rec, PIDStatus: pidStatus, Reachable: reachable}
	switch {
	case pidStatus == "dead":
		state.Status, state.Stale = "stale_pid", true
	case pidStatus == "reused":
		state.Status, state.Stale = "stale_pid_reused", true
	case pidStatus == "unknown":
		state.Status = "unknown_pid"
	case !reachable:
		state.Status, state.Stale = "stale_listener", true
	default:
		state.Status, state.Live = "live", true
	}
	return state
}

func processStatus(rec Record) string {
	if rec.PID <= 0 {
		return "dead"
	}
	exists, err := process.PidExists(int32(rec.PID))
	if err != nil {
		return "unknown"
	}
	if !exists {
		return "dead"
	}
	if rec.ProcessStartUnixMillis <= 0 {
		return "unknown"
	}
	started, err := processStart(rec.PID)
	if err != nil {
		return "unknown"
	}
	if started != rec.ProcessStartUnixMillis {
		return "reused"
	}
	return "alive"
}

func processStart(pid int) (int64, error) {
	p, err := process.NewProcess(int32(pid))
	if err != nil {
		return 0, err
	}
	return p.CreateTime()
}

func listenerReachable(address, port string) bool {
	host := strings.Trim(strings.TrimSpace(address), "[]")
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	} else if host == "::" {
		host = "::1"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 250*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func randomID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
