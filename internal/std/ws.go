package std

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"lunex/internal/runtime"
	shared "lunex/internal/std/shared"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
)

const maxWSFramePayload = 64 << 20

type wsConn struct {
	conn     net.Conn
	reader   *bufio.Reader
	mu       sync.Mutex
	closed   bool
	isClient bool
	onMsg    *runtime.Value
	onClose  *runtime.Value
}

func (c *wsConn) send(msg string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return fmt.Errorf("connection closed")
	}
	return writeWSFrame(c.conn, 0x1, []byte(msg), c.isClient)
}

func (c *wsConn) close() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	_ = writeWSFrame(c.conn, 0x8, nil, c.isClient)
	_ = c.conn.Close()
	c.mu.Unlock()
}

func (c *wsConn) readLoop() {
	defer func() {
		c.close()
		if c.onClose != nil && runtime.CallFunction != nil {
			runtime.CallFunction(c.onClose, nil, nil)
		}
	}()
	reader := c.reader
	if reader == nil {
		reader = bufio.NewReader(c.conn)
	}
	var fragments []byte
	fragmentOpcode := byte(0)
	for {
		fin, opcode, payload, err := readWSFrame(reader, !c.isClient)
		if err != nil {
			return
		}
		switch opcode {
		case 0x0:
			if fragmentOpcode == 0 || len(fragments)+len(payload) > maxWSFramePayload {
				return
			}
			fragments = append(fragments, payload...)
			if fin {
				if c.onMsg != nil && runtime.CallFunction != nil {
					runtime.CallFunction(c.onMsg, []*runtime.Value{runtime.StringVal(string(fragments))}, nil)
				}
				fragments = nil
				fragmentOpcode = 0
			}
		case 0x1, 0x2:
			if fragmentOpcode != 0 {
				return
			}
			if fin {
				if opcode == 0x1 && c.onMsg != nil && runtime.CallFunction != nil {
					runtime.CallFunction(c.onMsg, []*runtime.Value{runtime.StringVal(string(payload))}, nil)
				}
				if opcode == 0x2 && c.onMsg != nil && runtime.CallFunction != nil {
					runtime.CallFunction(c.onMsg, []*runtime.Value{runtime.StringVal(string(payload))}, nil)
				}
			} else {
				fragments = append(fragments[:0], payload...)
				fragmentOpcode = opcode
			}
		case 0x8:
			return
		case 0x9:
			c.mu.Lock()
			if !c.closed {
				_ = writeWSFrame(c.conn, 0xA, payload, c.isClient)
			}
			c.mu.Unlock()
		case 0xA:
		default:
			return
		}
	}
}

