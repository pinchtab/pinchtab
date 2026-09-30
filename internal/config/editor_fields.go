package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pinchtab/pinchtab/internal/safelog"
)

type fieldKind int

const (
	fieldString fieldKind = iota
	fieldInt
	fieldBool
	fieldList
)

type fieldSpec[T any] struct {
	key  string
	kind fieldKind
	get  func(*T) string
	set  func(t *T, path, value string) error
}

type fieldTable[T any] struct {
	section string
	fields  []fieldSpec[T]
}

func (tbl fieldTable[T]) lookup(field string) (fieldSpec[T], string, error) {
	path := tbl.section + "." + field
	for _, spec := range tbl.fields {
		if spec.key == field {
			return spec, path, nil
		}
	}
	return fieldSpec[T]{}, path, fmt.Errorf("unknown field %s", path)
}

func (tbl fieldTable[T]) setField(t *T, field, value string) error {
	spec, path, err := tbl.lookup(field)
	if err != nil {
		return err
	}
	return spec.set(t, path, value)
}

func (tbl fieldTable[T]) getField(t *T, field string) (string, error) {
	spec, _, err := tbl.lookup(field)
	if err != nil {
		return "", err
	}
	return spec.get(t), nil
}

func stringField[T any](key string, slot func(*T) *string) fieldSpec[T] {
	return fieldSpec[T]{
		key:  key,
		kind: fieldString,
		get:  func(t *T) string { return *slot(t) },
		set: func(t *T, _, value string) error {
			*slot(t) = value
			return nil
		},
	}
}

func stringPtrField[T any](key string, slot func(*T) **string) fieldSpec[T] {
	return fieldSpec[T]{
		key:  key,
		kind: fieldString,
		get:  func(t *T) string { return formatStringPtr(*slot(t)) },
		set: func(t *T, _, value string) error {
			*slot(t) = &value
			return nil
		},
	}
}

func intPtrField[T any](key string, slot func(*T) **int) fieldSpec[T] {
	return fieldSpec[T]{
		key:  key,
		kind: fieldInt,
		get:  func(t *T) string { return formatIntPtr(*slot(t)) },
		set:  func(t *T, path, value string) error { return setIntPtrField(slot(t), path, value) },
	}
}

func boolPtrField[T any](key string, slot func(*T) **bool) fieldSpec[T] {
	return fieldSpec[T]{
		key:  key,
		kind: fieldBool,
		get:  func(t *T) string { return formatBoolPtr(*slot(t)) },
		set:  func(t *T, path, value string) error { return setBoolPtrField(slot(t), path, value) },
	}
}

func listField[T any](key string, slot func(*T) *[]string, validate func([]string) error) fieldSpec[T] {
	return fieldSpec[T]{
		key:  key,
		kind: fieldList,
		get:  func(t *T) string { return strings.Join(*slot(t), ",") },
		set: func(t *T, _, value string) error {
			items := parseCSVList(value)
			if validate != nil {
				if err := validate(items); err != nil {
					return err
				}
			}
			*slot(t) = items
			return nil
		},
	}
}

func setIntPtrField(field **int, path, value string) error {
	n, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("%s must be a number: %w", path, err)
	}
	*field = &n
	return nil
}

func setBoolPtrField(field **bool, path, value string) error {
	b, err := parseBool(value)
	if err != nil {
		return fmt.Errorf("%s must be true or false: %w", path, err)
	}
	*field = &b
	return nil
}

var serverFields = fieldTable[ServerConfig]{
	section: "server",
	fields: []fieldSpec[ServerConfig]{
		stringField("port", func(s *ServerConfig) *string { return &s.Port }),
		stringField("bind", func(s *ServerConfig) *string { return &s.Bind }),
		stringField("token", func(s *ServerConfig) *string { return &s.Token }),
		stringField("stateDir", func(s *ServerConfig) *string { return &s.StateDir }),
		{
			key:  "logLevel",
			kind: fieldString,
			get:  func(s *ServerConfig) string { return s.LogLevel },
			set: func(s *ServerConfig, path, value string) error {
				if _, err := safelog.ParseLevel(value); err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				s.LogLevel = strings.ToLower(strings.TrimSpace(value))
				return nil
			},
		},
		intPtrField("networkBufferSize", func(s *ServerConfig) **int { return &s.NetworkBufferSize }),
		boolPtrField("retainNetworkBodies", func(s *ServerConfig) **bool { return &s.RetainNetworkBodies }),
		intPtrField("retainNetworkBodyMaxBytes", func(s *ServerConfig) **int { return &s.RetainNetworkBodyMaxBytes }),
		boolPtrField("trustProxyHeaders", func(s *ServerConfig) **bool { return &s.TrustProxyHeaders }),
		{
			key:  "cookieSecure",
			kind: fieldBool,
			get:  func(s *ServerConfig) string { return formatBoolPtr(s.CookieSecure) },
			set: func(s *ServerConfig, path, value string) error {
				switch strings.ToLower(strings.TrimSpace(value)) {
				case "", "auto", "null":
					s.CookieSecure = nil
					return nil
				}
				b, err := parseBool(value)
				if err != nil {
					return fmt.Errorf("%s must be true or false (or empty/auto/null to unset): %w", path, err)
				}
				s.CookieSecure = &b
				return nil
			},
		},
	},
}

