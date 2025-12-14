package i2plib

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type mockContract struct {
	MockSAM struct {
		AcceptPayloadASCII string `json:"accept_payload_ascii"`
	} `json:"mock_sam"`
}

func loadMockContract(t *testing.T) mockContract {
	t.Helper()
	path := filepath.Join("testdata", "contract.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var c mockContract
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	return c
}

type mockSAMServer struct {
	ln net.Listener
	// inputs
	acceptDestB64 string
	acceptPayload []byte
}

func startMockSAMServer(t *testing.T) *mockSAMServer {
	t.Helper()
	c := loadMockContract(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("mock SAM server unavailable in sandbox (listen failed): %v", err)
	}

	// Create a deterministic "private key" blob that satisfies DestinationFromBase64(..., hasPriv=true).
	// It must be at least 387 bytes, and include a valid certificate length at offsets 385..386.
	priv := make([]byte, 387)
	priv[385] = 0
	priv[386] = 0

	s := &mockSAMServer{
		ln:            ln,
		acceptDestB64: I2PBase64Encode(priv),
		acceptPayload: []byte(c.MockSAM.AcceptPayloadASCII),
	}
	go s.serve()
	return s
}

func (s *mockSAMServer) addr() Address {
	host, portStr, _ := net.SplitHostPort(s.ln.Addr().String())
	port, _ := strconv.Atoi(portStr)
	return Address{Host: host, Port: port}
}

func (s *mockSAMServer) close() {
	_ = s.ln.Close()
}

func (s *mockSAMServer) serve() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handleConn(c)
	}
}

func (s *mockSAMServer) handleConn(c net.Conn) {
	defer func() {
		_ = c.Close()
	}()

	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)

	// HELLO
	line, err := r.ReadString('\n')
	if err != nil {
		return
	}
	if !strings.HasPrefix(line, "HELLO VERSION") {
		return
	}
	_, _ = w.WriteString("HELLO REPLY RESULT=OK VERSION=3.1\n")
	_ = w.Flush()

	// Next command
	cmd, err := r.ReadString('\n')
	if err != nil {
		return
	}

	switch {
	case strings.HasPrefix(cmd, "DEST GENERATE"):
		// Return a deterministic private key blob; for testing we reuse acceptDestB64.
		_, _ = w.WriteString("DEST REPLY RESULT=OK PRIV=" + s.acceptDestB64 + "\n")
		_ = w.Flush()
		return

	case strings.HasPrefix(cmd, "SESSION CREATE"):
		// When DESTINATION=TRANSIENT, Python expects DESTINATION back.
		_, _ = w.WriteString("SESSION STATUS RESULT=OK DESTINATION=" + s.acceptDestB64 + "\n")
		_ = w.Flush()
		// Keep connection open to mimic Python session context manager.
		buf := make([]byte, 1024)
		for {
			if _, err := r.Read(buf); err != nil {
				return
			}
		}

	case strings.HasPrefix(cmd, "NAMING LOOKUP"):
		_, _ = w.WriteString("NAMING REPLY RESULT=OK VALUE=" + s.acceptDestB64 + "\n")
		_ = w.Flush()
		return

	case strings.HasPrefix(cmd, "STREAM CONNECT"):
		_, _ = w.WriteString("STREAM STATUS RESULT=OK\n")
		_ = w.Flush()
		// Echo mode: whatever client writes, echo back.
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				_, _ = c.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}

	case strings.HasPrefix(cmd, "STREAM ACCEPT"):
		_, _ = w.WriteString("STREAM STATUS RESULT=OK\n")
		_ = w.Flush()
		// Send dest line + payload (in a single write to match python comment).
		_, _ = c.Write([]byte(s.acceptDestB64 + "\n"))
		_, _ = c.Write(s.acceptPayload)

		// Echo mode for remaining bytes.
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				_, _ = c.Write(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}
}

func TestMockSAM_StreamAccept_ParsesDestAndFirstData(t *testing.T) {
	s := startMockSAMServer(t)
	t.Cleanup(s.close)

	client := NewDefaultSAMClient(s.addr())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sess, err := client.CreateStreamSession(ctx, "test", "STREAM", nil, nil, Address{})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	defer sess.Close()

	inc, err := client.StreamAccept(ctx, sess, Address{})
	if err != nil {
		t.Fatalf("stream accept: %v", err)
	}
	defer inc.Conn.Close()

	if inc.DestB64 == "" {
		t.Fatalf("expected dest")
	}
	if _, err := I2PBase64Decode(inc.DestB64); err != nil {
		t.Fatalf("dest should be valid i2p base64: %v", err)
	}
	if string(inc.FirstData) != "hello-from-remote" {
		t.Fatalf("first data mismatch: got %q", string(inc.FirstData))
	}
}

func TestTunnel_ClientTunnel_EchoThroughMockSAM(t *testing.T) {
	s := startMockSAMServer(t)
	t.Cleanup(s.close)

	// Start client tunnel on a free port.
	port, err := GetFreePort()
	if err != nil {
		t.Skipf("free port unavailable in sandbox: %v", err)
	}
	tunAddr := Address{Host: "127.0.0.1", Port: port}

	client := NewDefaultSAMClient(s.addr())
	tun := NewClientTunnel(tunAddr, "example.i2p", s.addr(), client, nil, "test", nil)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := tun.Run(ctx); err != nil {
		t.Fatalf("run: %v", err)
	}
	t.Cleanup(tun.Stop)

	// Connect to local tunnel and expect echo back.
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", itoa(port)), 2*time.Second)
	if err != nil {
		t.Fatalf("dial tunnel: %v", err)
	}
	defer conn.Close()

	msg := []byte("ping")
	_, _ = conn.Write(msg)
	buf := make([]byte, len(msg))
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(buf) != "ping" {
		t.Fatalf("echo mismatch: got %q", string(buf))
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [32]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + (n % 10))
		n /= 10
	}
	return string(b[i:])
}
