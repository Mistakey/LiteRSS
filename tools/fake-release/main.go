// Command fake-release serves internal/update/updatetest on a loopback port
// with a generated newer release, for a development instance started with
// LITERSS_UPDATE_API to walk the in-app update against. Its installers are
// random bytes: a development build never starts them. Usage is in
// docs/TESTING.md.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"

	"LiteRSS/internal/identity"
	"LiteRSS/internal/update"
	"LiteRSS/internal/update/updatetest"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:1241", "loopback address to listen on")
	ver := flag.String("version", "99.0.0", "version of the released update")
	size := flag.Int("size", 8<<20, "bytes of each generated installer")
	rate := flag.Int("rate", 1<<20, "download speed in bytes a second; 0 is unthrottled")
	corrupt := flag.Bool("corrupt", false, "serve installers that do not match SHA256SUMS")
	noSums := flag.Bool("no-sums", false, "leave SHA256SUMS out of the release")
	flag.Parse()

	if err := checkLoopback(*addr); err != nil {
		log.Fatal(err)
	}

	repo := identity.Current().UpdateRepo
	fake := updatetest.New(repo)
	opts := options{version: *ver, size: *size, rate: *rate, corrupt: *corrupt, noSums: *noSums}
	fake.Publish(opts.release())

	mux := http.NewServeMux()
	mux.Handle("/", fake)
	// Change the release without a restart:
	// POST /_fake/release?corrupt=1&nosums=0&rate=1048576
	mux.HandleFunc("POST /_fake/release", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		o := opts
		if v := q.Get("corrupt"); v != "" {
			o.corrupt = v == "1"
		}
		if v := q.Get("nosums"); v != "" {
			o.noSums = v == "1"
		}
		if v := q.Get("rate"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				http.Error(w, "rate must be a number", http.StatusBadRequest)
				return
			}
			o.rate = n
		}
		fake.Publish(o.release())
		fmt.Fprintf(w, "release v%s: corrupt=%v nosums=%v rate=%d\n", o.version, o.corrupt, o.noSums, o.rate)
	})

	log.Printf("fake release service on http://%s for %s: v%s, %d-byte installers at %d B/s, corrupt=%v, no-sums=%v",
		*addr, repo, *ver, *size, *rate, *corrupt, *noSums)
	log.Printf("start the development instance with %s=http://%s", update.APIEnv, *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

type options struct {
	version         string
	size, rate      int
	corrupt, noSums bool
}

// release builds the release: every platform's installer, the same random
// bytes for a given size.
func (o options) release() updatetest.Release {
	rng := rand.New(rand.NewPCG(1, uint64(o.size)))
	body := make([]byte, o.size)
	for i := range body {
		body[i] = byte(rng.UintN(256))
	}
	files := map[string][]byte{}
	for _, p := range [][2]string{{"windows", "amd64"}, {"windows", "arm64"}, {"darwin", "universal"}} {
		files[update.InstallerNames(o.version, p[0], p[1])[0]] = body
	}
	r := updatetest.Release{Tag: "v" + o.version, Files: files, NoSums: o.noSums, Rate: o.rate}
	if o.corrupt {
		r.Corrupt = update.InstallerNames(o.version, "windows", "amd64")[0]
	}
	return r
}

func checkLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return errors.New("-addr must be a loopback IP such as 127.0.0.1")
	}
	return nil
}
