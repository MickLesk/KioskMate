package mqttclient

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

const (
	packetConnect    = 1
	packetConnAck    = 2
	packetPublish    = 3
	packetSubscribe  = 8
	packetSubAck     = 9
	packetPingReq    = 12
	packetPingResp   = 13
	packetDisconnect = 14
)

func writePacket(w io.Writer, packetType byte, flags byte, payload []byte) error {
	header := []byte{packetType<<4 | flags}
	header = append(header, encodeRemainingLength(len(payload))...)
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

func readPacket(r io.Reader) (byte, []byte, error) {
	var first [1]byte
	if _, err := io.ReadFull(r, first[:]); err != nil {
		return 0, nil, err
	}
	remaining, err := decodeRemainingLength(r)
	if err != nil {
		return 0, nil, err
	}
	payload := make([]byte, remaining)
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return first[0] >> 4, payload, nil
}

func encodeRemainingLength(length int) []byte {
	var out []byte
	for {
		encoded := byte(length % 128)
		length /= 128
		if length > 0 {
			encoded |= 128
		}
		out = append(out, encoded)
		if length == 0 {
			return out
		}
	}
}

func decodeRemainingLength(r io.Reader) (int, error) {
	multiplier := 1
	value := 0
	for i := 0; i < 4; i++ {
		var b [1]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return 0, err
		}
		value += int(b[0]&127) * multiplier
		if b[0]&128 == 0 {
			return value, nil
		}
		multiplier *= 128
	}
	return 0, errors.New("malformed remaining length")
}

func mqttString(value string) []byte {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, uint16(len(value)))
	buf.WriteString(value)
	return buf.Bytes()
}

func connectPayload(clientID, username, password string, keepAlive uint16) []byte {
	var buf bytes.Buffer
	buf.Write(mqttString("MQTT"))
	buf.WriteByte(4)
	flags := byte(0x02)
	if username != "" {
		flags |= 0x80
	}
	if password != "" {
		flags |= 0x40
	}
	buf.WriteByte(flags)
	_ = binary.Write(&buf, binary.BigEndian, keepAlive)
	buf.Write(mqttString(clientID))
	if username != "" {
		buf.Write(mqttString(username))
	}
	if password != "" {
		buf.Write(mqttString(password))
	}
	return buf.Bytes()
}

func publishPayload(topic string, payload []byte) []byte {
	var buf bytes.Buffer
	buf.Write(mqttString(topic))
	buf.Write(payload)
	return buf.Bytes()
}

func subscribePayload(id uint16, topics []string) []byte {
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, id)
	for _, topic := range topics {
		buf.Write(mqttString(topic))
		buf.WriteByte(0)
	}
	return buf.Bytes()
}

func parsePublish(payload []byte) (string, []byte, error) {
	if len(payload) < 2 {
		return "", nil, errors.New("short publish")
	}
	size := int(binary.BigEndian.Uint16(payload[:2]))
	if len(payload) < 2+size {
		return "", nil, errors.New("short publish topic")
	}
	return string(payload[2 : 2+size]), payload[2+size:], nil
}
