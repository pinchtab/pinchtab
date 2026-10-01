package handlers

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/browserprobe"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

func TestMain(m *testing.M) {
	browserprobe.VersionTimeout = time.Minute
	os.Exit(testbrowser.Main(m))
}

func browserBridge(browserCtx context.Context, cfg *config.RuntimeConfig) *bridge.Bridge {
	b := bridge.New(context.Background(), browserCtx, cfg)
	b.SetTabCreateNavTimeout(testbrowser.TabLoadTimeout)
	return b
}
