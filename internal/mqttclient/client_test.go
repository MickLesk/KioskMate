package mqttclient

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestConnectTimesOutWhenBrokerDoesNotRespond(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(300 * time.Millisecond)
	}()

	client := &Client{
		URL:      "mqtt://" + listener.Addr().String(),
		ClientID: "timeout_test",
		Timeout:  40 * time.Millisecond,
	}
	start := time.Now()
	err = client.Connect()
	if err == nil {
		t.Fatal("Connect succeeded against a silent broker")
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("Connect timeout took %s", elapsed)
	}
	<-done
}

func TestTLSConfigRequiresCertificatePair(t *testing.T) {
	client := &Client{CertFile: "client.crt"}
	if _, err := client.tlsConfig("broker.local"); err == nil || !strings.Contains(err.Error(), "configured together") {
		t.Fatalf("unexpected TLS config error: %v", err)
	}
}

func TestConnectRejectsNonMQTTScheme(t *testing.T) {
	client := &Client{URL: "http://broker.local:1883"}
	if err := client.Connect(); err == nil || !strings.Contains(err.Error(), "unsupported MQTT URL scheme") {
		t.Fatalf("unexpected scheme error: %v", err)
	}
}

func TestPingTimesOutWithoutDeadlockWhenBrokerDoesNotRespond(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		time.Sleep(300 * time.Millisecond)
	}()

	client := &Client{
		URL:      "mqtt://" + listener.Addr().String(),
		ClientID: "timeout_ping_test",
		Timeout:  40 * time.Millisecond,
	}
	errc := make(chan error, 1)
	go func() {
		errc <- client.Ping()
	}()

	select {
	case err := <-errc:
		if err == nil {
			t.Fatal("Ping succeeded against a silent broker")
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("Ping deadlocked against a silent broker")
	}
	<-done
}
