package apiclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/pinchtab/pinchtab/internal/api/types"
	"github.com/pinchtab/pinchtab/internal/cli/clistate"
)

const (
	vocabHeader      = types.HeaderVocab
	vocabTabIDHeader = types.HeaderTabID
	vocabStoreLimit  = 16
)

// vocabEntry pairs a snapshot's vocabulary token with the tab the server
// actually resolved it for, so a later action echoes the token only when it
// targets that same tab.
type vocabEntry struct {
	TabID string `json:"tabId"`
	Token string `json:"token"`
}

// vocabStore is one server's token records plus the tab the last implicit
// snapshot resolved, so an implicit action can echo that tab's token and tag it.
type vocabStore struct {
	Current string       `json:"current"`
	Entries []vocabEntry `json:"entries"`
}

// VocabForAction returns the tab id to tag and the token to echo for an action.
// An explicit --tab X uses X's own record; an implicit action uses the current
// pointer's tab. The returned tab is sent as VocabTabHeader so the server can
// ignore the token when the action resolves a different tab.
func VocabForAction(base, requestedTab string) (vocabTab, token string) {
	store := loadVocabStore(base)
	tab := requestedTab
	if tab == "" {
		tab = store.Current
	}
	if tab == "" {
		return "", ""
	}
	for _, e := range store.Entries {
		if e.TabID == tab {
			return tab, e.Token
		}
	}
	return tab, ""
}

// VocabTokenFor returns the token stored for a resolved tab id, or "".
func VocabTokenFor(base, tabID string) string {
	if tabID == "" {
		return ""
	}
	_, token := VocabForAction(base, tabID)
	return token
}

func storeVocabToken(base, tabID, token string, implicit bool) {
	if tabID == "" || token == "" {
		return
	}
	store := loadVocabStore(base)
	kept := store.Entries[:0]
	for _, e := range store.Entries {
		if e.TabID != tabID {
			kept = append(kept, e)
		}
	}
	kept = append(kept, vocabEntry{TabID: tabID, Token: token})
	if len(kept) > vocabStoreLimit {
		kept = kept[len(kept)-vocabStoreLimit:]
	}
	store.Entries = kept
	if implicit {
		store.Current = tabID
	}
	writeVocabStore(base, store)
}

func loadVocabStore(base string) vocabStore {
	data, err := os.ReadFile(vocabStorePath(base))
	if err != nil {
		return vocabStore{}
	}
	var store vocabStore
	if err := json.Unmarshal(data, &store); err != nil {
		return vocabStore{}
	}
	return store
}

func writeVocabStore(base string, store vocabStore) {
	path := vocabStorePath(base)
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	data, err := json.Marshal(store)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0600)
}

func vocabStorePath(base string) string {
	return filepath.Join(clistate.Dir(), "vocab-"+fileSlug(base))
}

func fileSlug(s string) string {
	s = strings.TrimPrefix(strings.TrimPrefix(s, "http://"), "https://")
	s = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-':
			return r
		default:
			return '-'
		}
	}, strings.Trim(s, "/"))
	if s == "" {
		return "default"
	}
	return s
}

// execute applies the standard fatal-on-transport-error + exit-on-HTTP-error
// policy and returns the body.
func execute(client *http.Client, token string, r request) []byte {
	status, body := mustRequest(client, token, r)
	exitOnAPIError(r, status, body)
	return body
}

// executeE is execute's returning twin: long-running commands use it when they
// need to release resources before reporting a request failure.
func executeE(client *http.Client, token string, r request) ([]byte, error) {
	status, body, err := doRequest(client, token, r)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if status >= http.StatusBadRequest {
		return nil, &StatusError{Status: status, Body: body, message: strings.TrimSpace(renderAPIError(r, status, body))}
	}
	return body, nil
}

func render(r request, body []byte) map[string]any {
	if r.quiet {
		return decodeObject(body)
	}
	return printAndDecode(body)
}

// decodeObject populates a map from an object response; array/scalar responses
// leave it nil, so callers that need a map should branch on result == nil.
func decodeObject(body []byte) map[string]any {
	var result map[string]any
	_ = json.Unmarshal(body, &result)
	return result
}

func prepend(first RequestOption, opts []RequestOption) []RequestOption {
	return append([]RequestOption{first}, opts...)
}

func DoGet(client *http.Client, base, token, path string, params url.Values, opts ...RequestOption) map[string]any {
	r := newRequest(http.MethodGet, base, path, prepend(WithQuery(params), opts)...)
	return render(r, execute(client, token, r))
}

func DoGetRaw(client *http.Client, base, token, path string, params url.Values, opts ...RequestOption) []byte {
	return execute(client, token, newRequest(http.MethodGet, base, path, prepend(WithQuery(params), opts)...))
}

func DoPost(client *http.Client, base, token, path string, body map[string]any, opts ...RequestOption) map[string]any {
	r := newRequest(http.MethodPost, base, path, prepend(WithBody(body), opts)...)
	return render(r, execute(client, token, r))
}

// DoPostQuiet is like DoPost but does not print the response body. Callers are
// responsible for rendering whatever output is appropriate (e.g. a single
// field for machine-friendly piping).
func DoPostQuiet(client *http.Client, base, token, path string, body map[string]any, opts ...RequestOption) map[string]any {
	return DoPost(client, base, token, path, body, prepend(Quiet(), opts)...)
}

// DoPostRaw sends a POST and returns the raw response body without printing.
// Exits on HTTP errors.
func DoPostRaw(client *http.Client, base, token, path string, body map[string]any, opts ...RequestOption) []byte {
	return execute(client, token, newRequest(http.MethodPost, base, path, prepend(WithBody(body), opts)...))
}

func DoPostQuietWithStatus(client *http.Client, base, token, path string, body map[string]any, opts ...RequestOption) (int, []byte, map[string]any) {
	status, respBody := mustRequest(client, token, newRequest(http.MethodPost, base, path, prepend(WithBody(body), opts)...))
	var result map[string]any
	if status < http.StatusBadRequest {
		result = decodeObject(respBody)
	}
	return status, respBody, result
}

func DoDelete(client *http.Client, base, token, path string, params url.Values, opts ...RequestOption) map[string]any {
	r := newRequest(http.MethodDelete, base, path, prepend(WithQuery(params), opts)...)
	return render(r, execute(client, token, r))
}

func DoRawE(client *http.Client, base, token, method, path string, opts ...RequestOption) ([]byte, error) {
	return executeE(client, token, newRequest(method, base, path, opts...))
}
