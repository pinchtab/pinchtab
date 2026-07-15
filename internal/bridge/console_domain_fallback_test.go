package bridge

import (
	"testing"

	"github.com/pinchtab/pinchtab/internal/config"
)

func TestParseConsoleDomainEvent(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantOK  bool
		wantMsg consoleDomainMessage
	}{
		{
			name:   "console-api message",
			data:   `{"method":"Console.messageAdded","params":{"message":{"source":"console-api","level":"warning","text":"late-warn","url":"http://localhost:8088/console.html","line":6,"column":32}}}`,
			wantOK: true,
			wantMsg: consoleDomainMessage{
				Source: "console-api",
				Level:  "warning",
				Text:   "late-warn",
				URL:    "http://localhost:8088/console.html",
				Line:   6,
				Column: 32,
			},
		},
		{
			name:   "command reply ignored",
			data:   `{"id":1,"result":{}}`,
			wantOK: false,
		},
		{
			name:   "other event ignored",
			data:   `{"method":"Console.messagesCleared","params":{}}`,
			wantOK: false,
		},
		{
			name:   "malformed frame ignored",
			data:   `{"method":`,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, ok := parseConsoleDomainEvent([]byte(tt.data))
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && msg != tt.wantMsg {
				t.Errorf("msg = %+v, want %+v", msg, tt.wantMsg)
			}
		})
	}
}

func TestRecordConsoleDomainMessage(t *testing.T) {
	tm := &TabManager{logStore: NewConsoleLogStore(100)}

	tm.recordConsoleDomainMessage("tab1", consoleDomainMessage{
		Source: "console-api", Level: "error", Text: "boom", URL: "http://site/app.js",
	})
	tm.recordConsoleDomainMessage("tab1", consoleDomainMessage{
		Source: "javascript", Level: "error", Text: "Uncaught TypeError", URL: "http://site/app.js", Line: 3,
	})
	// Network noise and extension-internal sources never reach the stores.
	tm.recordConsoleDomainMessage("tab1", consoleDomainMessage{
		Source: "network", Level: "error", Text: "404",
	})
	tm.recordConsoleDomainMessage("tab1", consoleDomainMessage{
		Source: "console-api", Level: "log", Text: "ext", URL: "chrome-extension://abc/bg.js",
	})

	logs := tm.logStore.GetConsoleLogs("tab1", 0)
	if len(logs) != 1 {
		t.Fatalf("console logs = %d, want 1 (%+v)", len(logs), logs)
	}
	if logs[0].Level != "error" || logs[0].Message != "boom" {
		t.Errorf("unexpected console entry: %+v", logs[0])
	}

	errs := tm.logStore.GetErrorLogs("tab1", 0)
	if len(errs) != 1 {
		t.Fatalf("error logs = %d, want 1 (%+v)", len(errs), errs)
	}
	if errs[0].Message != "Uncaught TypeError" || errs[0].Line != 3 {
		t.Errorf("unexpected error entry: %+v", errs[0])
	}
}

func TestConsoleDomainEndpoint(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *config.RuntimeConfig
		want    string
		wantErr bool
	}{
		{
			name: "launched browser uses resolved debug port",
			cfg:  &config.RuntimeConfig{BrowserDebugPort: 9869},
			want: "ws://127.0.0.1:9869/devtools/page/TARGET1",
		},
		{
			name: "attach url host reused",
			cfg:  &config.RuntimeConfig{CDPAttachURL: "ws://10.0.0.5:9222/devtools/browser/abc"},
			want: "ws://10.0.0.5:9222/devtools/page/TARGET1",
		},
		{
			name: "secure attach upgrades to wss",
			cfg:  &config.RuntimeConfig{CDPAttachURL: "wss://remote:443/devtools/browser/abc"},
			want: "wss://remote:443/devtools/page/TARGET1",
		},
		{
			name:    "no endpoint known",
			cfg:     &config.RuntimeConfig{},
			wantErr: true,
		},
		{
			name:    "nil config",
			cfg:     nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := consoleDomainEndpoint(tt.cfg, "TARGET1")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("endpoint = %q, want %q", got, tt.want)
			}
		})
	}
}
