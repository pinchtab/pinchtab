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

func (c Conn) request(method, path string, opts ...RequestOption) request {
	return newRequest(method, c.Base, path, opts...)
}

func (c Conn) Get(path string, params url.Values, opts ...RequestOption) map[string]any {
	r := c.request(http.MethodGet, path, prepend(WithQuery(params), opts)...)
	return render(r, execute(c.Client, c.Token, r))
}

func (c Conn) Post(path string, body map[string]any, opts ...RequestOption) map[string]any {
	r := c.request(http.MethodPost, path, prepend(WithBody(body), opts)...)
	return render(r, execute(c.Client, c.Token, r))
}

func (c Conn) Raw(method, path string, opts ...RequestOption) ([]byte, error) {
	return executeE(c.Client, c.Token, c.request(method, path, opts...))
}

func (c Conn) WithClient(client *http.Client) Conn {
	c.Client = client
	return c
}

func (c Conn) GetRaw(path string, params url.Values, opts ...RequestOption) []byte {
	return execute(c.Client, c.Token, c.request(http.MethodGet, path, prepend(WithQuery(params), opts)...))
}

func (c Conn) PostQuiet(path string, body map[string]any, opts ...RequestOption) map[string]any {
	return c.Post(path, body, prepend(Quiet(), opts)...)
}

func (c Conn) PostQuietWithStatus(path string, body map[string]any, opts ...RequestOption) (int, []byte, map[string]any) {
	status, respBody := mustRequest(c.Client, c.Token, c.request(http.MethodPost, path, prepend(WithBody(body), opts)...))
	var result map[string]any
	if status < http.StatusBadRequest {
		result = decodeObject(respBody)
	}
	return status, respBody, result
}

func (c Conn) PostRaw(path string, body map[string]any, opts ...RequestOption) []byte {
	return execute(c.Client, c.Token, c.request(http.MethodPost, path, prepend(WithBody(body), opts)...))
}

func (c Conn) Delete(path string, params url.Values, opts ...RequestOption) map[string]any {
	r := c.request(http.MethodDelete, path, prepend(WithQuery(params), opts)...)
	return render(r, execute(c.Client, c.Token, r))
}

func (c Conn) OpenStream(path string, params url.Values) (*http.Response, error) {
	return OpenStream(c.Client, c.Base, c.Token, path, params)
}
