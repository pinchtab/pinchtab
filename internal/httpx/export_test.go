package httpx

func SafePath(base, userPath string) (string, error) {
	return SafeCreatePath(base, userPath)
}
