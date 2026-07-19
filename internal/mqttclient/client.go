package mqttclient

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Client struct {
	URL         string
	ClientID    string
	Username    string
	Password    string
	Version     string
	Timeout     time.Duration
	KeepAlive   time.Duration
	WillTopic   string
	WillPayload []byte
	WillRetain  bool

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

func (c *Client) keepAlive() uint16 {
	if c.KeepAlive <= 0 {
		return 30
	}
	seconds := int(c.KeepAlive.Seconds())
	if seconds <= 0 {
		return 30
	}
	if seconds > 65535 {
		return 65535
	}
	return uint16(seconds)
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
	var will *Will
	if strings.TrimSpace(c.WillTopic) != "" {
		will = &Will{Topic: c.WillTopic, Payload: c.WillPayload, Retain: c.WillRetain}
	}
	if err := writePacket(conn, packetConnect, 0, connectPayload(c.ClientID, c.Username, c.Password, c.keepAlive(), c.Version, will)); err != nil {
		_ = c.closeLocked()
		return err
	}
	packet, payload, err := readPacket(conn)
	if err != nil {
		_ = c.closeLocked()
		return err
	}
	_ = conn.SetDeadline(time.Time{})
	if err := connAckError(packet, payload, c.Version); err != nil {
		_ = c.closeLocked()
		return err
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
	conn := c.conn
	keepAlive := time.Duration(c.keepAlive()) * time.Second
	if keepAlive <= 0 {
		keepAlive = 30 * time.Second
	}
	c.mu.Unlock()
	for {
		_ = conn.SetReadDeadline(time.Now().Add(keepAlive / 2))
		packet, payload, err := readPacket(conn)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				c.mu.Lock()
				if c.conn == conn {
					_ = conn.SetWriteDeadline(time.Now().Add(c.timeout()))
					pingErr := writePacket(conn, packetPingReq, 0, nil)
					_ = conn.SetWriteDeadline(time.Time{})
					if pingErr != nil {
						_ = c.closeLocked()
						c.mu.Unlock()
						return pingErr
					}
				}
				c.mu.Unlock()
				continue
			}
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
