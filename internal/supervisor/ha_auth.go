package supervisor

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func checkHomeAssistantBan(ctx context.Context, target string) (bool, error) {
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false, fmt.Errorf("invalid Home Assistant URL")
	}
	parsed.Path = "/manifest.json"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	requestCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", "KioskMate Auth Safety Check")
	client := &http.Client{
		Timeout: 3 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || (len(via) > 0 && !strings.EqualFold(req.URL.Host, via[0].URL.Host)) {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusForbidden, nil
}
