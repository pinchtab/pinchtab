package actions

import (
	"fmt"
	"net/http"

	"github.com/pinchtab/pinchtab/internal/cli/apiclient"
)

// Dialog handles a JavaScript dialog (accept or dismiss).
func Dialog(client *http.Client, base, token string, action string, text string, tabID string) {
	body := map[string]any{"action": action}
	if text != "" {
		body["text"] = text
	}
	path := "/dialog"
	if tabID != "" {
		path = fmt.Sprintf("/tabs/%s/dialog", tabID)
	}
	apiclient.DoPost(client, base, token, path, body)
}
