package main

import (
	browseractions "github.com/pinchtab/pinchtab/internal/cli/actions"
	"github.com/spf13/cobra"
)

var setCmd = &cobra.Command{
	Use:   "set",
	Short: "Set browser emulation properties",
	Long:  "Commands for setting browser emulation properties such as viewport, geolocation, and network conditions.",
}

var setViewportCmd = &cobra.Command{
	Use:   "viewport <width> <height>",
	Short: "Set browser viewport dimensions",
	Long:  "Set the browser viewport dimensions using CDP emulation. Accepts optional --dpr and --mobile flags.",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.SetViewport(rt.client, rt.base, rt.token, cmd, args)
		})
	},
}

var setGeoCmd = &cobra.Command{
	Use:   "geo <latitude> <longitude>",
	Short: "Set browser geolocation",
	Long: `Set the browser geolocation using CDP emulation. Accepts optional --accuracy flag.

Negative coordinates need no escaping, so the western hemisphere and everything
south of the equator are written the same way as anywhere else:

  pinchtab set geo 51.5 -0.12 --tab <id>     # London
  pinchtab set geo -33.8 151.2 --tab <id>    # Sydney`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.SetGeolocation(rt.client, rt.base, rt.token, cmd, args)
		})
	},
}

var setOfflineCmd = &cobra.Command{
	Use:   "offline <true|false>",
	Short: "Enable or disable network offline emulation",
	Long:  "Enable or disable offline emulation for the tab: while offline every request on the tab fails as disconnected and navigator.onLine reports false. The state is per tab and survives navigation until turned off.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.SetOffline(rt.client, rt.base, rt.token, cmd, args)
		})
	},
}

var setHeadersCmd = &cobra.Command{
	Use:   "headers '<json>'",
	Short: "Set extra HTTP headers for all network requests",
	Long:  "Set extra HTTP headers using CDP network.SetExtraHTTPHeaders. Pass a JSON object as a single argument. An empty object {} clears extra headers.",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.SetHeaders(rt.client, rt.base, rt.token, cmd, args)
		})
	},
}

var setCredentialsCmd = &cobra.Command{
	Use:   "credentials <username> <password>",
	Short: "Set HTTP basic auth credentials",
	Long:  "Set HTTP basic auth credentials using CDP Fetch domain. The browser will automatically respond to 401/407 auth challenges.",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.SetCredentials(rt.client, rt.base, rt.token, cmd, args)
		})
	},
}

var setMediaCmd = &cobra.Command{
	Use:   "media <feature> <value>",
	Short: "Emulate a CSS media feature",
	Long:  "Emulate a CSS media feature using CDP emulation.SetEmulatedMedia. Example: pinchtab set media prefers-color-scheme dark",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		runCLI(func(rt cliRuntime) {
			browseractions.SetMedia(rt.client, rt.base, rt.token, cmd, args)
		})
	},
}

func init() {
	setViewportCmd.Flags().Float64("dpr", 0, "Device pixel ratio (default 1.0)")
	setViewportCmd.Flags().Bool("mobile", false, "Emulate mobile device")
	setGeoCmd.Flags().Float64("accuracy", 0, "Geolocation accuracy in meters (default 1.0)")

	setCmd.AddCommand(setViewportCmd)
	setCmd.AddCommand(setGeoCmd)
	setCmd.AddCommand(setOfflineCmd)
	setCmd.AddCommand(setHeadersCmd)
	setCmd.AddCommand(setCredentialsCmd)
	setCmd.AddCommand(setMediaCmd)
}
