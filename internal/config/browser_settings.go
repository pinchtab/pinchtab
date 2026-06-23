package config

import "strings"

func (fc *FileConfig) BrowserBinary() string {
	if fc == nil {
		return ""
	}
	return strings.TrimSpace(fc.Browser.BrowserBinary)
}

func (fc *FileConfig) BrowserDebugPort() int {
	if fc == nil || fc.Browser.BrowserDebugPort == nil {
		return 0
	}
	return *fc.Browser.BrowserDebugPort
}

func (fc *FileConfig) SetBrowserDebugPort(port int) {
	if fc == nil {
		return
	}
	if port <= 0 {
		fc.Browser.BrowserDebugPort = nil
		return
	}
	fc.Browser.BrowserDebugPort = intPtrIfPositive(port)
}

func (fc *FileConfig) BrowserExtraFlags() string {
	if fc == nil {
		return ""
	}
	return fc.Browser.BrowserExtraFlags
}

func (fc *FileConfig) BrowserVersion() string {
	if fc == nil {
		return ""
	}
	return fc.Browser.BrowserVersion
}
