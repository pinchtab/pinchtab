package actions

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
)

func Quick(client *http.Client, base, token string, args []string) {
	if len(args) < 1 {
		cli.Fatal("Usage: pinchtab quick <url>")
	}

	fmt.Println(cli.StyleStdout(cli.HeadingStyle, fmt.Sprintf("Navigating to %s...", args[0])))

	// Let the server settle the page before we snapshot, reusing navigation's
	// own readiness contract (waitForNavigationState). "networkidle" requires a
	// complete readyState plus a stable URL and is bounded server-side (~3s
	// ceiling), so it returns promptly on fast pages and replaces the old fixed
	// 1s sleep without regressing slow ones.
	navBody := map[string]any{"url": args[0], "waitFor": "networkidle"}
	navResult := apiclient.DoPost(client, base, token, "/navigate", navBody)

	fmt.Println()
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Page structure"))

	snapParams := url.Values{}
	snapParams.Set("filter", "interactive")
	snapParams.Set("compact", "true")
	apiclient.DoGet(client, base, token, "/snapshot", snapParams)

	if title, ok := navResult["title"].(string); ok {
		fmt.Println()
		fmt.Printf("%s %s\n", cli.StyleStdout(cli.MutedStyle, "Title:"), cli.StyleStdout(cli.ValueStyle, title))
	}
	if urlStr, ok := navResult["url"].(string); ok {
		fmt.Printf("%s %s\n", cli.StyleStdout(cli.MutedStyle, "URL:"), cli.StyleStdout(cli.ValueStyle, urlStr))
	}

	fmt.Println()
	fmt.Println(cli.StyleStdout(cli.HeadingStyle, "Quick actions"))
	fmt.Printf("  %s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab click <ref>"), cli.StyleStdout(cli.MutedStyle, "# Click an element (use refs from above)"))
	fmt.Printf("  %s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab type <ref> <text>"), cli.StyleStdout(cli.MutedStyle, "# Type into input field"))
	fmt.Printf("  %s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab screenshot"), cli.StyleStdout(cli.MutedStyle, "# Take a screenshot"))
	fmt.Printf("  %s %s\n", cli.StyleStdout(cli.CommandStyle, "pinchtab pdf --tab <id> -o output.pdf"), cli.StyleStdout(cli.MutedStyle, "# Save tab as PDF"))
}
