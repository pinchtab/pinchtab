package actions

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"

	"github.com/pinchtab/pinchtab/internal/cli"
	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
	"github.com/pinchtab/pinchtab/internal/heapsnap"
	"github.com/spf13/cobra"
)

type memoryUsage struct {
	TabID           string `json:"tabId"`
	UsedJSHeapSize  int64  `json:"usedJSHeapSize"`
	TotalJSHeapSize int64  `json:"totalJSHeapSize"`
	JSHeapSizeLimit int64  `json:"jsHeapSizeLimit"`
	Documents       int    `json:"documents"`
	Nodes           int    `json:"nodes"`
	Listeners       int    `json:"listeners"`
	Frames          int    `json:"frames"`
	GC              bool   `json:"gc"`
}

type memorySnapshot struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Bytes      int64  `json:"bytes"`
	NodeCount  int    `json:"nodeCount"`
	DurationMs int64  `json:"durationMs"`
	TabID      string `json:"tabId"`
}

type memorySummary struct {
	ID string `json:"id"`
	heapsnap.Summary
}

func Memory(conn apiclient.Conn, cmd *cobra.Command) {
	params := url.Values{}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		params.Set("tabId", v)
	}
	if gc, _ := cmd.Flags().GetBool("gc"); gc {
		params.Set("gc", "true")
	}
	if jsonOutput, _ := cmd.Flags().GetBool("json"); jsonOutput {
		conn.Get("/memory", params)
		return
	}
	body := apiclient.DoGetRaw(conn.Client, conn.Base, conn.Token, "/memory", params)
	var usage memoryUsage
	if err := json.Unmarshal(body, &usage); err != nil {
		fmt.Println(string(body))
		return
	}
	fmt.Print(formatMemoryUsage(usage))
}

func formatMemoryUsage(u memoryUsage) string {
	gc := ""
	if u.GC {
		gc = " (after gc)"
	}
	return fmt.Sprintf("heap used %s of %s (limit %s)%s\ndocuments=%d nodes=%d listeners=%d frames=%d\n",
		formatBytes(u.UsedJSHeapSize), formatBytes(u.TotalJSHeapSize), formatBytes(u.JSHeapSizeLimit), gc,
		u.Documents, u.Nodes, u.Listeners, u.Frames)
}

func MemorySnapshot(conn apiclient.Conn, cmd *cobra.Command) {
	body := map[string]any{}
	if v, _ := cmd.Flags().GetString("tab"); v != "" {
		body["tabId"] = v
	}
	raw := apiclient.DoPostRaw(conn.Client, conn.Base, conn.Token, "/memory/snapshot", body)
	var snap memorySnapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		cli.Fatal("Decode heap snapshot response: %v", err)
	}
	if out, _ := cmd.Flags().GetString("out"); out != "" {
		if err := copyLocalFile(snap.Path, out); err != nil {
			cli.Fatal("Copy %s → %s: %v (the file stays on the server at %s)", snap.Path, out, err, snap.Path)
		}
		snap.Path = out
	}
	if jsonOutput, _ := cmd.Flags().GetBool("json"); jsonOutput {
		encoded, _ := json.MarshalIndent(snap, "", "  ")
		fmt.Println(string(encoded))
		return
	}
	fmt.Println(cli.StyleStdout(cli.SuccessStyle, fmt.Sprintf("Heap snapshot %s → %s", snap.ID, snap.Path)))
	fmt.Printf("%s, %d nodes, %d ms\n", formatBytes(snap.Bytes), snap.NodeCount, snap.DurationMs)
	fmt.Printf("Summarize with: pinchtab memory summary %s\n", snap.ID)
}

func copyLocalFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func MemorySummary(conn apiclient.Conn, cmd *cobra.Command, id string) {
	params := url.Values{}
	if top, _ := cmd.Flags().GetInt("top"); top > 0 {
		params.Set("top", strconv.Itoa(top))
	}
	path := "/memory/snapshot/" + url.PathEscape(id) + "/summary"
	if jsonOutput, _ := cmd.Flags().GetBool("json"); jsonOutput {
		conn.Get(path, params)
		return
	}
	body := apiclient.DoGetRaw(conn.Client, conn.Base, conn.Token, path, params)
	var summary memorySummary
	if err := json.Unmarshal(body, &summary); err != nil {
		fmt.Println(string(body))
		return
	}
	fmt.Print(formatMemorySummary(summary))
}

