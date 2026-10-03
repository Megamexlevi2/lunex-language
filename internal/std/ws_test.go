package std

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"testing"
)

func TestWebSocketClientMaskingIsRandom(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	readHeader := func() ([4]byte, error) {
		var header [6]byte
		if _, err := io.ReadFull(serverConn, header[:]); err != nil {
			return [4]byte{}, err
		}
		if header[1]&0x80 == 0 {
			return [4]byte{}, fmt.Errorf("expected masked frame")
		}
		return [4]byte{header[2], header[3], header[4], header[5]}, nil
	}

	writeDone := make(chan error, 1)
	go func() { writeDone <- writeWSFrame(clientConn, 1, []byte("a"), true) }()
	first, err := readHeader()
	if err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 1)
	if _, err := io.ReadFull(serverConn, payload); err != nil {
		t.Fatal(err)
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}

	writeDone = make(chan error, 1)
	go func() { writeDone <- writeWSFrame(clientConn, 1, []byte("a"), true) }()
	second, err := readHeader()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadFull(serverConn, payload); err != nil {
		t.Fatal(err)
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("client masking key was reused")
	}
}

func TestWebSocketFrames(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	payload := []byte("hello")
	writeDone := make(chan error, 1)
	go func() { writeDone <- writeWSFrame(clientConn, 1, payload, true) }()
	fin, opcode, got, err := readWSFrame(bufio.NewReader(serverConn), true)
	if err != nil || !fin || opcode != 1 || string(got) != string(payload) {
		t.Fatalf("masked frame fin=%v opcode=%d payload=%q err=%v", fin, opcode, got, err)
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}

	writeDone = make(chan error, 1)
	go func() { writeDone <- writeWSFrame(clientConn, 1, payload, false) }()
	fin, opcode, got, err = readWSFrame(bufio.NewReader(serverConn), false)
	if err != nil || !fin || opcode != 1 || string(got) != string(payload) {
		t.Fatalf("unmasked frame fin=%v opcode=%d payload=%q err=%v", fin, opcode, got, err)
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}

	if got := computeWSAccept("dGhlIHNhbXBsZSBub25jZQ=="); got != "s3pPLMBiTxaQ9kYGzzhZRbK+xOo=" {
		t.Fatalf("unexpected accept value %q", got)
	}
}
