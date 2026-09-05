package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/spf13/cobra"
)

var cookiesCmd = &cobra.Command{
	Use:   "cookies",
	Short: "Manage browser cookies",
	Long:  "Read, set and clear browser cookies for the tab you are driving.",
}

var cookiesGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get cookies for the current page",
	Long:  "Read the cookies visible to a tab's current URL, with values. Use --name for one cookie, --url to read another origin.",
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.CookiesGet(rt.client, rt.base, rt.token, cmd)
		})
	},
}

var cookiesSetCmd = &cobra.Command{
	Use:   "set <name> <value>",
	Short: "Set a cookie on the current page",
	Long: `Set one cookie on the tab's current URL, which is how a session is reused
without replaying a whole saved state. The URL defaults to the tab's current page;
pass --url to target another origin. An empty value is honoured, blanking the
cookie without deleting it.`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.CookiesSet(rt.client, rt.base, rt.token, cmd, args[0], args[1])
		})
	},
}

var cookiesClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear every cookie in the browser (all origins, not just this tab)",
	Long: `Clear all browser cookies via CDP. This affects all origins and cannot be
scoped to one tab or one domain — there is no per-cookie removal verb. Nothing in
the CLI restores them; re-set what you need with cookies set, or reload a saved
state with state load.`,
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.CookiesClear(rt.client, rt.base, rt.token, cmd)
		})
	},
}

func init() {
	addTabFlag(cookiesGetCmd, cookiesSetCmd)

	cookiesGetCmd.Flags().String("name", "", "Only return the cookie with this name")
	cookiesGetCmd.Flags().String("url", "", "Read cookies for this URL instead of the tab's current page")

	cookiesSetCmd.Flags().String("url", "", "Target URL (default: the tab's current page)")
	cookiesSetCmd.Flags().String("domain", "", "Cookie domain")
	cookiesSetCmd.Flags().String("path", "", "Cookie path")
	cookiesSetCmd.Flags().String("same-site", "", "SameSite attribute: Strict, Lax or None")
	cookiesSetCmd.Flags().Bool("secure", false, "Mark the cookie Secure")
	cookiesSetCmd.Flags().Bool("http-only", false, "Mark the cookie HttpOnly")
}
