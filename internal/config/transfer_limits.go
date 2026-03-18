package config

const (
	DefaultDownloadMaxBytes      = 20 << 20
	MaxDownloadMaxBytes          = 100 << 20
	DefaultUploadMaxRequestBytes = 10 << 20
	MaxUploadMaxRequestBytes     = 100 << 20
	DefaultUploadMaxFiles        = 8
	MaxUploadMaxFiles            = 32
	DefaultUploadMaxFileBytes    = 5 << 20
	MaxUploadMaxFileBytes        = 25 << 20
	DefaultUploadMaxTotalBytes   = 10 << 20
	MaxUploadMaxTotalBytes       = 100 << 20
)

func clampPositiveLimit(value, fallback, max int) int {
	if value <= 0 {
		return fallback
	}
	if value > max {
		return max
	}
	return value
}

func (cfg *RuntimeConfig) EffectiveDownloadMaxBytes() int {
	if cfg == nil {
		return DefaultDownloadMaxBytes
	}
	return clampPositiveLimit(cfg.DownloadMaxBytes, DefaultDownloadMaxBytes, MaxDownloadMaxBytes)
}

func (cfg *RuntimeConfig) EffectiveUploadMaxRequestBytes() int {
	if cfg == nil {
		return DefaultUploadMaxRequestBytes
	}
	return clampPositiveLimit(cfg.UploadMaxRequestBytes, DefaultUploadMaxRequestBytes, MaxUploadMaxRequestBytes)
}

func (cfg *RuntimeConfig) EffectiveUploadMaxFiles() int {
	if cfg == nil {
		return DefaultUploadMaxFiles
	}
	return clampPositiveLimit(cfg.UploadMaxFiles, DefaultUploadMaxFiles, MaxUploadMaxFiles)
}

func (cfg *RuntimeConfig) EffectiveUploadMaxFileBytes() int {
	if cfg == nil {
		return DefaultUploadMaxFileBytes
	}
	return clampPositiveLimit(cfg.UploadMaxFileBytes, DefaultUploadMaxFileBytes, MaxUploadMaxFileBytes)
}

func (cfg *RuntimeConfig) EffectiveUploadMaxTotalBytes() int {
	if cfg == nil {
		return DefaultUploadMaxTotalBytes
	}
	return clampPositiveLimit(cfg.UploadMaxTotalBytes, DefaultUploadMaxTotalBytes, MaxUploadMaxTotalBytes)
}
