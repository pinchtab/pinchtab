package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/cli/output"
	"github.com/pinchtab/pinchtab/internal/server"
	"github.com/pinchtab/pinchtab/internal/session"
	"github.com/spf13/cobra"
)

type sessionCreateResult struct {
	ID     string   `json:"id"`
	Token  string   `json:"sessionToken"`
	Status string   `json:"status"`
	Grants []string `json:"grants,omitempty"`
}

// printSessionCreated splits the two values by stream on purpose. The token is the only
// thing on stdout, because the documented usage captures it with $(...). The id goes to
// stderr as a hint: it is the handle `session revoke` takes, and withholding it is why
// revoking your own session used to need a list-and-correlate detour through
// `session list`.
func printSessionCreated(result sessionCreateResult) {
	fmt.Println(result.Token)
	if result.ID != "" {
		output.Hint("session id " + result.ID + " " + sessionScopeSummary(result.Grants) +
			" — revoke takes the id: pinchtab session revoke " + result.ID)
	}
}

// sessionScopeSummary renders the scope the SERVER applied, read back from the create
// response rather than from the flags, so a request whose grants did not take effect is
// visible at the surface a human uses. Without it the one output that most needs the echo
// — the default, non-JSON create — printed the token and nothing about scope at all.
func sessionScopeSummary(grants []string) string {
	if len(grants) == 0 {
		return "(unscoped: reaches every non-admin route)"
	}
	return "(grants: " + strings.Join(grants, ", ") + ")"
}

func init() {
	sessionCmd := &cobra.Command{
		Use:     "session",
		Short:   "Agent session management",
		GroupID: "primary",
	}

	infoCmd := &cobra.Command{
		Use:   "info",
		Short: "Show current agent session details",
		Run: func(cmd *cobra.Command, args []string) {
			sessionToken := os.Getenv("PINCHTAB_SESSION")
			if sessionToken == "" {
				fmt.Fprintln(os.Stderr, "Error: no session set")
				output.Hint(cli.NoSessionHint)
				os.Exit(1)
			}
			runCLI(func(rt cliRuntime) {
				apiclient.DoGet(rt.client, rt.base, rt.token, "/sessions/me", nil)
			})
		},
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all agent sessions",
		Run: func(cmd *cobra.Command, args []string) {
			runCLI(func(rt cliRuntime) {
				apiclient.DoGet(rt.client, rt.base, rt.token, "/sessions", nil)
			})
		},
	}

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new agent session",
		Run: func(cmd *cobra.Command, args []string) {
			agentID, _ := cmd.Flags().GetString("agent-id")
			label, _ := cmd.Flags().GetString("label")
			grants, _ := cmd.Flags().GetStringSlice("grant")
			jsonOutput, _ := cmd.Flags().GetBool("json")
			if agentID == "" {
				fmt.Fprintln(os.Stderr, "Error: --agent-id is required")
				os.Exit(1)
			}
			// pflag parses an empty --grant value into an EMPTY slice, so the entry
			// that names nothing never reaches the validator and the flag reads as
			// absent: `--grant ""` exited 0 with a session that reaches every
			// non-admin route. Changed() is what tells the two apart, and the empty
			// name is handed to the one owner rather than a second refusal message
			// being written here.
			if cmd.Flags().Changed("grant") && len(grants) == 0 {
				grants = []string{""}
			}
			// Validated here as well as by the server so a typo costs no round trip
			// and the message can name the whole vocabulary.
			grants, err := session.ValidateGrants(grants)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
			body := map[string]any{"agentId": agentID}
			if label != "" {
				body["label"] = label
			}
			if len(grants) > 0 {
				body["grants"] = grants
			}
			// Auto-start the control plane first: the documented "create a session
			// before any browser command" order otherwise fails cold on a fresh
			// machine (only browser commands start the server).
			runCLIEnsuringServerNoBrowser("session create", func(rt cliRuntime) {
				if jsonOutput {
					apiclient.DoPost(rt.client, rt.base, rt.token, "/sessions", body)
					return
				}
				statusCode, rawBody, _ := apiclient.DoPostQuietWithStatus(rt.client, rt.base, rt.token, "/sessions", body)
				if statusCode >= 400 {
					exitSessionUnavailable(rawBody)
					apiclient.ExitWithAPIError(statusCode, rawBody)
				}
				var result sessionCreateResult
				if err := json.Unmarshal(rawBody, &result); err != nil {
					fmt.Fprintf(os.Stderr, "Error: failed to parse session response\n")
					os.Exit(1)
				}
				if result.Status != "active" {
					fmt.Fprintf(os.Stderr, "Error: session is %s\n", result.Status)
					output.Hint("create a new session: pinchtab session create --agent-id <id>")
					os.Exit(1)
				}
				printSessionCreated(result)
			})
		},
	}
	createCmd.Flags().String("agent-id", "", "Agent ID to associate with the session (required)")
	createCmd.Flags().String("label", "", "Optional human-readable label")
	createCmd.Flags().StringSlice("grant", nil,
		"Limit the session to one capability group; repeatable. Without it the session reaches every non-admin route. Valid: "+strings.Join(session.GrantNames(), ", "))
	addJSONFlag(createCmd)

	revokeCmd := &cobra.Command{
		Use:   "revoke <session-id>",
		Short: "Revoke an agent session",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			runCLI(func(rt cliRuntime) {
				apiclient.DoPost(rt.client, rt.base, rt.token, "/sessions/"+args[0]+"/revoke", nil)
			})
		},
	}

	sessionCmd.AddCommand(infoCmd, listCmd, createCmd, revokeCmd)
	rootCmd.AddCommand(sessionCmd)
}

// exitSessionUnavailable renders the two states in which the session family exists but is
// not mounted, each with the remedy that can actually succeed there. It decides on the
// CODE: the three states used to share a bare 404, so keying off the status printed a
// config edit at bridge users, for whom no config value mounts the route.
//
// Anything else — including an unrecognised code and a genuinely unknown path, which
// carries no code at all — falls through to the generic API error. A permissive default
// here would silently reuse the config remedy for the next mode, which is the conflation
// this replaced.
// Both halves are read because only one state has a remedy: enabling agent sessions on a
// full server is a file edit plus a restart, which is not one command, so that state carries
// its guidance as a hint. A reader of the remedy alone would print nothing there.
func sessionUnavailableAdvice(rawBody []byte) (message, hint, remedy string, ok bool) {
	var resp struct {
		Code    string `json:"code"`
		Error   string `json:"error"`
		Details struct {
			Hint   string `json:"hint"`
			Remedy string `json:"remedy"`
		} `json:"details"`
	}
	if json.Unmarshal(rawBody, &resp) != nil {
		return "", "", "", false
	}
	switch resp.Code {
	case server.CodeSessionsUnavailableInBridgeMode, server.CodeSessionsDisabled:
		return resp.Error, resp.Details.Hint, resp.Details.Remedy, true
	}
	return "", "", "", false
}

func exitSessionUnavailable(rawBody []byte) {
	message, hint, remedy, ok := sessionUnavailableAdvice(rawBody)
	if !ok {
		return
	}
	fmt.Fprintf(os.Stderr, "Error: %s\n", message)
	if hint != "" {
		output.Hint(hint)
	}
	if remedy != "" {
		output.Hint(remedy)
	}
	os.Exit(1)
}
