package stealth

import (
	"os"
	"testing"
	"time"

	"github.com/pinchtab/pinchtab/internal/browserprobe"
)

func TestMain(m *testing.M) {
	browserprobe.VersionTimeout = time.Minute
	os.Exit(m.Run())
}
