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

func TestConnectPayloadMQTT5WillIncludesPropertiesByte(t *testing.T) {
	will := &Will{Topic: "kioskmate/node/availability", Payload: []byte("offline"), Retain: true}
	payload := connectPayload("kioskmate_cmd", "", "", 30, "5.0", will)
	if payload[6] != 5 {
		t.Fatalf("protocol level = %d, want 5", payload[6])
	}
	if payload[7]&0x04 == 0 {
		t.Fatalf("will flag not set: %#x", payload[7])
	}
	// header: "MQTT"(2+4) + level(1) + flags(1) + keepalive(2) + conn properties length(1) = 11
	offset := 11
	clientIDLen := int(payload[offset])<<8 | int(payload[offset+1])
	offset += 2 + clientIDLen
	if payload[offset] != 0 {
		t.Fatalf("will properties length byte = %d, want 0", payload[offset])
	}
	offset++
	willTopicLen := int(payload[offset])<<8 | int(payload[offset+1])
	offset += 2
	willTopic := string(payload[offset : offset+willTopicLen])
	if willTopic != will.Topic {
		t.Fatalf("will topic = %q, want %q", willTopic, will.Topic)
	}
	offset += willTopicLen
	willPayloadLen := int(payload[offset])<<8 | int(payload[offset+1])
	offset += 2
	if willPayloadLen != len(will.Payload) {
		t.Fatalf("will payload length = %d, want %d", willPayloadLen, len(will.Payload))
	}
	if string(payload[offset:offset+willPayloadLen]) != string(will.Payload) {
		t.Fatalf("will payload = %q, want %q", string(payload[offset:offset+willPayloadLen]), string(will.Payload))
	}
}

func TestConnectPayloadMQTT311WillHasNoPropertiesByte(t *testing.T) {
	will := &Will{Topic: "kioskmate/node/availability", Payload: []byte("offline")}
	payload := connectPayload("kioskmate_cmd", "", "", 30, "3.1.1", will)
	if payload[6] != 4 {
		t.Fatalf("protocol level = %d, want 4", payload[6])
	}
	// header for v3.1.1: "MQTT"(2+4) + level(1) + flags(1) + keepalive(2) = 10 (no properties length byte)
	offset := 10
	clientIDLen := int(payload[offset])<<8 | int(payload[offset+1])
	offset += 2 + clientIDLen
	willTopicLen := int(payload[offset])<<8 | int(payload[offset+1])
	offset += 2
	willTopic := string(payload[offset : offset+willTopicLen])
	if willTopic != will.Topic {
		t.Fatalf("will topic = %q, want %q (unexpected properties byte before it)", willTopic, will.Topic)
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
