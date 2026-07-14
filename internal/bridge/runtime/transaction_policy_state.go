package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"time"
)

const (
	transactionPolicyStateRoot      = "transaction-policy"
	transactionPolicyCurrentFile    = "current"
	maxTransactionPolicyGenerations = 4
)

var transactionPolicyGenerationName = regexp.MustCompile(`^generation-[a-f0-9]{64}$`)

func writeTransactionPolicyExtension(stateDir string, manifest transactionPolicyManifest, rules []dnrRule) (string, error) {
	stateRoot, err := filepath.Abs(stateDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(stateRoot, 0700); err != nil {
		return "", err
	} // never chmod StateDir
	// StateDir may be shared with other application state and may intentionally
	// have broader permissions. Only our dedicated child is policy-owned.
	if err := checkStateDirectory(stateRoot); err != nil {
		return "", fmt.Errorf("unsafe stateDir: %w", err)
	}
	root := filepath.Join(stateRoot, transactionPolicyStateRoot)
	if err := os.Mkdir(root, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	if err := checkPolicyDirectory(root); err != nil {
		return "", fmt.Errorf("unsafe transaction policy state root: %w", err)
	}
	unlock, err := lockTransactionPolicyRoot(root)
	if err != nil {
		return "", err
	}
	defer unlock()
	manifestData, err := jsonData(manifest)
	if err != nil {
		return "", err
	}
	rulesData, err := jsonData(rules)
	if err != nil {
		return "", err
	}
	background := []byte("chrome.runtime.onMessage.addListener((_, __, reply) => { chrome.declarativeNetRequest.getEnabledRulesets().then(reply); return true; });\n")
	probe := []byte("<script>chrome.runtime.sendMessage({pinchTabPolicyProbe: true});</script>\n")
	digest := policyDigest(manifestData, rulesData, background, probe)
	name := "generation-" + digest
	target := filepath.Join(root, name)
	if _, err := os.Lstat(target); err == nil {
		if err := checkTransactionPolicyGenerationContent(target, manifest, rules); err != nil {
			return "", err
		}
	} else if !os.IsNotExist(err) {
		return "", err
	} else {
		staging, err := os.MkdirTemp(root, ".staging-")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(staging)
		if err := os.Chmod(staging, 0700); err != nil {
			return "", err
		}
		for _, file := range []struct {
			name string
			data []byte
		}{{"manifest.json", manifestData}, {"rules.json", rulesData}, {"background.js", background}, {"probe.html", probe}} {
			if err := os.WriteFile(filepath.Join(staging, file.name), file.data, 0600); err != nil {
				return "", err
			}
		}
		if err := os.Rename(staging, target); err != nil {
			return "", err
		}
	}
	if err := publishTransactionPolicyCurrent(root, name); err != nil {
		return "", err
	}
	if err := retainTransactionPolicyGenerations(root, name); err != nil {
		return "", err
	}
	return target, nil
}

func jsonData(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}
func policyDigest(parts ...[]byte) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(fmt.Sprintf("%d:", len(part))))
		_, _ = h.Write(part)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func checkStateDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("not a real directory")
	}
	return nil
}

func checkPolicyDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("not a real directory")
	}
	if info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("permissions %o are not private", info.Mode().Perm())
	}
	return checkPolicyOwner(info)
}
func checkPolicyOwner(info fs.FileInfo) error {
	if runtimeGOOS == "windows" {
		return nil
	}
	v := reflect.ValueOf(info.Sys())
	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}
	uid := v.FieldByName("Uid")
	if uid.IsValid() && uid.CanUint() && int(uid.Uint()) != osGeteuid() {
		return fmt.Errorf("not owned by current user")
	}
	return nil
}
func checkTransactionPolicyGeneration(path string) error {
	if err := checkPolicyDirectory(path); err != nil {
		return err
	}
	for _, name := range []string{"manifest.json", "rules.json", "background.js", "probe.html"} {
		info, err := os.Lstat(filepath.Join(path, name))
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 {
			return fmt.Errorf("unsafe generated file %s", name)
		}
		if err := checkPolicyOwner(info); err != nil {
			return err
		}
	}
	return nil
}

// checkTransactionPolicyGenerationContent binds the content-addressed name to
// the exact bytes compiled from this launch configuration. Ownership alone is
// insufficient: a same-user accidental or compromised replacement must fail
// before Chrome starts.
func checkTransactionPolicyGenerationContent(path string, manifest transactionPolicyManifest, rules []dnrRule) error {
	if err := checkTransactionPolicyGeneration(path); err != nil {
		return err
	}
	manifestData, err := jsonData(manifest)
	if err != nil {
		return err
	}
	rulesData, err := jsonData(rules)
	if err != nil {
		return err
	}
	background := []byte("chrome.runtime.onMessage.addListener((_, __, reply) => { chrome.declarativeNetRequest.getEnabledRulesets().then(reply); return true; });\n")
	probe := []byte("<script>chrome.runtime.sendMessage({pinchTabPolicyProbe: true});</script>\n")
	expected := "generation-" + policyDigest(manifestData, rulesData, background, probe)
	if filepath.Base(path) != expected {
		return fmt.Errorf("generation name does not authenticate policy content")
	}
	for _, file := range []struct {
		name string
		data []byte
	}{{"manifest.json", manifestData}, {"rules.json", rulesData}, {"background.js", background}, {"probe.html", probe}} {
		actual, err := os.ReadFile(filepath.Join(path, file.name))
		if err != nil {
			return err
		}
		if string(actual) != string(file.data) {
			return fmt.Errorf("generated %s does not match compiled policy", file.name)
		}
	}
	return nil
}
func lockTransactionPolicyRoot(root string) (func(), error) {
	path := filepath.Join(root, ".lock")
	deadline := time.Now().Add(5 * time.Second)
	for {
		err := os.Mkdir(path, 0700)
		if err == nil {
			return func() { _ = os.Remove(path) }, nil
		}
		if !os.IsExist(err) {
			return nil, err
		}
		info, statErr := os.Lstat(path)
		if statErr != nil {
			return nil, statErr
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsDir() {
			return nil, fmt.Errorf("unsafe transaction policy lock")
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for transaction policy state lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func publishTransactionPolicyCurrent(root, name string) error {
	current := filepath.Join(root, transactionPolicyCurrentFile)
	if info, err := os.Lstat(current); err == nil {
		if info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("unsafe transaction policy current pointer")
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	temp, err := os.CreateTemp(root, ".current-")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0600); err == nil {
		_, err = temp.WriteString(name + "\n")
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(tempName, current)
}
func retainTransactionPolicyGenerations(root, current string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	type generation struct {
		name string
		mod  time.Time
	}
	var gens []generation
	for _, entry := range entries {
		if !transactionPolicyGenerationName.MatchString(entry.Name()) {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if err := checkTransactionPolicyGeneration(path); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		gens = append(gens, generation{entry.Name(), info.ModTime()})
	}
	sort.Slice(gens, func(i, j int) bool { return gens[i].mod.After(gens[j].mod) })
	kept := 0
	for _, gen := range gens {
		if gen.name == current {
			continue // current is always retained, even if timestamps are skewed.
		}
		if kept < maxTransactionPolicyGenerations-1 {
			kept++
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, gen.name)); err != nil {
			return err
		}
	}
	return nil
}
