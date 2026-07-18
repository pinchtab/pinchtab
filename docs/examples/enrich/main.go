// Command enrich demonstrates pinchtab library mode: enrich a single page
// through a running pinchtab server and print its BrowserPageData as JSON.
//
// Usage:
//
//	go run ./docs/examples/enrich --server http://localhost:9867 --token $PINCHTAB_TOKEN https://example.com
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/pinchtab/pinchtab/pkg/pinchtabaudit"
)

func main() {
	server := flag.String("server", "http://localhost:9867", "pinchtab server base URL")
	token := flag.String("token", os.Getenv("PINCHTAB_TOKEN"), "API token (defaults to $PINCHTAB_TOKEN)")
	screenshot := flag.Bool("screenshot", false, "capture a screenshot (inflates output)")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: enrich [--server URL] [--token TOKEN] <page-url>")
		os.Exit(2)
	}

	client := pinchtabaudit.New(*server, *token)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	page, err := client.EnrichPage(ctx, flag.Arg(0), &pinchtabaudit.PageOptions{
		Screenshot: pinchtabaudit.Bool(*screenshot),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "enrich: %v\n", err)
		os.Exit(1)
	}

	out, err := json.MarshalIndent(page, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}
