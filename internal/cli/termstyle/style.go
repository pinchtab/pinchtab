package termstyle

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Color string

type Style struct {
	foreground Color
	bold       bool
}

func NewStyle() Style {
	return Style{}
}

func (s Style) Foreground(color Color) Style {
	s.foreground = color
	return s
}

func (s Style) Bold(enabled bool) Style {
	s.bold = enabled
	return s
}

func (s Style) Render(text string) string {
	if !ShouldColorizeFile(os.Stdout) {
		return text
	}
	var codes []string
	if s.bold {
		codes = append(codes, "1")
	}
	if r, g, b, ok := parseHexColor(string(s.foreground)); ok {
		codes = append(codes, fmt.Sprintf("38;2;%d;%d;%d", r, g, b))
	}
	if len(codes) == 0 {
		return text
	}
	return "\x1b[" + strings.Join(codes, ";") + "m" + text + "\x1b[0m"
}

func RenderToFile(file *os.File, style Style, text string) string {
	if !ShouldColorizeFile(file) {
		return text
	}
	return style.Render(text)
}

func ShouldColorizeFile(file *os.File) bool {
	if file == nil {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if force := os.Getenv("CLICOLOR_FORCE"); force != "" && force != "0" {
		return true
	}
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func parseHexColor(value string) (int64, int64, int64, bool) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(trimmed) != 6 {
		return 0, 0, 0, false
	}
	raw, err := strconv.ParseUint(trimmed, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return int64((raw >> 16) & 0xff), int64((raw >> 8) & 0xff), int64(raw & 0xff), true
}
