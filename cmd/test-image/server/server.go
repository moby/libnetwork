package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/sirupsen/logrus"
)

var basePaths = map[string]HTTPHandlerFunc{
	"/":      notImplemented,
	"/ready": ready,
}

type HTTPHandlerFunc func(s *TestServer, w http.ResponseWriter, r *http.Request)

type httpHandler struct {
	server *TestServer
	F      HTTPHandlerFunc
}

func (h httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.F(h.server, w, r)
}

type TestServer struct {
	srv               *http.Server
	port              int
	mux               *http.ServeMux
	registeredHanders map[string]bool
	sync.Mutex
}

// New creates a new diagnostic server
func NewTestServer() *TestServer {
	return &TestServer{
		registeredHanders: make(map[string]bool),
	}
}

// Init initialize the mux for the http handling and register the base hooks
func (s *TestServer) Init() {
	s.mux = http.NewServeMux()

	// Register local handlers
	s.RegisterHandler(basePaths)
}

// RegisterHandler allows to register new handlers to the mux and to a specific path
func (s *TestServer) RegisterHandler(hdlrs map[string]HTTPHandlerFunc) {
	s.Lock()
	defer s.Unlock()
	for path, fun := range hdlrs {
		if _, ok := s.registeredHanders[path]; ok {
			continue
		}
		s.mux.Handle(path, httpHandler{s, fun})
		s.registeredHanders[path] = true
	}
}

func (s *TestServer) Start(ip string, port int) {
	s.Lock()
	defer s.Unlock()

	s.port = port

	logrus.Infof("Starting the diagnostic server listening on %d for commands", port)
	srv := &http.Server{Addr: fmt.Sprintf("%s:%d", ip, port), Handler: s}
	s.srv = srv
	go func(n *TestServer) {
		// Ingore ErrServerClosed that is returned on the Shutdown call
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logrus.Fatalf("ListenAndServe error: %s", err)
		}
	}(s)
}

func (s *TestServer) Stop() {
	s.Lock()
	defer s.Unlock()

	s.srv.Shutdown(context.Background())
	s.srv = nil
	logrus.Info("Disabling the diagnostic server")
}

// ServeHTTP this is the method called by the ListenAndServe, and is needed to allow us to
// use our custom mux
func (s *TestServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func notImplemented(s *TestServer, w http.ResponseWriter, r *http.Request) {
	logrus.Info("command not implemented")
	w.WriteHeader(404)
}

func ready(s *TestServer, w http.ResponseWriter, r *http.Request) {
	logrus.Info("ready")
	w.WriteHeader(200)
}
