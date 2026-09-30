package urls

func IsValid(rawURL string) bool {
	_, err := Sanitize(rawURL)
	return err == nil
}
