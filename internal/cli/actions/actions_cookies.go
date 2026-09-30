package actions

import (
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/spf13/cobra"
)

// CookiesGet reads the cookies visible to a tab's current URL.
func CookiesGet(conn apiclient.Conn, cmd *cobra.Command) {
	params := url.Values{}
	if tab, _ := cmd.Flags().GetString("tab"); tab != "" {
		params.Set("tabId", tab)
	}
	if name, _ := cmd.Flags().GetString("name"); name != "" {
		params.Set("name", name)
	}
	if target, _ := cmd.Flags().GetString("url"); target != "" {
		params.Set("url", target)
	}

	result := requireBytes(apiclient.DoGetRaw(conn.Client, conn.Base, conn.Token, "/cookies", params), "Failed to get cookies")
	printIndented(decodeMap(result, "Failed to parse response"))
}

// CookiesSet sets one cookie on the tab's current URL. The URL is left to the
// server to default from the tab, so injecting a session cookie is one command
// and never needs the caller to look the page up first.
func CookiesSet(conn apiclient.Conn, cmd *cobra.Command, name, value string) {
	cookie := map[string]any{"name": name, "value": value}
	for flag, key := range map[string]string{
		"domain":    "domain",
		"path":      "path",
		"same-site": "sameSite",
	} {
		if v, _ := cmd.Flags().GetString(flag); v != "" {
			cookie[key] = v
		}
	}
	if secure, _ := cmd.Flags().GetBool("secure"); secure {
		cookie["secure"] = true
	}
	if httpOnly, _ := cmd.Flags().GetBool("http-only"); httpOnly {
		cookie["httpOnly"] = true
	}

	body := map[string]any{"cookies": []any{cookie}}
	if tab, _ := cmd.Flags().GetString("tab"); tab != "" {
		body["tabId"] = tab
	}
	if target, _ := cmd.Flags().GetString("url"); target != "" {
		body["url"] = target
	}

	result := requireMap(apiclient.DoPostQuiet(conn.Client, conn.Base, conn.Token, "/cookies", body), "Failed to set cookie")
	if !cookieWriteConfirmed(result) {
		runtimeErr("ERROR: cookies: %q was not set (%v)", name, jsonLine(result))
	}
	printCookiesResult(cmd, result)
}

// CookiesClear clears all browser cookies.
func CookiesClear(conn apiclient.Conn, cmd *cobra.Command) {
	result := apiclient.DoDelete(conn.Client, conn.Base, conn.Token, "/cookies", nil, apiclient.Quiet())
	if result == nil {
		runtimeErr("ERROR: cookies: clear failed")
	}
	printCookiesResult(cmd, result)
}

func printCookiesResult(cmd *cobra.Command, result map[string]any) {
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		printIndented(result)
		return
	}
	fmt.Println("OK")
}

// cookieWriteConfirmed reports whether a /cookies response proves the browser stored the
// cookie. It fails CLOSED: an absent or unreadable "set" has not confirmed the write, which
// is not the same as confirming it, so renaming the field breaks the command rather than
// silently retiring the check. Separate from the exit path so the rule is testable.
func cookieWriteConfirmed(result map[string]any) bool {
	set, ok := result["set"].(float64)
	return ok && set >= 1
}

func jsonLine(v any) string {
	out, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(out)
}
