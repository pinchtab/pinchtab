package staticfetch_test

import (
	"os"
	"testing"

	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

func TestMain(m *testing.M) { os.Exit(testbrowser.Main(m)) }
