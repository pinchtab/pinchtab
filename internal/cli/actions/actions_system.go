package actions

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/spf13/cobra"
)

func Health(client *http.Client, base, token string, cmd *cobra.Command) {
	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		apiclient.DoGet(client, base, token, "/health", nil)
		return
	}

	body := apiclient.DoGetRaw(client, base, token, "/health", nil)
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		output.Value("ok")
		printHealthHints(true)
		return
	}
	status, _ := result["status"].(string)
	if status == "ok" {
		output.Value("ok")
		printHealthHints(true)
		return
	}
	reason, _ := result["reason"].(string)
	if reason != "" {
		output.Value("degraded: " + reason)
	} else {
		output.Value(status)
	}
	printHealthHints(false)
}

func printHealthHints(ok bool) {
	_, _ = fmt.Fprintln(os.Stdout)
	_, _ = fmt.Fprintln(os.Stdout, "Next steps:")
	if ok {
		_, _ = fmt.Fprintf(os.Stdout, "  %-64s %s\n", "export PINCHTAB_SESSION=$(pinchtab session create --agent-id <id>)", "# start a dedicated session")
		_, _ = fmt.Fprintf(os.Stdout, "  %-64s %s\n", "pinchtab nav <url>", "# open a page in the current tab")
		_, _ = fmt.Fprintf(os.Stdout, "  %-64s %s\n", "pinchtab snap", "# inspect interactive elements")
		return
	}
	_, _ = fmt.Fprintf(os.Stdout, "  %-44s %s\n", "pinchtab daemon", "# check service status and logs")
	_, _ = fmt.Fprintf(os.Stdout, "  %-44s %s\n", "pinchtab security", "# review security posture")
	_, _ = fmt.Fprintf(os.Stdout, "  %-44s %s\n", "pinchtab health --json", "# full health payload")
}

func Instances(client *http.Client, base, token string, cmd *cobra.Command) {
	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		apiclient.DoGet(client, base, token, "/instances", nil)
		return
	}

	body := apiclient.DoGetRaw(client, base, token, "/instances", nil)

	instances, err := decodeInstancesResponse(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse instances: %v\n", err)
		os.Exit(1)
	}

	if len(instances) == 0 {
		fmt.Println("No instances running")
		return
	}

	// Human-readable: id  port  mode  status
	for _, inst := range instances {
		id, _ := inst["id"].(string)
		port, _ := inst["port"].(string)
		headless, _ := inst["headless"].(bool)
		status, _ := inst["status"].(string)

		mode := "headless"
		if !headless {
			mode = "headed"
		}

		fmt.Printf("%s\t%s\t%s\t%s\n", id, port, mode, status)
	}
}

// --- profiles ---

func Profiles(client *http.Client, base, token string, cmd *cobra.Command) {
	jsonOutput, _ := cmd.Flags().GetBool("json")
	if jsonOutput {
		apiclient.DoGet(client, base, token, "/profiles", nil)
		return
	}

	body := apiclient.DoGetRaw(client, base, token, "/profiles", nil)

	profiles, err := decodeProfilesResponse(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse profiles: %v\n", err)
		os.Exit(1)
	}

	if len(profiles) == 0 {
		fmt.Println("No profiles available")
		return
	}

	for _, prof := range profiles {
		id, _ := prof["id"].(string)
		name, _ := prof["name"].(string)
		fmt.Printf("%s\t%s\n", id, name)
	}
}

func decodeProfilesResponse(body []byte) ([]map[string]any, error) {
	var profiles []map[string]any
	if err := json.Unmarshal(body, &profiles); err == nil {
		return profiles, nil
	}
	return nil, fmt.Errorf("expected /profiles to return a JSON array")
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

func decodeInstancesResponse(body []byte) ([]map[string]any, error) {
	var instances []map[string]any
	if err := json.Unmarshal(body, &instances); err == nil {
		return instances, nil
	}
	return nil, fmt.Errorf("expected /instances to return a JSON array")
}
