package translation

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
)

// BaiduEndpoint is the Baidu general translation API.
const BaiduEndpoint = "https://fanyi-api.baidu.com/api/trans/vip/translate"

// BaiduMaxQueryBytes bounds one request's q; Baidu refuses longer queries
// (its documented limit is 6000 bytes), so callers split larger batches.
const BaiduMaxQueryBytes = 5000

// Baidu translates titles into Chinese through the Baidu Translate API, the
// only title translator (spec D11).
type Baidu struct {
	AppID     string
	SecretKey string
	// Endpoint is BaiduEndpoint unless a test points it elsewhere.
	Endpoint string
	// Client carries the request; the caller passes the shared outbound
	// client (pitfall 8).
	Client *http.Client
}

// BaiduError is an error code Baidu answered with.
type BaiduError struct {
	Code string
	Msg  string
}

func (e *BaiduError) Error() string {
	return fmt.Sprintf("baidu api error %s: %s", e.Code, e.Msg)
}

// TranslateLines translates each line into Simplified Chinese in one
// request, answering the translations in order. Lines must be non-empty and
// hold no line breaks: Baidu translates q line by line.
func (t *Baidu) TranslateLines(ctx context.Context, lines []string) ([]string, error) {
	if len(lines) == 0 {
		return nil, nil
	}
	q := strings.Join(lines, "\n")

	n, err := rand.Int(rand.Reader, big.NewInt(1000000000))
	if err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	salt := n.String()
	// MD5 is what the Baidu API signs requests with: md5(appid+q+salt+key).
	sum := md5.Sum([]byte(t.AppID + q + salt + t.SecretKey))

	form := url.Values{}
	form.Set("q", q)
	form.Set("from", "auto")
	form.Set("to", "zh")
	form.Set("appid", t.AppID)
	form.Set("salt", salt)
	form.Set("sign", hex.EncodeToString(sum[:]))

	endpoint := t.Endpoint
	if endpoint == "" {
		endpoint = BaiduEndpoint
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("baidu api request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("baidu api returned status %d", resp.StatusCode)
	}

	var result struct {
		ErrorCode   string `json:"error_code"`
		ErrorMsg    string `json:"error_msg"`
		TransResult []struct {
			Dst string `json:"dst"`
		} `json:"trans_result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode baidu response: %w", err)
	}
	if result.ErrorCode != "" && result.ErrorCode != "52000" {
		return nil, &BaiduError{Code: result.ErrorCode, Msg: result.ErrorMsg}
	}
	if len(result.TransResult) != len(lines) {
		return nil, fmt.Errorf("baidu answered %d translations for %d lines", len(result.TransResult), len(lines))
	}
	out := make([]string, len(lines))
	for i, r := range result.TransResult {
		out[i] = r.Dst
	}
	return out, nil
}
