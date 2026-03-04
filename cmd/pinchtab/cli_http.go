package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
)

func doPost(client *http.Client, base, token, path string, body map[string]any) map[string]any {
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", base+path, strings.NewReader(string(data)))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Connection error: %v\n", err)
		fmt.Fprintf(os.Stderr, "   Make sure pinchtab server is running on %s\n", base)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", resp.StatusCode, string(respBody))
		os.Exit(1)
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		log.Printf("warning: error unmarshaling response: %v", err)
	}
	return result
}

func doGetRaw(client *http.Client, base, token, path string, params url.Values) []byte {
	u := base + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, _ := http.NewRequest("GET", u, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Connection error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}
	return body
}

func doDelete(client *http.Client, base, token, path string) {
	req, _ := http.NewRequest("DELETE", base+path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Connection error: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}
}

func doGet(client *http.Client, base, token, path string, params url.Values) map[string]any {
	u := base + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, _ := http.NewRequest("GET", u, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Connection error: %v\n", err)
		fmt.Fprintf(os.Stderr, "   Make sure pinchtab server is running on %s\n", base)
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		fmt.Fprintf(os.Stderr, "Error %d: %s\n", resp.StatusCode, string(body))
		os.Exit(1)
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("warning: error unmarshaling response: %v", err)
	}
	return result
}
