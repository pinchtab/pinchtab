package actions

import (
	"encoding/json"
	"fmt"
	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"io"
	"log"
	"net/http"
	"os"
)

func Health(client *http.Client, base, token string) {
	apiclient.DoGet(client, base, token, "/health", nil)
}

func Instances(client *http.Client, base, token string) {
	body := apiclient.DoGetRaw(client, base, token, "/instances", nil)

	instances, err := decodeInstancesResponse(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse instances: %v\n", err)
		os.Exit(1)
	}

	// Transform to cleaner output format
	output := make([]map[string]any, len(instances))
	for i, inst := range instances {
		id, _ := inst["id"].(string)
		port, _ := inst["port"].(string)
		headless, _ := inst["headless"].(bool)
		status, _ := inst["status"].(string)

		mode := "headless"
		if !headless {
			mode = "headed"
		}

		output[i] = map[string]any{
			"id":     id,
			"port":   port,
			"mode":   mode,
			"status": status,
		}
	}

	// Output as JSON
	data, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(data))
}

// --- profiles ---

func Profiles(client *http.Client, base, token string) {
	result := apiclient.DoGet(client, base, token, "/profiles", nil)

	// Display profiles in a friendly format
	if profiles, ok := result["profiles"].([]interface{}); ok && len(profiles) > 0 {
		fmt.Println()
		for _, prof := range profiles {
			if m, ok := prof.(map[string]any); ok {
				name, _ := m["name"].(string)

				fmt.Printf("%s %s\n", cli.StyleStdout(cli.ValueStyle, "profile:"), name)
			}
		}
		fmt.Println()
	} else {
		fmt.Println("No profiles available")
	}
}

// --- internal helpers ---

// getInstances fetches the list of running instances
func getInstances(client *http.Client, base, token string) []map[string]any {
	resp, err := http.NewRequest("GET", base+"/instances", nil)
	if err != nil {
		return nil
	}
	if token != "" {
		resp.Header.Set("Authorization", "Bearer "+token)
	}

	result, err := client.Do(resp)
	if err != nil || result.StatusCode >= 400 {
		return nil
	}
	defer func() { _ = result.Body.Close() }()

	body, err := io.ReadAll(result.Body)
	if err != nil {
		log.Printf("warning: error reading instances response: %v", err)
		return nil
	}

	instances, err := decodeInstancesResponse(body)
	if err != nil {
		log.Printf("warning: error decoding instances response: %v", err)
		return nil
	}
	return instances
}

// launchInstance launches a managed instance for the requested profile.
func launchInstance(client *http.Client, base, token string, profile string) {
	body := map[string]any{"profileId": profile}
	apiclient.DoPost(client, base, token, "/instances/start", body)
}

func decodeInstancesResponse(body []byte) ([]map[string]any, error) {
	var instances []map[string]any
	if err := json.Unmarshal(body, &instances); err == nil {
		return instances, nil
	}
	return nil, fmt.Errorf("expected /instances to return a JSON array")
}
