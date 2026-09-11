package config

import (
	"encoding/json"
	"testing"
)

func FuzzConfigImport(f *testing.F) {
	f.Add([]byte(`{"version":4,"admin":{"port":33333},"kiosk":{"pages":[{"name":"Main","url":"https://example.test"}]}}`))
	f.Add([]byte(`{"version":2,"kiosk":{"urls":["http://homeassistant.local:8123"]}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var cfg Config
		if json.Unmarshal(data, &cfg) != nil {
			return
		}
		normalize(&cfg)
		_ = Validate(&cfg)
	})
}
