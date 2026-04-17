package actions

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
)

type capturedRequest struct {
	Method  string
	Path    string
	Query   string
	Body    string
	Headers http.Header
}

type mockServer struct {
	server      *httptest.Server
	lastMethod  string
	lastPath    string
	lastQuery   string
	lastBody    string
	lastHeaders http.Header
	requests    []capturedRequest
	response    string
	statusCode  int
}

func newMockServer() *mockServer {
	m := &mockServer{statusCode: 200, response: `{"status":"ok"}`}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.lastMethod = r.Method
		m.lastPath = r.URL.Path
		m.lastQuery = r.URL.RawQuery
		m.lastHeaders = r.Header
		if r.Body != nil {
			body, _ := io.ReadAll(r.Body)
			m.lastBody = string(body)
		}
		m.requests = append(m.requests, capturedRequest{
			Method:  m.lastMethod,
			Path:    m.lastPath,
			Query:   m.lastQuery,
			Body:    m.lastBody,
			Headers: m.lastHeaders.Clone(),
		})
		w.WriteHeader(m.statusCode)
		_, _ = w.Write([]byte(m.response))
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	srv := &httptest.Server{
		Listener: listener,
		Config:   &http.Server{Handler: handler},
	}
	srv.Start()
	m.server = srv
	return m
}

func (m *mockServer) close()       { m.server.Close() }
func (m *mockServer) base() string { return m.server.URL }
