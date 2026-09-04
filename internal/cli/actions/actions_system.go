package actions

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli"
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
	if ok {
		cli.WriteCommandHints(os.Stdout, "Next steps:", cli.NextStepsRunningHints, 64, false)
		return
	}
	cli.WriteCommandHints(os.Stdout, "Next steps:", []cli.CommandHint{
		{Command: "pinchtab daemon", Comment: "# check service status and logs"},
		{Command: "pinchtab security", Comment: "# review security posture"},
		{Command: "pinchtab health --json", Comment: "# full health payload"},
	}, 44, false)
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

	fmt.Print(formatProfileList(profiles))
}

func ProfilesCreate(client *http.Client, base, token, name string) {
	raw := apiclient.DoPostRaw(client, base, token, "/profiles", map[string]any{"name": name})
	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &created); err != nil || created.ID == "" || created.Name == "" {
		fmt.Fprintf(os.Stderr, "Failed to parse created profile: %s\n", strings.TrimSpace(string(raw)))
		return
	}
	fmt.Printf("%s\t%s\n", created.ID, created.Name)
}

// ProfilesPrune reclaims quarantine backlog. Without --confirm it reports what would go
// and frees nothing, so the bare invocation is safe for an agent to run.
func ProfilesPrune(client *http.Client, base, token string, cmd *cobra.Command) {
	confirm, _ := cmd.Flags().GetBool("confirm")
	profile, _ := cmd.Flags().GetString("profile")
	body := map[string]any{"confirm": confirm}
	if profile != "" {
		body["profile"] = profile
	}

	if jsonOutput, _ := cmd.Flags().GetBool("json"); jsonOutput {
		apiclient.DoPost(client, base, token, "/profiles/prune", body)
		return
	}

	raw := apiclient.DoPostRaw(client, base, token, "/profiles/prune", body)
	var resp struct {
		Removed    bool  `json:"removed"`
		Count      int   `json:"count"`
		TotalBytes int64 `json:"totalBytes"`
		Profiles   []struct {
			Name  string `json:"name"`
			Bytes int64  `json:"bytes"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse prune response: %v\n", err)
		os.Exit(1)
	}

	if resp.Count == 0 {
		fmt.Println("No quarantined profiles to reclaim")
		return
	}
	for _, prof := range resp.Profiles {
		fmt.Printf("%s\t%s\n", prof.Name, formatBytes(prof.Bytes))
	}
	if resp.Removed {
		fmt.Printf("\nReclaimed %d quarantined profile(s), %s freed\n", resp.Count, formatBytes(resp.TotalBytes))
		return
	}
	fmt.Printf("\n%d quarantined profile(s), %s reclaimable. Nothing was removed; re-run with --confirm.\n", resp.Count, formatBytes(resp.TotalBytes))
}

// formatProfileList lists user profiles first, then the quarantined
// directories under a heading carrying their count and combined size, so an
// operator sees what quarantine is holding without inferring it from names.
func formatProfileList(profiles []map[string]any) string {
	if len(profiles) == 0 {
		return "No profiles available\n"
	}

	var live, quarantined strings.Builder
	quarantinedCount := 0
	quarantinedBytes := int64(0)
	for _, prof := range profiles {
		id, _ := prof["id"].(string)
		name, _ := prof["name"].(string)
		size, _ := prof["diskUsage"].(float64)
		if isQuarantined, _ := prof["quarantined"].(bool); isQuarantined {
			quarantinedCount++
			quarantinedBytes += int64(size)
			fmt.Fprintf(&quarantined, "%s\t%s\t%s\n", id, name, formatBytes(int64(size)))
			continue
		}
		fmt.Fprintf(&live, "%s\t%s\n", id, name)
	}

	out := live.String()
	if quarantinedCount > 0 {
		out += fmt.Sprintf("\nQuarantined (%d, %s total):\n%s", quarantinedCount, formatBytes(quarantinedBytes), quarantined.String())
	}
	return out
}

func formatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for rest := n / unit; rest >= unit; rest /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func decodeProfilesResponse(body []byte) ([]map[string]any, error) {
	var profiles []map[string]any
	if err := json.Unmarshal(body, &profiles); err == nil {
		return profiles, nil
	}
	return nil, fmt.Errorf("expected /profiles to return a JSON array")
}

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
