package mqttclient

import "testing"

func FuzzMQTTPacketParsers(f *testing.F) {
	f.Add([]byte{0, 4, 't', 'e', 's', 't', 'o', 'k'}, "3.1.1", byte(packetConnAck))
	f.Add([]byte{0, 0x87, 0}, "5.0", byte(packetConnAck))
	f.Fuzz(func(t *testing.T, payload []byte, version string, packet byte) {
		_, _, _ = parsePublish(payload, version)
		_ = connAckError(packet, payload, version)
	})
}
