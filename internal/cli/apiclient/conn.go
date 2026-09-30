package apiclient

import (
	"net/http"
	"net/url"
)

type Conn struct {
	Client *http.Client
	Base   string
	Token  string
}

func (c Conn) Get(path string, params url.Values, opts ...RequestOption) map[string]any {
	return DoGet(c.Client, c.Base, c.Token, path, params, opts...)
}

func (c Conn) Post(path string, body map[string]any, opts ...RequestOption) map[string]any {
	return DoPost(c.Client, c.Base, c.Token, path, body, opts...)
}

func (c Conn) Raw(method, path string, opts ...RequestOption) ([]byte, error) {
	return DoRawE(c.Client, c.Base, c.Token, method, path, opts...)
}

func (c Conn) TabPath(tabID, suffix string) string {
	return TabPath(tabID, suffix)
}
