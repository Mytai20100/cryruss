package api

import (
	"net"
	"net/http"
	"strings"

	cryruss "github.com/cryruss/cryruss"
	"github.com/cryruss/cryruss/pkg/api/handler"
	"github.com/cryruss/cryruss/pkg/config"
)

type Server struct {
	mux      *http.ServeMux
	handlers *handler.Handlers
}

func NewServer() *Server {
	s := &Server{
		mux:      http.NewServeMux(),
		handlers: handler.New(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// System
	s.handle("GET /_ping", s.handlers.Ping)
	s.handle("GET /version", s.handlers.Version)
	s.handle("GET /info", s.handlers.Info)

	// Containers
	s.handle("GET /containers/json", s.handlers.ContainerList)
	s.handle("POST /containers/create", s.handlers.ContainerCreate)
	s.handle("GET /containers/{id}/json", s.handlers.ContainerInspect)
	s.handle("POST /containers/{id}/start", s.handlers.ContainerStart)
	s.handle("POST /containers/{id}/stop", s.handlers.ContainerStop)
	s.handle("POST /containers/{id}/restart", s.handlers.ContainerRestart)
	s.handle("POST /containers/{id}/kill", s.handlers.ContainerKill)
	s.handle("DELETE /containers/{id}", s.handlers.ContainerRemove)
	s.handle("GET /containers/{id}/logs", s.handlers.ContainerLogs)
	s.handle("POST /containers/{id}/exec", s.handlers.ContainerExec)
	s.handle("GET /containers/{id}/stats", s.handlers.ContainerStats)
	s.handle("GET /containers/{id}/top", s.handlers.ContainerTop)
	s.handle("POST /containers/prune", s.handlers.ContainerPrune)
	s.handle("POST /containers/{id}/rename", s.handlers.ContainerRename)
	s.handle("POST /containers/{id}/pause", s.handlers.ContainerPause)
	s.handle("POST /containers/{id}/unpause", s.handlers.ContainerUnpause)
	s.handle("GET /containers/{id}/changes", s.handlers.ContainerChanges)

	// Images
	s.handle("GET /images/json", s.handlers.ImageList)
	s.handle("POST /images/create", s.handlers.ImageCreate)
	s.handle("GET /images/{name}/json", s.handlers.ImageInspect)
	s.handle("DELETE /images/{name}", s.handlers.ImageRemove)
	s.handle("POST /images/{name}/tag", s.handlers.ImageTag)
	s.handle("POST /images/prune", s.handlers.ImagePrune)
	s.handle("GET /images/search", s.handlers.ImageSearch)

	// Networks
	s.handle("GET /networks", s.handlers.NetworkList)
	s.handle("POST /networks/create", s.handlers.NetworkCreate)
	s.handle("GET /networks/{id}", s.handlers.NetworkInspect)
	s.handle("DELETE /networks/{id}", s.handlers.NetworkRemove)
	s.handle("POST /networks/prune", s.handlers.NetworkPrune)
	s.handle("POST /networks/{id}/connect", s.handlers.NetworkConnect)
	s.handle("POST /networks/{id}/disconnect", s.handlers.NetworkDisconnect)

	// Volumes
	s.handle("GET /volumes", s.handlers.VolumeList)
	s.handle("POST /volumes/create", s.handlers.VolumeCreate)
	s.handle("GET /volumes/{name}", s.handlers.VolumeInspect)
	s.handle("DELETE /volumes/{name}", s.handlers.VolumeRemove)
	s.handle("POST /volumes/prune", s.handlers.VolumePrune)
}

// handle registers a route with versioned and bare paths.
// pattern must be in the form "METHOD /path" (e.g. "GET /networks/{id}").
// The method is kept in the registered pattern so that routes sharing the
// same path template but different methods (e.g. GET vs DELETE) do not
// collide in the ServeMux.
func (s *Server) handle(pattern string, fn http.HandlerFunc) {
	parts := strings.SplitN(pattern, " ", 2)
	method, path := parts[0], parts[1]
	_ = method // method is already part of pattern; kept for clarity

	// Register bare path — include the method to avoid duplicate-path panics
	s.mux.HandleFunc(pattern, fn)
	// Register versioned paths /v1.x/...
	for _, v := range []string{"v1.24", "v1.40", "v1.41", "v1.42", "v1.43", "v1.44", "v1.45"} {
		s.mux.HandleFunc(method+" /"+v+path, fn)
	}
}

func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("unix", config.Global.SocketPath)
	if err != nil {
		return err
	}
	defer ln.Close()
	return http.Serve(ln, s.versionMiddleware(s.mux))
}

func (s *Server) versionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "cryruss/"+cryruss.Version)
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}
