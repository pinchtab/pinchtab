package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

type profileInstanceStatus struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
	Status  string `json:"status"`
	Port    string `json:"port"`
	ID      string `json:"id"`
	Error   string `json:"error"`
}

func handleConnectCommand() {
	profile := ""
	dashboardURL := os.Getenv("PINCHTAB_DASHBOARD_URL")
	jsonOut := false

	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--json":
			jsonOut = true
		case "--dashboard":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "missing value for --dashboard")
				os.Exit(2)
			}
			i++
			dashboardURL = args[i]
		default:
			if profile == "" {
				profile = arg
			}
		}
	}

	if profile == "" {
		fmt.Fprintln(os.Stderr, "Usage: pinchtab connect <profile> [--dashboard http://localhost:9867] [--json]")
		os.Exit(2)
	}
	if dashboardURL == "" {
		dashboardURL = "http://localhost:" + cfg.Port
	}

	reqURL := stringsTrimRightSlash(dashboardURL) + "/profiles/" + url.PathEscape(profile) + "/instance"
	res, err := http.Get(reqURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 8<<10))
		fmt.Fprintf(os.Stderr, "connect failed: dashboard returned %d: %s\n", res.StatusCode, string(body))
		os.Exit(1)
	}

	var st profileInstanceStatus
	if err := json.NewDecoder(res.Body).Decode(&st); err != nil {
		fmt.Fprintf(os.Stderr, "connect failed: invalid response: %v\n", err)
		os.Exit(1)
	}
	if !st.Running || st.Port == "" {
		errMsg := st.Error
		if errMsg == "" {
			errMsg = st.Status
		}
		fmt.Fprintf(os.Stderr, "profile %q not running (%s)\n", profile, errMsg)
		os.Exit(1)
	}

	instanceURL := "http://localhost:" + st.Port
	if jsonOut {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]string{
			"profile": st.Name,
			"id":      st.ID,
			"status":  st.Status,
			"port":    st.Port,
			"url":     instanceURL,
		})
		return
	}

	fmt.Println(instanceURL)
}

func stringsTrimRightSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
