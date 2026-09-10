package admin

import "testing"

func TestRedactSecretsRemovesURLCredentialsAndQuery(t *testing.T) {
	result := redactSecrets(map[string]any{
		"page_url": "https://user:secret@example.test/dashboard?token=private#view",
		"urls":     []string{"https://example.test/one?access_token=private"},
	}).(map[string]any)
	if result["page_url"] != "https://example.test/dashboard" {
		t.Fatalf("page URL was not redacted: %#v", result["page_url"])
	}
	urls := result["urls"].([]any)
	if urls[0] != "https://example.test/one" {
		t.Fatalf("URL list was not redacted: %#v", urls)
	}
}