var securityFields = fieldTable[SecurityConfig]{
	section: "security",
	fields: []fieldSpec[SecurityConfig]{
		boolPtrField("allowEvaluate", func(s *SecurityConfig) **bool { return &s.AllowEvaluate }),
		boolPtrField("allowClipboard", func(s *SecurityConfig) **bool { return &s.AllowClipboard }),
		boolPtrField("allowMacro", func(s *SecurityConfig) **bool { return &s.AllowMacro }),
		boolPtrField("allowScreencast", func(s *SecurityConfig) **bool { return &s.AllowScreencast }),
		boolPtrField("allowDownload", func(s *SecurityConfig) **bool { return &s.AllowDownload }),
		boolPtrField("allowCookies", func(s *SecurityConfig) **bool { return &s.AllowCookies }),
		boolPtrField("allowStateExport", func(s *SecurityConfig) **bool { return &s.AllowStateExport }),
		boolPtrField("allowUpload", func(s *SecurityConfig) **bool { return &s.AllowUpload }),
		boolPtrField("allowNetworkIntercept", func(s *SecurityConfig) **bool { return &s.AllowNetworkIntercept }),
		boolPtrField("allowMemory", func(s *SecurityConfig) **bool { return &s.AllowMemory }),
		boolPtrField("allowFileScheme", func(s *SecurityConfig) **bool { return &s.AllowFileScheme }),
		boolPtrField("trustLoopbackProxy", func(s *SecurityConfig) **bool { return &s.TrustLoopbackProxy }),
		stringPtrField("stateEncryptionKey", func(s *SecurityConfig) **string { return &s.StateEncryptionKey }),
		listField("allowedDomains", func(s *SecurityConfig) *[]string { return &s.AllowedDomains }, validateAllowlistEntries),
		listField("downloadAllowedDomains", func(s *SecurityConfig) *[]string { return &s.DownloadAllowedDomains }, nil),
		listField("trustedProxyCIDRs", func(s *SecurityConfig) *[]string { return &s.TrustedProxyCIDRs }, nil),
		listField("trustedResolveCIDRs", func(s *SecurityConfig) *[]string { return &s.TrustedResolveCIDRs }, nil),
		intPtrField("downloadMaxBytes", func(s *SecurityConfig) **int { return &s.DownloadMaxBytes }),
		intPtrField("memorySnapshotMaxBytes", func(s *SecurityConfig) **int { return &s.MemorySnapshotMaxBytes }),
		intPtrField("uploadMaxRequestBytes", func(s *SecurityConfig) **int { return &s.UploadMaxRequestBytes }),
		intPtrField("uploadMaxFiles", func(s *SecurityConfig) **int { return &s.UploadMaxFiles }),
		intPtrField("uploadMaxFileBytes", func(s *SecurityConfig) **int { return &s.UploadMaxFileBytes }),
		intPtrField("uploadMaxTotalBytes", func(s *SecurityConfig) **int { return &s.UploadMaxTotalBytes }),
		intPtrField("maxRedirects", func(s *SecurityConfig) **int { return &s.MaxRedirects }),
	},
}

func setSecurityField(s *SecurityConfig, field, value string) error {
	if sub, ok := strings.CutPrefix(field, "attach."); ok {
		return setAttachField(&s.Attach, sub, value)
	}
	if sub, ok := strings.CutPrefix(field, "idpi."); ok {
		return setIDPIField(s, sub, value)
	}
	return securityFields.setField(s, field, value)
}

func getSecurityField(s *SecurityConfig, field string) (string, error) {
	if sub, ok := strings.CutPrefix(field, "attach."); ok {
		return getAttachField(&s.Attach, sub)
	}
	if sub, ok := strings.CutPrefix(field, "idpi."); ok {
		return getIDPIField(s.IDPI, sub)
	}
	return securityFields.getField(s, field)
}
