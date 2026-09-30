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

func (c Conn) WithClient(client *http.Client) Conn {
	c.Client = client
	return c
}

func (c Conn) GetRaw(path string, params url.Values, opts ...RequestOption) []byte {
	return DoGetRaw(c.Client, c.Base, c.Token, path, params, opts...)
}

func (c Conn) PostQuiet(path string, body map[string]any, opts ...RequestOption) map[string]any {
	return DoPostQuiet(c.Client, c.Base, c.Token, path, body, opts...)
}

func (c Conn) PostQuietWithStatus(path string, body map[string]any, opts ...RequestOption) (int, []byte, map[string]any) {
	return DoPostQuietWithStatus(c.Client, c.Base, c.Token, path, body, opts...)
}

func (c Conn) PostRaw(path string, body map[string]any, opts ...RequestOption) []byte {
	return DoPostRaw(c.Client, c.Base, c.Token, path, body, opts...)
}

func (c Conn) Delete(path string, params url.Values, opts ...RequestOption) map[string]any {
	return DoDelete(c.Client, c.Base, c.Token, path, params, opts...)
}

func (c Conn) OpenStream(path string, params url.Values) (*http.Response, error) {
	return OpenStream(c.Client, c.Base, c.Token, path, params)
}
