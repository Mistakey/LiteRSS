// Package browser hands links to the system browser. Link targets come from
// feed content, so only absolute http(s) URLs are passed on; anything else
// could launch a local file or a registered protocol handler.
package browser

import (
	"errors"
	"net/url"
	"strings"
)

// ErrUnsupportedURL is returned for links that are not absolute http(s) URLs.
var ErrUnsupportedURL = errors.New("only absolute http(s) links can be opened in the browser")

// Opener opens a validated link. The shell supplies the platform call.
type Opener struct {
	open func(string) error
}

// New wraps the platform call that opens a URL in the system browser.
func New(open func(string) error) Opener {
	return Opener{open: open}
}

// Open validates raw and passes its normalized form to the system browser.
func (o Opener) Open(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ErrUnsupportedURL
	}
	return o.open(u.String())
}