func readWSFrame(r *bufio.Reader, peerIsClient bool) (bool, byte, []byte, error) {
	b0, err := r.ReadByte()
	if err != nil {
		return false, 0, nil, err
	}
	fin := b0&0x80 != 0
	rsv := b0 & 0x70
	opcode := b0 & 0x0f
	if rsv != 0 {
		return false, 0, nil, fmt.Errorf("websocket reserved bits are set")
	}
	if (opcode >= 0x3 && opcode <= 0x7) || opcode >= 0xB {
		return false, 0, nil, fmt.Errorf("invalid websocket opcode")
	}
	if opcode >= 0x8 && !fin {
		return false, 0, nil, fmt.Errorf("invalid websocket control frame")
	}
	b1, err := r.ReadByte()
	if err != nil {
		return false, 0, nil, err
	}
	masked := b1&0x80 != 0
	if masked != peerIsClient {
		return false, 0, nil, fmt.Errorf("invalid websocket masking")
	}
	payloadLen := uint64(b1 & 0x7f)
	switch payloadLen {
	case 126:
		var ext [2]byte
		if _, err := r.Read(ext[:]); err != nil {
			return false, 0, nil, err
		}
		payloadLen = uint64(ext[0])<<8 | uint64(ext[1])
	case 127:
		var ext [8]byte
		if _, err := r.Read(ext[:]); err != nil {
			return false, 0, nil, err
		}
		if ext[0]&0x80 != 0 {
			return false, 0, nil, fmt.Errorf("invalid websocket payload length")
		}
		for _, b := range ext {
			payloadLen = payloadLen<<8 | uint64(b)
		}
	}
	if opcode >= 0x8 && payloadLen > 125 {
		return false, 0, nil, fmt.Errorf("websocket control frame payload too large")
	}
	if payloadLen > maxWSFramePayload {
		return false, 0, nil, fmt.Errorf("websocket frame exceeds 64 MiB")
	}
	var mask [4]byte
	if masked {
		if _, err := r.Read(mask[:]); err != nil {
			return false, 0, nil, err
		}
	}
	payload := make([]byte, int(payloadLen))
	if _, err := ioReadFull(r, payload); err != nil {
		return false, 0, nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return fin, opcode, payload, nil
}

func ioReadFull(r *bufio.Reader, p []byte) (int, error) {
	n := 0
	for n < len(p) {
		m, err := r.Read(p[n:])
		n += m
		if err != nil {
			return n, err
		}
		if m == 0 {
			return n, fmt.Errorf("short websocket read")
		}
	}
	return n, nil
}

func writeWSFrame(conn net.Conn, opcode byte, payload []byte, mask bool) error {
	if len(payload) > maxWSFramePayload {
		return fmt.Errorf("websocket frame exceeds 64 MiB")
	}
	if (opcode >= 0x3 && opcode <= 0x7) || opcode >= 0xB {
		return fmt.Errorf("invalid websocket opcode")
	}
	first := byte(0x80 | (opcode & 0x0f))
	header := make([]byte, 0, 14)
	header = append(header, first)
	length := uint64(len(payload))
	maskBit := byte(0)
	if mask {
		maskBit = 0x80
	}
	switch {
	case length < 126:
		header = append(header, maskBit|byte(length))
	case length <= 0xffff:
		header = append(header, maskBit|126, byte(length>>8), byte(length))
	default:
		header = append(header, maskBit|127)
		for i := 7; i >= 0; i-- {
			header = append(header, byte(length>>(uint(i)*8)))
		}
	}
	if !mask {
		if _, err := conn.Write(header); err != nil {
			return err
		}
		if len(payload) == 0 {
			return nil
		}
		_, err := conn.Write(payload)
		return err
	}
	var key [4]byte
	if _, err := rand.Read(key[:]); err != nil {
		return err
	}
	header = append(header, key[:]...)
	frame := make([]byte, len(payload))
	for i := range payload {
		frame[i] = payload[i] ^ key[i%4]
	}
	if _, err := conn.Write(header); err != nil {
		return err
	}
	if len(frame) == 0 {
		return nil
	}
	_, err := conn.Write(frame)
	return err
}

func wsHandshake(w http.ResponseWriter, r *http.Request) (net.Conn, *bufio.Reader, error) {
	if r.Method != http.MethodGet {
		return nil, nil, fmt.Errorf("websocket upgrade requires GET")
	}
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, nil, fmt.Errorf("invalid websocket upgrade")
	}
	if !strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") {
		return nil, nil, fmt.Errorf("missing websocket connection upgrade")
	}
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		return nil, nil, fmt.Errorf("unsupported websocket version")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(decoded) != 16 {
		return nil, nil, fmt.Errorf("invalid Sec-WebSocket-Key")
	}
	accept := computeWSAccept(key)
	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("hijacking not supported")
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		return nil, nil, err
	}
	resp := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := buf.WriteString(resp); err != nil {
		conn.Close()
		return nil, nil, err
	}
	if err := buf.Flush(); err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, buf.Reader, nil
}

func computeWSAccept(key string) string {
	const magic = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	h := sha1.New()
	_, _ = h.Write([]byte(key + magic))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func wsConnValue(c *wsConn) *runtime.Value {
	return runtime.ObjectVal(map[string]*runtime.Value{
		"send": runtime.FuncVal(&runtime.Function{Name: "send", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.Undefined, nil
			}
			msg := args[0].ToString()
			if args[0].Tag == runtime.TypeObject || args[0].Tag == runtime.TypeArray {
				msg = shared.ValueToJSON(args[0])
			}
			return runtime.Undefined, c.send(msg)
		}}),
		"close": runtime.FuncVal(&runtime.Function{Name: "close", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			c.close()
			return runtime.Undefined, nil
		}}),
		"onMessage": runtime.FuncVal(&runtime.Function{Name: "onMessage", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) > 0 && args[0].Tag == runtime.TypeFunction {
				c.onMsg = args[0]
			}
			return runtime.Undefined, nil
		}}),
		"onClose": runtime.FuncVal(&runtime.Function{Name: "onClose", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) > 0 && args[0].Tag == runtime.TypeFunction {
				c.onClose = args[0]
			}
			return runtime.Undefined, nil
		}}),
		"isClosed": runtime.FuncVal(&runtime.Function{Name: "isClosed", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			c.mu.Lock()
			closed := c.closed
			c.mu.Unlock()
			return runtime.BoolVal(closed), nil
		}}),
	})
}

