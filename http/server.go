package http

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rebeljah/picast/media"
)

// type MiddlewareHandler struct {
// 	h http.Handler
// 	next http.Handler
// }

// func NewMiddleWare(h http.Handler, next http.Handler) MiddlewareHandler {
// 	return MiddlewareHandler{h: h, next: next}
// }

// func (m *MiddlewareHandler) ServeHTTP(rw ResponseWriter, r *Request) {
// 	m.h.ServeHTTP(rw, r)
// 	next.ServeHTTP(rw, r)
// }

type Server struct {
	http.Server
	mediaManifest media.Manifest
	interruptOnce sync.Once
}

func NewServer(manifest media.Manifest) *Server {
	return &Server{
		Server:        http.Server{},
		mediaManifest: manifest,
	}

}

// return a manifest of metadata for all current media on the server.
// if an id is passed (e.g example.com/manifest/3247g2387g), only responds
// with metadata for that media. Response will be JSON and can be unmarshalled
// into a picast/media MediaMetadata struct(s). The returned body is either a
// JSONified map[media.UID]]media.MetaData or media.MetaData
func (s *Server) handleGetManifest(rw http.ResponseWriter, r *http.Request) {
	path := strings.Trim(r.URL.Path, "/")
	segments := strings.Split(path, "/")

	var buf []byte
	var err error

	switch len(segments) {
	case 1: // "/manifest" → return entire manifest
		buf, err = s.mediaManifest.JSON()
		if err != nil {
			http.Error(rw, "Failed to encode manifest", http.StatusInternalServerError)
			return
		}

	case 2: // "/manifest/{id}" → return specific media metadata
		id := media.UID(r.PathValue("id"))
		metadata, ok := s.mediaManifest.Get(id)
		if !ok {
			http.Error(rw, "Media not found", http.StatusNotFound)
			return
		}

		buf, err = json.Marshal(metadata)
		if err != nil {
			http.Error(rw, "Failed to encode media entry", http.StatusInternalServerError)
			return
		}

	default: // invalid path
		http.Error(rw, "Not Found", http.StatusNotFound)
		return
	}

	rw.Header().Set("Content-Type", "application/json")
	rw.Write(buf)
}

func (s *Server) ListenAndServe(addr string) error {
	log.Println("Starting HTTP server on :" + addr)

	http.HandleFunc("GET /manifest", s.handleGetManifest)
	http.HandleFunc("GET /manifest/", s.handleGetManifest)
	http.HandleFunc("GET /manifest/{id}", s.handleGetManifest)
	http.HandleFunc("GET /manifest/{id}/", s.handleGetManifest)

	s.Addr = addr

	return s.Server.ListenAndServe()
}

func (s *Server) Interrupt(err error) {
	s.interruptOnce.Do(func() {
		log.Printf("Interrupting HTTP server: %v\n", err)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := s.Server.Shutdown(ctx); err != nil {
			s.Server.Close()
		}

		log.Println("HTTP server shutdown complete")
	})
}
