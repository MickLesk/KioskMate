package mqttclient

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type capturedPacket struct {
	header  byte
	payload []byte
}

func readRawPacket(conn net.Conn) (capturedPacket, error) {
	var header [1]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return capturedPacket{}, err
	}
	remaining, err := decodeRemainingLength(conn)
	if err != nil {
		return capturedPacket{}, err
	}
	payload := make([]byte, remaining)
	_, err = io.ReadFull(conn, payload)
	return capturedPacket{header: header[0], payload: payload}, err
}

func TestBrokerDialogueAndRetainedCleanupForMQTT311And5(t *testing.T) {
	for _, version := range []string{"3.1.1", "5.0"} {
		t.Run(version, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			captured := make(chan capturedPacket, 1)
			errc := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					errc <- err
					return
				}
				defer conn.Close()
				connect, err := readRawPacket(conn)
				if err != nil {
					errc <- err
					return
				}
				if connect.header>>4 != packetConnect {
					errc <- io.ErrUnexpectedEOF
					return
				}
				if version == "5.0" {
					_, err = conn.Write([]byte{0x20, 0x03, 0x00, 0x00, 0x00})
				} else {
					_, err = conn.Write([]byte{0x20, 0x02, 0x00, 0x00})
				}
				if err != nil {
					errc <- err
					return
				}
				publish, err := readRawPacket(conn)
				if err != nil {
					errc <- err
					return
				}
				captured <- publish
			}()

			client := &Client{URL: "mqtt://" + listener.Addr().String(), ClientID: "fixture", Version: version, Timeout: time.Second}
			if err := client.Connect(); err != nil {
				t.Fatal(err)
			}
			if err := client.Publish("homeassistant/sensor/old/config", nil, true); err != nil {
				t.Fatal(err)
			}
			select {
			case packet := <-captured:
				if packet.header>>4 != packetPublish || packet.header&0x01 == 0 {
					t.Fatalf("publish header = 0x%02x; want retained PUBLISH", packet.header)
				}
				topic, body, err := parsePublish(packet.payload, version)
				if err != nil || topic != "homeassistant/sensor/old/config" || len(body) != 0 {
					t.Fatalf("cleanup publish topic=%q body=%q err=%v", topic, body, err)
				}
			case err := <-errc:
				t.Fatal(err)
			case <-time.After(2 * time.Second):
				t.Fatal("broker did not receive retained cleanup publish")
			}
			_ = client.Close()
		})
	}
}

func TestMQTTTLSValidatesConfiguredCAAndServerName(t *testing.T) {
	certificate, caFile := mqttTestCertificate(t)
	start := func() (net.Listener, <-chan error) {
		listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() {
			conn, err := listener.Accept()
			if err != nil {
				done <- err
				return
			}
			defer conn.Close()
			if _, err := readRawPacket(conn); err != nil {
				done <- err
				return
			}
			_, err = conn.Write([]byte{0x20, 0x03, 0x00, 0x00, 0x00})
			done <- err
		}()
		return listener, done
	}

	listener, done := start()
	client := &Client{URL: "mqtts://" + listener.Addr().String(), ClientID: "tls-fixture", Version: "5.0", CAFile: caFile, ServerName: "localhost", Timeout: time.Second}
	if err := client.Connect(); err != nil {
		t.Fatalf("trusted TLS broker connection failed: %v", err)
	}
	_ = client.Close()
	_ = listener.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}

	badListener, badDone := start()
	badClient := &Client{URL: "mqtts://" + badListener.Addr().String(), ClientID: "tls-bad-name", Version: "5.0", CAFile: caFile, ServerName: "wrong.invalid", Timeout: time.Second}
	if err := badClient.Connect(); err == nil {
		t.Fatal("TLS connection unexpectedly accepted a mismatched server name")
	}
	_ = badListener.Close()
	<-badDone
}

func mqttTestCertificate(t *testing.T) (tls.Certificate, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:    []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certificate, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(path, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	return certificate, path
}
