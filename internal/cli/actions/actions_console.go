package actions

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

func Console(conn apiclient.Conn, cmd *cobra.Command) {
	if v, _ := cmd.Flags().GetBool("clear"); v {
		ConsoleClear(conn, cmd)
		return
	}
	readLog(conn, cmd, "/console", "Failed to get console logs", printConsoleLogs)
}

func ConsoleClear(conn apiclient.Conn, cmd *cobra.Command) {
	clearLog(conn, cmd, "/console/clear", "Console logs cleared")
}

func Errors(conn apiclient.Conn, cmd *cobra.Command) {
	if v, _ := cmd.Flags().GetBool("clear"); v {
		ErrorsClear(conn, cmd)
		return
	}
	readLog(conn, cmd, "/errors", "Failed to get error logs", printErrorLogs)
}

func ErrorsClear(conn apiclient.Conn, cmd *cobra.Command) {
	clearLog(conn, cmd, "/errors/clear", "Error logs cleared")
}

func readLog(conn apiclient.Conn, cmd *cobra.Command, path, failMsg string, print func([]byte)) {
	params := tabParams(cmd)
	if v, _ := cmd.Flags().GetString("limit"); v != "" {
		params.Set("limit", v)
	}
	if v, _ := cmd.Flags().GetBool("json"); v {
		conn.Get(path, params)
		return
	}
	result := apiclient.DoGetRaw(conn.Client, conn.Base, conn.Token, path, params)
	if result == nil {
		runtimeErr("%s", failMsg)
	}
	print(result)
}

func clearLog(conn apiclient.Conn, cmd *cobra.Command, path, doneMsg string) {
	if conn.Post(path+"?"+tabParams(cmd).Encode(), nil) != nil {
		fmt.Println(doneMsg)
	}
}

func tabParams(cmd *cobra.Command) url.Values {
	params := url.Values{}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}
	return params
}

func printConsoleLogs(data []byte) {
	var resp struct {
		TabID   string `json:"tabId"`
		Console []struct {
			Timestamp time.Time `json:"timestamp"`
			Level     string    `json:"level"`
			Message   string    `json:"message"`
		} `json:"console"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		runtimeErr("Failed to parse response: %v", err)
	}

	if len(resp.Console) == 0 {
		fmt.Println("No console logs")
		return
	}

	for _, entry := range resp.Console {
		timeStr := entry.Timestamp.Format("15:04:05")
		level := strings.ToUpper(entry.Level)
		fmt.Printf("%s [%s] %s\n", timeStr, level, sanitizeTerminalText(entry.Message))
	}
}

func printErrorLogs(data []byte) {
	var resp struct {
		TabID  string `json:"tabId"`
		Errors []struct {
			Timestamp time.Time `json:"timestamp"`
			Message   string    `json:"message"`
			URL       string    `json:"url"`
			Line      int64     `json:"line"`
			Column    int64     `json:"column"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		runtimeErr("Failed to parse response: %v", err)
	}

	if len(resp.Errors) == 0 {
		fmt.Println("No errors")
		return
	}

	for _, entry := range resp.Errors {
		timeStr := entry.Timestamp.Format("15:04:05")
		fmt.Printf("%s [ERROR] %s\n", timeStr, sanitizeTerminalText(entry.Message))
		if entry.URL != "" {
			fmt.Printf("  at %s:%d:%d\n", sanitizeTerminalText(entry.URL), entry.Line, entry.Column)
		}
	}
}

func sanitizeTerminalText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if isTerminalControlRune(r) {
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isTerminalControlRune(r rune) bool {
	return (r >= 0 && r < 0x20) || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}
