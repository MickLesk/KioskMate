package mqttclient

import "testing"

func TestConnectPayloadMQTT5IncludesProperties(t *testing.T) {
	payload := connectPayload("kioskmate", "", "", 30, "5.0", nil)
	if len(payload) < 11 {
		t.Fatalf("connect payload too short: %d", len(payload))
	}
	if payload[6] != 5 {
		t.Fatalf("protocol level = %d, want 5", payload[6])
	}
	if payload[10] != 0 {
		t.Fatalf("properties length = %d, want 0", payload[10])
	}
}

func TestPublishPayloadMQTT5SkipsPropertiesWhenParsing(t *testing.T) {
	payload := publishPayload("kioskmate/test", []byte("ok"), "5.0")
	topic, body, err := parsePublish(payload, "5.0")
	if err != nil {
		t.Fatal(err)
	}
	if topic != "kioskmate/test" {
		t.Fatalf("topic = %q", topic)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q", string(body))
	}
}

func TestSubscribePayloadMQTT5IncludesProperties(t *testing.T) {
	payload := subscribePayload(7, []string{"kioskmate/command"}, "5.0")
	if len(payload) < 3 {
		t.Fatalf("subscribe payload too short: %d", len(payload))
	}
	if payload[0] != 0 || payload[1] != 7 {
		t.Fatalf("packet id bytes = %v", payload[:2])
	}
	if payload[2] != 0 {
		t.Fatalf("properties length = %d, want 0", payload[2])
	}
}
