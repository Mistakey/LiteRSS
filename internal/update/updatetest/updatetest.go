// Package updatetest is a fake GitHub release service for the in-app update:
// the releases API's latest release and its asset downloads, served from one
// base URL as update.APIEnv expects. tools/fake-release serves it to a
// development instance.
package updatetest

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Release is the latest release the fake serves.
type Release struct {
	// Tag is the release tag, such as v9.0.0.
	Tag string
	// Files are the assets by name. SHA256SUMS is generated from them unless
	// Files holds one or NoSums is set.
	Files map[string][]byte
	// NoSums leaves SHA256SUMS out.
	NoSums bool
	// Corrupt serves a different body for the named asset than the one
	// SHA256SUMS lists, as a damaged download would.
	Corrupt string
	// Rate slows downloads to this many bytes a second; 0 is unthrottled.
	Rate int
}

// Server is the fake. The zero Release answers 404: nothing released yet.
type Server struct {
	Repo string

	mu      sync.Mutex
	release Release
}

// New returns a fake for the owner/name repo.
func New(repo string) *Server {
	return &Server{Repo: repo}
}

// Publish replaces the latest release.
func (s *Server) Publish(r Release) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.release = r
}

// Sums returns the SHA256SUMS text for files, as sha256sum writes it.
func Sums(files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		sum := sha256.Sum256(files[name])
		fmt.Fprintf(&b, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	return b.String()
}

// files returns the served assets, SHA256SUMS included.
func (r Release) files() map[string][]byte {
	out := map[string][]byte{}
	for name, data := range r.Files {
		out[name] = data
	}
	if _, ok := out["SHA256SUMS"]; !ok && !r.NoSums && len(r.Files) > 0 {
		out["SHA256SUMS"] = []byte(Sums(r.Files))
	}
	return out
}

func (s *Server) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	s.mu.Lock()
	rel := s.release
	s.mu.Unlock()

	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	base := "http://" + req.Host
	download := "/" + s.Repo + "/releases/download/" + rel.Tag + "/"
	switch {
	case req.URL.Path == "/repos/"+s.Repo+"/releases/latest":
		if rel.Tag == "" {
			http.Error(w, `{"message": "Not Found"}`, http.StatusNotFound)
			return
		}
		type asset struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
			Size int    `json:"size"`
		}
		files := rel.files()
		assets := []asset{}
		for name, data := range files {
			assets = append(assets, asset{Name: name, URL: base + download + name, Size: len(data)})
		}
		sort.Slice(assets, func(i, j int) bool { return assets[i].Name < assets[j].Name })
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"tag_name": rel.Tag,
			"html_url": "https://github.com/" + s.Repo + "/releases/tag/" + rel.Tag,
			"assets":   assets,
		})
	case rel.Tag != "" && strings.HasPrefix(req.URL.Path, download):
		name := strings.TrimPrefix(req.URL.Path, download)
		data, ok := rel.files()[name]
		if !ok {
			http.NotFound(w, req)
			return
		}
		if name == rel.Corrupt && len(data) > 0 {
			data = append([]byte{}, data...)
			data[0] ^= 0xff
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprint(len(data)))
		serve(w, req, data, rel.Rate)
	default:
		http.NotFound(w, req)
	}
}

// serve writes data at rate bytes a second, in tenths of a second.
func serve(w http.ResponseWriter, req *http.Request, data []byte, rate int) {
	if rate <= 0 {
		w.Write(data)
		return
	}
	chunk := max(rate/10, 1)
	flusher, _ := w.(http.Flusher)
	for len(data) > 0 {
		n := min(chunk, len(data))
		if _, err := w.Write(data[:n]); err != nil {
			return
		}
		if flusher != nil {
			flusher.Flush()
		}
		data = data[n:]
		select {
		case <-req.Context().Done():
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}
