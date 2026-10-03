package bridge

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

func TestSetFileInputFilesRefusesANonFileElementNamingIt(t *testing.T) {
	ctx := newActionTab(t, 30*time.Second)
	html := `<body><input type="file" id="target"><button type="submit" id="send">Send</button></body>`
	if err := chromedp.Run(ctx,
		chromedp.Navigate("data:text/html;base64,"+base64.StdEncoding.EncodeToString([]byte(html))),
		chromedp.WaitVisible("#send", chromedp.ByID),
	); err != nil {
		t.Fatal(err)
	}
	b := &Bridge{}
	nodeID, err := b.ResolveSelectorToNodeID(ctx, "#send", nil, "")
	if err != nil {
		t.Fatal(err)
	}

	err = b.SetFileInputFiles(ctx, nodeID, []string{stageFile(t, t.TempDir(), "cv.txt", "cv")})

	if !errors.Is(err, ErrInvalidActionRequest) {
		t.Fatalf("got %v, want ErrInvalidActionRequest", err)
	}
	if msg := err.Error(); !strings.Contains(msg, `<button type="submit">`) || strings.Contains(msg, "-32000") {
		t.Fatalf("error %q should name the button and carry no raw CDP code", msg)
	}
}
