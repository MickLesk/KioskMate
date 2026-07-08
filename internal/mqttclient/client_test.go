package mqttclient

import (
	"net"
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