func formatMemorySummary(s memorySummary) string {
	out := fmt.Sprintf("%s: %d nodes, %d edges, %s self size\n", s.ID, s.NodeCount, s.EdgeCount, formatBytes(s.TotalSelfSize))
	out += "\nTop constructors by self size\n" + constructorTable(s.TopBySize)
	out += "\nTop constructors by count\n" + constructorTable(s.TopByCount)
	out += "\nDuplicate strings\n"
	if len(s.DuplicateStrings) == 0 {
		return out + "  (none)\n"
	}
	out += fmt.Sprintf("  %8s  %10s  %s\n", "COUNT", "SELF SIZE", "VALUE")
	for _, d := range s.DuplicateStrings {
		out += fmt.Sprintf("  %8d  %10s  %q\n", d.Count, formatBytes(d.SelfSize), d.Value)
	}
	return out
}

func constructorTable(rows []heapsnap.Constructor) string {
	out := fmt.Sprintf("  %-32s  %8s  %10s\n", "CONSTRUCTOR", "COUNT", "SELF SIZE")
	for _, c := range rows {
		out += fmt.Sprintf("  %-32s  %8d  %10s\n", c.Name, c.Count, formatBytes(c.SelfSize))
	}
	return out
}

type memoryComparison struct {
	Top      int  `json:"top"`
	Retained bool `json:"retained"`
	heapsnap.Comparison
}

func MemoryCompare(conn apiclient.Conn, cmd *cobra.Command, baseID, headID string) {
	params := url.Values{}
	params.Set("base", baseID)
	params.Set("head", headID)
	if top, _ := cmd.Flags().GetInt("top"); top > 0 {
		params.Set("top", strconv.Itoa(top))
	}
	if retained, _ := cmd.Flags().GetBool("retained"); retained {
		params.Set("retained", "true")
	}
	if jsonOutput, _ := cmd.Flags().GetBool("json"); jsonOutput {
		conn.Get("/memory/compare", params)
		return
	}
	body := apiclient.DoGetRaw(conn.Client, conn.Base, conn.Token, "/memory/compare", params)
	var cmp memoryComparison
	if err := json.Unmarshal(body, &cmp); err != nil {
		fmt.Println(string(body))
		return
	}
	fmt.Print(formatMemoryComparison(cmp))
}

func formatMemoryComparison(c memoryComparison) string {
	out := fmt.Sprintf("%s → %s: self size %s → %s (%s), nodes %+d, %d constructors changed\n",
		c.Base.ID, c.Head.ID, formatBytes(c.Base.TotalSelfSize), formatBytes(c.Head.TotalSelfSize),
		formatSignedBytes(c.SizeDelta), c.NodeDelta, c.Changed)
	out += "\nConstructors by size delta\n"
	if len(c.Constructors) == 0 {
		out += "  (no change)\n"
	} else {
		header := fmt.Sprintf("  %-32s  %8s  %12s  %10s  %10s", "CONSTRUCTOR", "COUNT Δ", "SIZE Δ", "BASE", "HEAD")
		if c.Retained {
			header += fmt.Sprintf("  %10s", "RETAINED")
		}
		out += header + "\n"
		for _, r := range c.Constructors {
			line := fmt.Sprintf("  %-32s  %+8d  %12s  %10s  %10s", r.Name, r.CountDelta, formatSignedBytes(r.SizeDelta), formatBytes(r.BaseSelfSize), formatBytes(r.HeadSelfSize))
			if r.RetainedSize != nil {
				line += fmt.Sprintf("  %10s", formatBytes(*r.RetainedSize))
			}
			out += line + "\n"
		}
	}
	out += "\nNew duplicate strings\n"
	if len(c.NewDuplicateStrings) == 0 {
		return out + "  (none)\n"
	}
	out += fmt.Sprintf("  %8s  %10s  %s\n", "COUNT", "SELF SIZE", "VALUE")
	for _, d := range c.NewDuplicateStrings {
		out += fmt.Sprintf("  %8d  %10s  %q\n", d.Count, formatBytes(d.SelfSize), d.Value)
	}
	return out
}

func formatSignedBytes(n int64) string {
	if n < 0 {
		return "-" + formatBytes(-n)
	}
	return "+" + formatBytes(n)
}
