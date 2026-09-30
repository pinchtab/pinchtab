package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	benchPinchtabURL   = "http://localhost:9867"
	benchPinchtabToken = "benchmark-token"
)

func postJSON(ctx context.Context, client *http.Client, retryCfg RetryConfig, url string, headers map[string]string, body any) (map[string]any, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	resp, err := DoWithRetry(ctx, retryCfg, func() (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		return client.Do(req)
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, respBody)
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return result, nil
}

type pinchtabClient struct {
	baseURL string
	token   string
	client  *http.Client
}

func newBenchPinchtabClient(timeout time.Duration) *pinchtabClient {
	return &pinchtabClient{
		baseURL: benchPinchtabURL,
		token:   benchPinchtabToken,
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *pinchtabClient) do(method, path string) ([]byte, error) {
	req, err := http.NewRequest(method, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("build request %s %s: %w", method, path, err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, body)
	}
	return body, nil
}

func (c *pinchtabClient) health() error {
	_, err := c.do(http.MethodGet, "/health")
	return err
}

func (c *pinchtabClient) post(path string) error {
	_, err := c.do(http.MethodPost, path)
	return err
}

func (c *pinchtabClient) saveTo(path, dest string) error {
	body, err := c.do(http.MethodGet, path)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dest, body, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dest, err)
	}
	return nil
}
