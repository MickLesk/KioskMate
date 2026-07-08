package mqttclient

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"sync"
	"time"
)

type Client struct {
	URL      string
	ClientID string
	Username string
	Password string
	Version  string
	Timeout  time.Duration

	mu   sync.Mutex
	conn net.Conn
	next uint16
}

func (c *Client) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 6 * time.Second
}

func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connectLocked()
}

func (c *Client) connectLocked() error {
	if c.conn != nil {
		return nil
	}
	u, err := url.Parse(c.URL)
	if err != nil {
		return err
	}
	host := u.Host
	if host == "" {
		return errors.New("mqtt url missing host")
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		if u.Scheme == "mqtts" {
			host = net.JoinHostPort(host, "8883")
		} else {
			host = net.JoinHostPort(host, "1883")
		}
	}
	var conn net.Conn
	dialer := net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	if u.Scheme == "mqtts" {
		conn, err = tls.DialWithDialer(&dialer, "tcp", host, &tls.Config{ServerName: u.Hostname(), MinVersion: tls.VersionTLS12})
	} else {
		conn, err = dialer.Dial("tcp", host)
	}
	if err != nil {
		return err
	}
	c.conn = conn
	if err := conn.SetDeadline(time.Now().Add(c.timeout())); err != nil {
		_ = c.closeLocked()
		return err
	}
	if err := writePacket(conn, packetConnect, 0, connectPayload(c.ClientID, c.Username, c.Password, 30, c.Version)); err != nil {
		_ = c.closeLocked()
		return err
	}
	packet, payload, err := readPacket(conn)
	if err != nil {
		_ = c.closeLocked()
		return err
	}
	_ = conn.SetDeadline(time.Time{})
	if packet != packetConnAck || len(payload) < 2 || payload[1] != 0 {
		_ = c.closeLocked()
		return fmt.Errorf("mqtt connack failed: packet=%d payload=%v", packet, payload)
	}
	return nil
}

func (c *Client) Publish(topic string, payload []byte, retained bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.connectLocked(); err != nil {
		return err
	}
	flags := byte(0)
	if retained {
		flags |= 1
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(c.timeout()))
	if err := writePacket(c.conn, packetPublish, flags, publishPayload(topic, payload, c.Version)); err != nil {
		_ = c.closeLocked()
		return err
	}
	_ = c.conn.SetWriteDeadline(time.Time{})
	return nil
}

func (c *Client) Ping() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.connectLocked(); err != nil {
		return err
	}
	_ = c.conn.SetDeadline(time.Now().Add(c.timeout()))
	if err := writePacket(c.conn, packetPingReq, 0, nil); err != nil {
		_ = c.closeLocked()
		return err
	}
	packet, _, err := readPacket(c.conn)
	_ = c.conn.SetDeadline(time.Time{})
	if err != nil && !errors.Is(err, io.EOF) {
		_ = c.closeLocked()
		return err
	}
	if packet != packetPingResp {
		return fmt.Errorf("unexpected mqtt ping response: %d", packet)
	}
	return nil
}

func (c *Client) Subscribe(topics []string, handler func(topic string, payload []byte)) error {
	c.mu.Lock()
	if err := c.connectLocked(); err != nil {
		c.mu.Unlock()
		return err
	}
	c.next++
	if c.next == 0 {
		c.next = 1
	}
	id := c.next
	_ = c.conn.SetWriteDeadline(time.Now().Add(c.timeout()))
	if err := writePacket(c.conn, packetSubscribe, 2, subscribePayload(id, topics, c.Version)); err != nil {
		_ = c.closeLocked()
		c.mu.Unlock()
		return err
	}
	_ = c.conn.SetWriteDeadline(time.Time{})
	c.mu.Unlock()
	for {
		packet, payload, err := readPacket(c.conn)
		if err != nil {
			_ = c.Close()
			return err
		}
		switch packet {
		case packetPublish:
			topic, body, err := parsePublish(payload, c.Version)
			if err == nil {
				handler(topic, body)
			}
		case packetSubAck, packetPingResp:
		}
	}
}

func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeLocked()
}

func (c *Client) closeLocked() error {
	if c.conn == nil {
		return nil
	}
	_ = writePacket(c.conn, packetDisconnect, 0, nil)
	err := c.conn.Close()
	c.conn = nil
	return err
}
