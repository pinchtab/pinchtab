package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/spf13/cobra"
)

var dashboardCmd = &cobra.Command{
	Use:   "dashboard",
	Short: "Open the dashboard in your browser",
	Long:  "Resolve the dashboard URL, copy the auth token to clipboard, and open the browser.",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		noOpen, _ := cmd.Flags().GetBool("no-open")
		portOverride, _ := cmd.Flags().GetString("port")
		runDashboardCommand(cfg.Port, cfg.Bind, cfg.Token, noOpen, portOverride)
	},
}

func init() {
	dashboardCmd.GroupID = "primary"
	dashboardCmd.Flags().Bool("no-open", false, "Print URL without opening the browser")
	dashboardCmd.Flags().String("port", "", "Override dashboard port")
	rootCmd.AddCommand(dashboardCmd)
}

func runDashboardCommand(cfgPort, cfgBind, cfgToken string, noOpen bool, portOverride string) {
	port, err := resolvePort(cfgPort, portOverride)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
	host := resolveHost(cfgBind)
	url := fmt.Sprintf("http://%s:%s", host, port)

	// Health check
	if !isDashboardReachable(host, port) {
		fmt.Println(cli.StyleStdout(cli.WarningStyle, "  Dashboard doesn't appear to be running."))
		fmt.Printf("  Start with %s or %s\n\n",
			cli.StyleStdout(cli.CommandStyle, "pinchtab server"),
			cli.StyleStdout(cli.CommandStyle, "pinchtab daemon start"))
	}

	fmt.Printf("%s %s\n", cli.StyleStdout(cli.HeadingStyle, "Dashboard:"), cli.StyleStdout(cli.ValueStyle, url))

	// Copy token to clipboard (never embed in URL)
	if cfgToken != "" {
		if err := copyToClipboard(cfgToken); err == nil {
			fmt.Println(cli.StyleStdout(cli.SuccessStyle, "  Token copied to clipboard") +
				cli.StyleStdout(cli.MutedStyle, " — paste it on the login page."))
		} else {
			fmt.Println(cli.StyleStdout(cli.WarningStyle, "  Token could not be copied to clipboard."))
		}
	}

	// Open browser
	if !noOpen {
		if err := openBrowser(url); err == nil {
			fmt.Println(cli.StyleStdout(cli.SuccessStyle, "  Opened in your browser."))
		} else {
			fmt.Printf("  Open this URL in your browser: %s\n", cli.StyleStdout(cli.ValueStyle, url))
		}
	} else {
		fmt.Printf("  Open this URL in your browser: %s\n", cli.StyleStdout(cli.ValueStyle, url))
	}
}

func resolvePort(cfgPort, override string) (string, error) {
	port := cfgPort
	if override != "" {
		port = override
	}
	n, err := strconv.Atoi(port)
	if err != nil {
		return "", fmt.Errorf("invalid port %q: must be a number", port)
	}
	if n < 1 || n > 65535 {
		return "", fmt.Errorf("invalid port %d: must be between 1 and 65535", n)
	}
	return port, nil
}

func resolveHost(bind string) string {
	switch bind {
	case "", "loopback", "localhost", "lan", "0.0.0.0":
		return "127.0.0.1"
	default:
		return bind
	}
}

func isDashboardReachable(host, port string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 1*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func openBrowser(url string) error {
	// url is constructed internally from config host+port, not raw user input.
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url) // #nosec G204 -- url built from internal config values (host:port)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url) // #nosec G204 -- url built from internal config values (host:port)
	default:
		cmd = exec.Command("xdg-open", url) // #nosec G204 -- url built from internal config values (host:port)
	}
	return cmd.Start()
}