func WsModule() *runtime.Value {
	return runtime.ObjectVal(map[string]*runtime.Value{
		"createServer": runtime.FuncVal(&runtime.Function{Name: "createServer", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			port := 8080
			if len(args) > 0 {
				port = int(args[0].ToNumber())
			}
			var connHandler *runtime.Value
			var options *runtime.Value
			if len(args) >= 3 {
				if args[1] != nil && args[1].Tag == runtime.TypeFunction {
					connHandler = args[1]
					options = args[2]
				} else {
					options = args[1]
					connHandler = args[2]
				}
			} else if len(args) == 2 {
				if args[1] != nil && args[1].Tag == runtime.TypeFunction {
					connHandler = args[1]
				} else {
					options = args[1]
				}
			}

			var tlsConfig *tls.Config
			if options != nil && options.Tag == runtime.TypeObject {
				certFile := ""
				keyFile := ""
				if v, ok := options.ObjVal["certFile"]; ok && v != nil {
					certFile = v.ToString()
				}
				if v, ok := options.ObjVal["keyFile"]; ok && v != nil {
					keyFile = v.ToString()
				}
				if certFile != "" || keyFile != "" {
					if certFile == "" || keyFile == "" {
						return runtime.Null, fmt.Errorf("websocket TLS requires both certFile and keyFile")
					}
					cert, err := tls.LoadX509KeyPair(certFile, keyFile)
					if err != nil {
						return runtime.Null, err
					}
					tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12}
				}
			}

			var clients []*wsConn
			var clientsMu sync.Mutex

			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				conn, reader, err := wsHandshake(w, r)
				if err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				c := &wsConn{conn: conn, reader: reader, isClient: false}
				connVal := wsConnValue(c)
				clientsMu.Lock()
				clients = append(clients, c)
				clientsMu.Unlock()
				c.onClose = runtime.FuncVal(&runtime.Function{Name: "_onClose", Native: func(a []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
					clientsMu.Lock()
					for i, cl := range clients {
						if cl == c {
							clients = append(clients[:i], clients[i+1:]...)
							break
						}
					}
					clientsMu.Unlock()
					return runtime.Undefined, nil
				}})
				if connHandler != nil && runtime.CallFunction != nil {
					runtime.CallFunction(connHandler, []*runtime.Value{connVal}, nil)
				}
				go c.readLoop()
			})

			ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
			if err != nil {
				return runtime.Null, err
			}
			actualPort := ln.Addr().(*net.TCPAddr).Port
			if tlsConfig != nil {
				ln = tls.NewListener(ln, tlsConfig)
			}
			runtime.KeepAliveAdd()
			go func() {
				defer runtime.KeepAliveDone()
				_ = http.Serve(ln, mux)
			}()

			return runtime.ObjectVal(map[string]*runtime.Value{
				"port":   runtime.NumberVal(float64(actualPort)),
				"secure": runtime.BoolVal(tlsConfig != nil),
				"broadcast": runtime.FuncVal(&runtime.Function{Name: "broadcast", Native: func(a []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
					if len(a) == 0 {
						return runtime.Undefined, nil
					}
					msg := a[0].ToString()
					if a[0].Tag == runtime.TypeObject || a[0].Tag == runtime.TypeArray {
						msg = shared.ValueToJSON(a[0])
					}
					clientsMu.Lock()
					snap := make([]*wsConn, len(clients))
					copy(snap, clients)
					clientsMu.Unlock()
					for _, cl := range snap {
						if err := cl.send(msg); err != nil {
							cl.close()
						}
					}
					return runtime.Undefined, nil
				}}),
				"clientCount": runtime.FuncVal(&runtime.Function{Name: "clientCount", Native: func(a []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
					clientsMu.Lock()
					n := len(clients)
					clientsMu.Unlock()
					return runtime.NumberVal(float64(n)), nil
				}}),
				"close": runtime.FuncVal(&runtime.Function{Name: "close", Native: func(a []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
					return runtime.Undefined, ln.Close()
				}}),
			}), nil
		}}),

		"send": runtime.FuncVal(&runtime.Function{Name: "send", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.Undefined, nil
			}
			if sendFn, ok := args[0].ObjVal["send"]; ok {
				_, err := runtime.CallFunction(sendFn, []*runtime.Value{args[1]}, nil)
				return runtime.Undefined, err
			}
			return runtime.Undefined, nil
		}}),

		"onMessage": runtime.FuncVal(&runtime.Function{Name: "onMessage", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.Undefined, nil
			}
			if fn, ok := args[0].ObjVal["onMessage"]; ok {
				_, err := runtime.CallFunction(fn, []*runtime.Value{args[1]}, nil)
				return runtime.Undefined, err
			}
			return runtime.Undefined, nil
		}}),

		"onClose": runtime.FuncVal(&runtime.Function{Name: "onClose", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) < 2 {
				return runtime.Undefined, nil
			}
			if fn, ok := args[0].ObjVal["onClose"]; ok {
				_, err := runtime.CallFunction(fn, []*runtime.Value{args[1]}, nil)
				return runtime.Undefined, err
			}
			return runtime.Undefined, nil
		}}),

		"closeServer": runtime.FuncVal(&runtime.Function{Name: "closeServer", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) > 0 {
				if closeFn, ok := args[0].ObjVal["close"]; ok {
					_, err := runtime.CallFunction(closeFn, nil, nil)
					return runtime.Undefined, err
				}
			}
			return runtime.Undefined, nil
		}}),

		"connect": runtime.FuncVal(&runtime.Function{Name: "connect", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) == 0 {
				return runtime.Null, fmt.Errorf("url required")
			}
			rawURL := args[0].ToString()
			u, err := url.Parse(rawURL)
			if err != nil || u.Host == "" {
				return runtime.Null, fmt.Errorf("invalid websocket URL: %s", rawURL)
			}
			if u.Scheme != "ws" && u.Scheme != "wss" {
				return runtime.Null, fmt.Errorf("unsupported websocket scheme: %s", u.Scheme)
			}
			hostname := u.Hostname()
			port := u.Port()
			if port == "" {
				if u.Scheme == "wss" {
					port = "443"
				} else {
					port = "80"
				}
			}
			addr := net.JoinHostPort(hostname, port)
			var conn net.Conn
			if u.Scheme == "wss" {
				dialer := &net.Dialer{}
				conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: hostname, MinVersion: tls.VersionTLS12})
			} else {
				conn, err = net.Dial("tcp", addr)
			}
			if err != nil {
				return runtime.Null, err
			}

			var nonce [16]byte
			if _, err := rand.Read(nonce[:]); err != nil {
				conn.Close()
				return runtime.Null, err
			}
			wsKey := base64.StdEncoding.EncodeToString(nonce[:])
			path := u.EscapedPath()
			if path == "" {
				path = "/"
			}
			if u.RawQuery != "" {
				path += "?" + u.RawQuery
			}
			hostHeader := u.Host
			handshake := "GET " + path + " HTTP/1.1\r\n" +
				"Host: " + hostHeader + "\r\n" +
				"Upgrade: websocket\r\n" +
				"Connection: Upgrade\r\n" +
				"Sec-WebSocket-Key: " + wsKey + "\r\n" +
				"Sec-WebSocket-Version: 13\r\n\r\n"
			if _, err = conn.Write([]byte(handshake)); err != nil {
				conn.Close()
				return runtime.Null, err
			}

			reader := bufio.NewReader(conn)
			statusLine, err := reader.ReadString('\n')
			if err != nil {
				conn.Close()
				return runtime.Null, err
			}
			parts := strings.Fields(statusLine)
			if len(parts) < 2 || parts[1] != "101" {
				conn.Close()
				return runtime.Null, fmt.Errorf("WebSocket upgrade failed: %s", strings.TrimSpace(statusLine))
			}
			accept := ""
			for {
				line, readErr := reader.ReadString('\n')
				if readErr != nil {
					conn.Close()
					return runtime.Null, readErr
				}
				line = strings.TrimRight(line, "\r\n")
				if line == "" {
					break
				}
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "Sec-WebSocket-Accept") {
					accept = strings.TrimSpace(parts[1])
				}
			}
			if accept != computeWSAccept(wsKey) {
				conn.Close()
				return runtime.Null, fmt.Errorf("invalid Sec-WebSocket-Accept")
			}

			c := &wsConn{conn: conn, reader: reader, isClient: true}
			go c.readLoop()
			return wsConnValue(c), nil
		}}),

		"closeClient": runtime.FuncVal(&runtime.Function{Name: "closeClient", Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
			if len(args) > 0 {
				if closeFn, ok := args[0].ObjVal["close"]; ok {
					_, err := runtime.CallFunction(closeFn, nil, nil)
					return runtime.Undefined, err
				}
			}
			return runtime.Undefined, nil
		}}),
	})
}
