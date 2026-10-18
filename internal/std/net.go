package std

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"lunex/internal/runtime"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	netDefaultReadSize = 4096
	netMaxReadSize     = 16 << 20
)

type stdNetConn struct {
	conn    net.Conn
	reader  *bufio.Reader
	stateMu sync.Mutex
	readMu  sync.Mutex
	writeMu sync.Mutex
	closed  bool
}

type stdNetListener struct {
	listener net.Listener
	stateMu  sync.Mutex
	closed   bool
}

type stdNetPacketConn struct {
	conn    net.PacketConn
	stateMu sync.Mutex
	readMu  sync.Mutex
	writeMu sync.Mutex
	closed  bool
}

func netFunction(name string, fn func([]*runtime.Value) (*runtime.Value, error)) *runtime.Value {
	return runtime.FuncVal(&runtime.Function{Name: name, Native: func(args []*runtime.Value, _ *runtime.Value) (*runtime.Value, error) {
		return fn(args)
	}})
}

func netArg(args []*runtime.Value, index int) *runtime.Value {
	if index >= 0 && index < len(args) && args[index] != nil {
		return args[index]
	}
	return runtime.Undefined
}

func netStringArg(args []*runtime.Value, index int, fn string) (string, error) {
	v := netArg(args, index)
	if v.Tag != runtime.TypeString {
		return "", fmt.Errorf("std.net.%s: argument %d must be a string", fn, index+1)
	}
	return v.StrVal, nil
}

func netIntArg(args []*runtime.Value, index int, fn string, min, max int64) (int64, error) {
	v := netArg(args, index)
	if v.Tag != runtime.TypeNumber || math.IsNaN(v.NumVal) || math.IsInf(v.NumVal, 0) || math.Trunc(v.NumVal) != v.NumVal {
		return 0, fmt.Errorf("std.net.%s: argument %d must be an integer", fn, index+1)
	}
	if v.NumVal < float64(min) || v.NumVal > float64(max) {
		return 0, fmt.Errorf("std.net.%s: argument %d is out of range", fn, index+1)
	}
	return int64(v.NumVal), nil
}

func netOptionsArg(args []*runtime.Value, index int, method string) (map[string]*runtime.Value, error) {
	v := netArg(args, index)
	if v.Tag == runtime.TypeUndefined || v.Tag == runtime.TypeNull {
		return nil, nil
	}
	if v.Tag != runtime.TypeObject {
		return nil, fmt.Errorf("std.net.%s: options must be an object", method)
	}
	return v.ObjVal, nil
}

func netOptionBool(options map[string]*runtime.Value, key string) bool {
	v := options[key]
	return v != nil && v.Tag == runtime.TypeBool && v.BoolVal
}

func netOptionStringStrict(options map[string]*runtime.Value, key, fn string) (string, error) {
	v, ok := options[key]
	if !ok || v == nil || v.Tag == runtime.TypeNull || v.Tag == runtime.TypeUndefined {
		return "", nil
	}
	if v.Tag != runtime.TypeString {
		return "", fmt.Errorf("std.net.%s: option %q must be a string", fn, key)
	}
	return v.StrVal, nil
}

func netOptionInt(options map[string]*runtime.Value, key string, fallback int64) (int64, error) {
	v, ok := options[key]
	if !ok || v == nil || v.Tag == runtime.TypeNull || v.Tag == runtime.TypeUndefined {
		return fallback, nil
	}
	if v.Tag != runtime.TypeNumber || math.IsNaN(v.NumVal) || math.IsInf(v.NumVal, 0) || math.Trunc(v.NumVal) != v.NumVal || v.NumVal < 0 || v.NumVal > float64(2147483647) {
		return 0, fmt.Errorf("std.net: option %q must be a non-negative integer in milliseconds", key)
	}
	return int64(v.NumVal), nil
}

func netContext(options map[string]*runtime.Value) (context.Context, context.CancelFunc, error) {
	ms, err := netOptionInt(options, "timeoutMs", 0)
	if err != nil {
		return nil, nil, err
	}
	if ms == 0 {
		ms, err = netOptionInt(options, "timeout", 0)
		if err != nil {
			return nil, nil, err
		}
	}
	if ms <= 0 {
		return context.Background(), func() {}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(ms)*time.Millisecond)
	return ctx, cancel, nil
}

func netNormalizeNetwork(network string, allowed ...string) (string, error) {
	for _, value := range allowed {
		if network == value {
			return network, nil
		}
	}
	return "", fmt.Errorf("std.net: unsupported network %q", network)
}

func netBytesValue(data []byte) *runtime.Value {
	values := make([]*runtime.Value, len(data))
	for i, b := range data {
		values[i] = runtime.NumberVal(float64(b))
	}
	return runtime.ArrayVal(values)
}

func netBytesArg(v *runtime.Value, fn string) ([]byte, error) {
	if v == nil {
		return nil, fmt.Errorf("std.net.%s: data is required", fn)
	}
	if v.Tag == runtime.TypeString {
		return []byte(v.StrVal), nil
	}
	if v.Tag != runtime.TypeArray {
		return nil, fmt.Errorf("std.net.%s: data must be a string or an array of bytes", fn)
	}
	data := make([]byte, len(v.ArrVal))
	for i, item := range v.ArrVal {
		if item == nil || item.Tag != runtime.TypeNumber || math.IsNaN(item.NumVal) || math.IsInf(item.NumVal, 0) || math.Trunc(item.NumVal) != item.NumVal || item.NumVal < 0 || item.NumVal > 255 {
			return nil, fmt.Errorf("std.net.%s: byte at index %d must be an integer from 0 to 255", fn, i)
		}
		data[i] = byte(item.NumVal)
	}
	return data, nil
}

func netReadSize(args []*runtime.Value, index int, fn string) (int, error) {
	if len(args) <= index || netArg(args, index).Tag == runtime.TypeUndefined || netArg(args, index).Tag == runtime.TypeNull {
		return netDefaultReadSize, nil
	}
	n, err := netIntArg(args, index, fn, 1, netMaxReadSize)
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func netPacketReadSize(args []*runtime.Value, fn string) (int, error) {
	if len(args) == 0 || netArg(args, 0).Tag == runtime.TypeUndefined || netArg(args, 0).Tag == runtime.TypeNull {
		return 65535, nil
	}
	return netReadSize(args, 0, fn)
}

func netObjectAddr(addr net.Addr) *runtime.Value {
	if addr == nil {
		return runtime.Null
	}
	return runtime.ObjectVal(map[string]*runtime.Value{
		"network": runtime.StringVal(addr.Network()),
		"address": runtime.StringVal(addr.String()),
	})
}

func (c *stdNetConn) isClosed() bool {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	return c.closed
}

func (c *stdNetConn) close() error {
	c.stateMu.Lock()
	if c.closed {
		c.stateMu.Unlock()
		return nil
	}
	c.closed = true
	c.stateMu.Unlock()
	return c.conn.Close()
}

func (c *stdNetConn) readBytes(size int) ([]byte, error) {
	if c.isClosed() {
		return nil, net.ErrClosed
	}
	c.readMu.Lock()
	defer c.readMu.Unlock()
	buf := make([]byte, size)
	n, err := c.reader.Read(buf)
	if n > 0 {
		return buf[:n], nil
	}
	if errors.Is(err, io.EOF) {
		return []byte{}, nil
	}
	if err != nil {
		return nil, err
	}
	return []byte{}, nil
}

func (c *stdNetConn) writeBytes(data []byte) (int, error) {
	if c.isClosed() {
		return 0, net.ErrClosed
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	written := 0
	for written < len(data) {
		n, err := c.conn.Write(data[written:])
		written += n
		if err != nil {
			return written, err
		}
		if n == 0 {
			return written, io.ErrShortWrite
		}
	}
	return written, nil
}

func (c *stdNetConn) setDeadline(method string, args []*runtime.Value) (*runtime.Value, error) {
	if len(args) == 0 {
		return runtime.Undefined, fmt.Errorf("std.net.%s: timeout in milliseconds is required", method)
	}
	ms, err := netIntArg(args, 0, method, 0, 2147483647)
	if err != nil {
		return runtime.Undefined, err
	}
	var deadline time.Time
	if ms > 0 {
		deadline = time.Now().Add(time.Duration(ms) * time.Millisecond)
	}
	switch method {
	case "setDeadline":
		err = c.conn.SetDeadline(deadline)
	case "setReadDeadline":
		err = c.conn.SetReadDeadline(deadline)
	case "setWriteDeadline":
		err = c.conn.SetWriteDeadline(deadline)
	}
	return runtime.Undefined, err
}

func netConnValue(conn net.Conn) *runtime.Value {
	c := &stdNetConn{conn: conn, reader: bufio.NewReader(conn)}
	obj := runtime.ObjectVal(map[string]*runtime.Value{})
	obj.ObjVal["read"] = netFunction("read", func(args []*runtime.Value) (*runtime.Value, error) {
		size, err := netReadSize(args, 0, "read")
		if err != nil {
			return runtime.Undefined, err
		}
		data, err := c.readBytes(size)
		if err != nil {
			return runtime.Undefined, err
		}
		return runtime.StringVal(string(data)), nil
	})
	obj.ObjVal["readBytes"] = netFunction("readBytes", func(args []*runtime.Value) (*runtime.Value, error) {
		size, err := netReadSize(args, 0, "readBytes")
		if err != nil {
			return runtime.Undefined, err
		}
		data, err := c.readBytes(size)
		if err != nil {
			return runtime.Undefined, err
		}
		return netBytesValue(data), nil
	})
	obj.ObjVal["readFull"] = netFunction("readFull", func(args []*runtime.Value) (*runtime.Value, error) {
		if len(args) == 0 {
			return runtime.Undefined, fmt.Errorf("std.net.readFull: byte count is required")
		}
		size, err := netIntArg(args, 0, "readFull", 0, netMaxReadSize)
		if err != nil {
			return runtime.Undefined, err
		}
		if c.isClosed() {
			return runtime.Undefined, net.ErrClosed
		}
		c.readMu.Lock()
		defer c.readMu.Unlock()
		data := make([]byte, int(size))
		if _, err := io.ReadFull(c.reader, data); err != nil {
			return runtime.Undefined, err
		}
		return runtime.StringVal(string(data)), nil
	})
	obj.ObjVal["readLine"] = netFunction("readLine", func(args []*runtime.Value) (*runtime.Value, error) {
		if c.isClosed() {
			return runtime.Undefined, net.ErrClosed
		}
		c.readMu.Lock()
		defer c.readMu.Unlock()
		line, err := c.reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return runtime.Undefined, err
		}
		return runtime.StringVal(strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")), nil
	})
	obj.ObjVal["write"] = netFunction("write", func(args []*runtime.Value) (*runtime.Value, error) {
		if len(args) == 0 {
			return runtime.Undefined, fmt.Errorf("std.net.write: data is required")
		}
		data, err := netBytesArg(args[0], "write")
		if err != nil {
			return runtime.Undefined, err
		}
		n, err := c.writeBytes(data)
		if err != nil {
			return runtime.Undefined, err
		}
		return runtime.NumberVal(float64(n)), nil
	})
	obj.ObjVal["writeBytes"] = netFunction("writeBytes", func(args []*runtime.Value) (*runtime.Value, error) {
		if len(args) == 0 || netArg(args, 0).Tag != runtime.TypeArray {
			return runtime.Undefined, fmt.Errorf("std.net.writeBytes: data must be an array of bytes")
		}
		data, err := netBytesArg(args[0], "writeBytes")
		if err != nil {
			return runtime.Undefined, err
		}
		n, err := c.writeBytes(data)
		if err != nil {
			return runtime.Undefined, err
		}
		return runtime.NumberVal(float64(n)), nil
	})
	obj.ObjVal["close"] = netFunction("close", func(args []*runtime.Value) (*runtime.Value, error) {
		return runtime.Undefined, c.close()
	})
	obj.ObjVal["isClosed"] = netFunction("isClosed", func(args []*runtime.Value) (*runtime.Value, error) {
		return runtime.BoolVal(c.isClosed()), nil
	})
	obj.ObjVal["localAddr"] = netFunction("localAddr", func(args []*runtime.Value) (*runtime.Value, error) {
		return runtime.StringVal(c.conn.LocalAddr().String()), nil
	})
	obj.ObjVal["remoteAddr"] = netFunction("remoteAddr", func(args []*runtime.Value) (*runtime.Value, error) {
		return runtime.StringVal(c.conn.RemoteAddr().String()), nil
	})
	obj.ObjVal["localAddress"] = obj.ObjVal["localAddr"]
	obj.ObjVal["remoteAddress"] = obj.ObjVal["remoteAddr"]
	obj.ObjVal["setDeadline"] = netFunction("setDeadline", func(args []*runtime.Value) (*runtime.Value, error) {
		return c.setDeadline("setDeadline", args)
	})
	obj.ObjVal["setReadDeadline"] = netFunction("setReadDeadline", func(args []*runtime.Value) (*runtime.Value, error) {
		return c.setDeadline("setReadDeadline", args)
	})
	obj.ObjVal["setWriteDeadline"] = netFunction("setWriteDeadline", func(args []*runtime.Value) (*runtime.Value, error) {
		return c.setDeadline("setWriteDeadline", args)
	})
	obj.ObjVal["setNoDelay"] = netFunction("setNoDelay", func(args []*runtime.Value) (*runtime.Value, error) {
		v := netArg(args, 0)
		tcp, ok := c.conn.(*net.TCPConn)
		if !ok {
			return runtime.Undefined, fmt.Errorf("std.net.setNoDelay: connection is not TCP")
		}
		if len(args) == 0 || v.Tag != runtime.TypeBool {
			return runtime.Undefined, fmt.Errorf("std.net.setNoDelay: enabled must be a boolean")
		}
		return runtime.Undefined, tcp.SetNoDelay(v.BoolVal)
	})
	obj.ObjVal["setKeepAlive"] = netFunction("setKeepAlive", func(args []*runtime.Value) (*runtime.Value, error) {
		v := netArg(args, 0)
		tcp, ok := c.conn.(*net.TCPConn)
		if !ok {
			return runtime.Undefined, fmt.Errorf("std.net.setKeepAlive: connection is not TCP")
		}
		if len(args) == 0 || v.Tag != runtime.TypeBool {
			return runtime.Undefined, fmt.Errorf("std.net.setKeepAlive: enabled must be a boolean")
		}
		return runtime.Undefined, tcp.SetKeepAlive(v.BoolVal)
	})
	obj.ObjVal["setKeepAlivePeriod"] = netFunction("setKeepAlivePeriod", func(args []*runtime.Value) (*runtime.Value, error) {
		if len(args) == 0 {
			return runtime.Undefined, fmt.Errorf("std.net.setKeepAlivePeriod: milliseconds are required")
		}
		ms, err := netIntArg(args, 0, "setKeepAlivePeriod", 1, 2147483647)
		if err != nil {
			return runtime.Undefined, err
		}
		tcp, ok := c.conn.(*net.TCPConn)
		if !ok {
			return runtime.Undefined, fmt.Errorf("std.net.setKeepAlivePeriod: connection is not TCP")
		}
		return runtime.Undefined, tcp.SetKeepAlivePeriod(time.Duration(ms) * time.Millisecond)
	})
	obj.ObjVal["tlsState"] = netFunction("tlsState", func(args []*runtime.Value) (*runtime.Value, error) {
		tlsConn, ok := c.conn.(*tls.Conn)
		if !ok {
			return runtime.Null, nil
		}
		state := tlsConn.ConnectionState()
		peers := make([]*runtime.Value, 0, len(state.PeerCertificates))
		for _, cert := range state.PeerCertificates {
			peers = append(peers, runtime.ObjectVal(map[string]*runtime.Value{
				"subject":   runtime.StringVal(cert.Subject.String()),
				"issuer":    runtime.StringVal(cert.Issuer.String()),
				"notBefore": runtime.StringVal(cert.NotBefore.UTC().Format(time.RFC3339)),
				"notAfter":  runtime.StringVal(cert.NotAfter.UTC().Format(time.RFC3339)),
			}))
		}
		return runtime.ObjectVal(map[string]*runtime.Value{
			"version":           runtime.StringVal(netTLSVersionName(state.Version)),
			"serverName":        runtime.StringVal(state.ServerName),
			"handshakeComplete": runtime.BoolVal(state.HandshakeComplete),
			"cipherSuite":       runtime.NumberVal(float64(state.CipherSuite)),
			"peerCertificates":  runtime.ArrayVal(peers),
		}), nil
	})
	return obj
}

func (l *stdNetListener) isClosed() bool {
	l.stateMu.Lock()
	defer l.stateMu.Unlock()
	return l.closed
}

func (l *stdNetListener) close() error {
	l.stateMu.Lock()
	if l.closed {
		l.stateMu.Unlock()
		return nil
	}
	l.closed = true
	l.stateMu.Unlock()
	return l.listener.Close()
}

func netListenerValue(listener net.Listener) *runtime.Value {
	l := &stdNetListener{listener: listener}
	return runtime.ObjectVal(map[string]*runtime.Value{
		"accept": netFunction("accept", func(args []*runtime.Value) (*runtime.Value, error) {
			if l.isClosed() {
				return runtime.Undefined, net.ErrClosed
			}
			conn, err := l.listener.Accept()
			if err != nil {
				return runtime.Undefined, err
			}
			return netConnValue(conn), nil
		}),
		"close": netFunction("close", func(args []*runtime.Value) (*runtime.Value, error) {
			return runtime.Undefined, l.close()
		}),
		"addr": netFunction("addr", func(args []*runtime.Value) (*runtime.Value, error) {
			return netObjectAddr(l.listener.Addr()), nil
		}),
		"address": netFunction("address", func(args []*runtime.Value) (*runtime.Value, error) {
			return runtime.StringVal(l.listener.Addr().String()), nil
		}),
		"setDeadline": netFunction("setDeadline", func(args []*runtime.Value) (*runtime.Value, error) {
			ms, err := netIntArg(args, 0, "listener.setDeadline", 0, 2147483647)
			if err != nil {
				return runtime.Undefined, err
			}
			setter, ok := l.listener.(interface{ SetDeadline(time.Time) error })
			if !ok {
				return runtime.Undefined, fmt.Errorf("std.net.listener.setDeadline: this listener does not support deadlines")
			}
			var deadline time.Time
			if ms > 0 {
				deadline = time.Now().Add(time.Duration(ms) * time.Millisecond)
			}
			return runtime.Undefined, setter.SetDeadline(deadline)
		}),
		"isClosed": netFunction("isClosed", func(args []*runtime.Value) (*runtime.Value, error) {
			return runtime.BoolVal(l.isClosed()), nil
		}),
	})
}

func (p *stdNetPacketConn) isClosed() bool {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	return p.closed
}

func (p *stdNetPacketConn) close() error {
	p.stateMu.Lock()
	if p.closed {
		p.stateMu.Unlock()
		return nil
	}
	p.closed = true
	p.stateMu.Unlock()
	return p.conn.Close()
}

func netPacketConnValue(conn net.PacketConn) *runtime.Value {
	p := &stdNetPacketConn{conn: conn}
	obj := runtime.ObjectVal(map[string]*runtime.Value{})
	obj.ObjVal["readFrom"] = netFunction("readFrom", func(args []*runtime.Value) (*runtime.Value, error) {
		size, err := netPacketReadSize(args, "readFrom")
		if err != nil {
			return runtime.Undefined, err
		}
		if p.isClosed() {
			return runtime.Undefined, net.ErrClosed
		}
		p.readMu.Lock()
		defer p.readMu.Unlock()
		buf := make([]byte, size)
		n, addr, err := p.conn.ReadFrom(buf)
		if err != nil {
			return runtime.Undefined, err
		}
		host, port := addr.String(), int64(-1)
		if udp, ok := addr.(*net.UDPAddr); ok {
			host = udp.IP.String()
			port = int64(udp.Port)
		}
		return runtime.ObjectVal(map[string]*runtime.Value{
			"data":      runtime.StringVal(string(buf[:n])),
			"bytesRead": runtime.NumberVal(float64(n)),
			"address":   runtime.StringVal(addr.String()),
			"host":      runtime.StringVal(host),
			"port":      runtime.NumberVal(float64(port)),
			"network":   runtime.StringVal(addr.Network()),
		}), nil
	})
	obj.ObjVal["readBytesFrom"] = netFunction("readBytesFrom", func(args []*runtime.Value) (*runtime.Value, error) {
		size, err := netPacketReadSize(args, "readBytesFrom")
		if err != nil {
			return runtime.Undefined, err
		}
		if p.isClosed() {
			return runtime.Undefined, net.ErrClosed
		}
		p.readMu.Lock()
		defer p.readMu.Unlock()
		buf := make([]byte, size)
		n, addr, err := p.conn.ReadFrom(buf)
		if err != nil {
			return runtime.Undefined, err
		}
		host, port := addr.String(), int64(-1)
		if udp, ok := addr.(*net.UDPAddr); ok {
			host = udp.IP.String()
			port = int64(udp.Port)
		}
		return runtime.ObjectVal(map[string]*runtime.Value{
			"data":      netBytesValue(buf[:n]),
			"bytesRead": runtime.NumberVal(float64(n)),
			"address":   runtime.StringVal(addr.String()),
			"host":      runtime.StringVal(host),
			"port":      runtime.NumberVal(float64(port)),
			"network":   runtime.StringVal(addr.Network()),
		}), nil
	})
	obj.ObjVal["writeTo"] = netFunction("writeTo", func(args []*runtime.Value) (*runtime.Value, error) {
		if len(args) < 2 {
			return runtime.Undefined, fmt.Errorf("std.net.writeTo: data and address are required")
		}
		data, err := netBytesArg(args[0], "writeTo")
		if err != nil {
			return runtime.Undefined, err
		}
		if args[1] == nil || args[1].Tag != runtime.TypeString {
			return runtime.Undefined, fmt.Errorf("std.net.writeTo: address must be a string")
		}
		var addr net.Addr
		if p.conn.LocalAddr().Network() == "unixgram" {
			addr, err = net.ResolveUnixAddr("unixgram", args[1].StrVal)
		} else {
			addr, err = net.ResolveUDPAddr("udp", args[1].StrVal)
		}
		if err != nil {
			return runtime.Undefined, err
		}
		if p.isClosed() {
			return runtime.Undefined, net.ErrClosed
		}
		p.writeMu.Lock()
		defer p.writeMu.Unlock()
		n, err := p.conn.WriteTo(data, addr)
		if err != nil {
			return runtime.Undefined, err
		}
		return runtime.NumberVal(float64(n)), nil
	})
	obj.ObjVal["close"] = netFunction("close", func(args []*runtime.Value) (*runtime.Value, error) {
		return runtime.Undefined, p.close()
	})
	obj.ObjVal["isClosed"] = netFunction("isClosed", func(args []*runtime.Value) (*runtime.Value, error) {
		return runtime.BoolVal(p.isClosed()), nil
	})
	obj.ObjVal["localAddr"] = netFunction("localAddr", func(args []*runtime.Value) (*runtime.Value, error) {
		return runtime.StringVal(p.conn.LocalAddr().String()), nil
	})
	obj.ObjVal["addr"] = netFunction("addr", func(args []*runtime.Value) (*runtime.Value, error) {
		return netObjectAddr(p.conn.LocalAddr()), nil
	})
	obj.ObjVal["setDeadline"] = netFunction("setDeadline", func(args []*runtime.Value) (*runtime.Value, error) {
		if len(args) == 0 {
			return runtime.Undefined, fmt.Errorf("std.net.setDeadline: timeout in milliseconds is required")
		}
		ms, err := netIntArg(args, 0, "setDeadline", 0, 2147483647)
		if err != nil {
			return runtime.Undefined, err
		}
		var deadline time.Time
		if ms > 0 {
			deadline = time.Now().Add(time.Duration(ms) * time.Millisecond)
		}
		return runtime.Undefined, p.conn.SetDeadline(deadline)
	})
	return obj
}

func netHostPortValue(network string, addr net.Addr) *runtime.Value {
	obj := map[string]*runtime.Value{
		"network": runtime.StringVal(network),
		"address": runtime.StringVal(addr.String()),
	}
	switch a := addr.(type) {
	case *net.TCPAddr:
		obj["ip"] = runtime.StringVal(a.IP.String())
		obj["port"] = runtime.NumberVal(float64(a.Port))
		obj["zone"] = runtime.StringVal(a.Zone)
	case *net.UDPAddr:
		obj["ip"] = runtime.StringVal(a.IP.String())
		obj["port"] = runtime.NumberVal(float64(a.Port))
		obj["zone"] = runtime.StringVal(a.Zone)
	case *net.IPAddr:
		obj["ip"] = runtime.StringVal(a.IP.String())
		obj["zone"] = runtime.StringVal(a.Zone)
	}
	return runtime.ObjectVal(obj)
}

func NetModule() *runtime.Value {
	mod := runtime.ObjectVal(map[string]*runtime.Value{})
	add := func(name string, fn func([]*runtime.Value) (*runtime.Value, error)) {
		mod.ObjVal[name] = netFunction(name, fn)
	}

	add("dial", func(args []*runtime.Value) (*runtime.Value, error) {
		network, err := netStringArg(args, 0, "dial")
		if err != nil {
			return runtime.Undefined, err
		}
		address, err := netStringArg(args, 1, "dial")
		if err != nil {
			return runtime.Undefined, err
		}
		if _, err := netNormalizeNetwork(network, "tcp", "tcp4", "tcp6", "udp", "udp4", "udp6", "unix", "unixgram", "unixpacket"); err != nil {
			return runtime.Undefined, err
		}
		options, err := netOptionsArg(args, 2, "dial")
		if err != nil {
			return runtime.Undefined, err
		}
		ctx, cancel, err := netContext(options)
		if err != nil {
			return runtime.Undefined, err
		}
		defer cancel()
		conn, err := (&net.Dialer{}).DialContext(ctx, network, address)
		if err != nil {
			return runtime.Undefined, err
		}
		return netConnValue(conn), nil
	})

	add("dialTimeout", func(args []*runtime.Value) (*runtime.Value, error) {
		if len(args) < 3 {
			return runtime.Undefined, fmt.Errorf("std.net.dialTimeout: network, address, and timeoutMs are required")
		}
		network, err := netStringArg(args, 0, "dialTimeout")
		if err != nil {
			return runtime.Undefined, err
		}
		address, err := netStringArg(args, 1, "dialTimeout")
		if err != nil {
			return runtime.Undefined, err
		}
		if _, err := netNormalizeNetwork(network, "tcp", "tcp4", "tcp6", "udp", "udp4", "udp6", "unix", "unixgram", "unixpacket"); err != nil {
			return runtime.Undefined, err
		}
		ms, err := netIntArg(args, 2, "dialTimeout", 0, 2147483647)
		if err != nil {
			return runtime.Undefined, err
		}
		conn, err := (&net.Dialer{Timeout: time.Duration(ms) * time.Millisecond}).Dial(network, address)
		if err != nil {
			return runtime.Undefined, err
		}
		return netConnValue(conn), nil
	})

	add("listen", func(args []*runtime.Value) (*runtime.Value, error) {
		network, err := netStringArg(args, 0, "listen")
		if err != nil {
			return runtime.Undefined, err
		}
		address, err := netStringArg(args, 1, "listen")
		if err != nil {
			return runtime.Undefined, err
		}
		if _, err := netNormalizeNetwork(network, "tcp", "tcp4", "tcp6", "unix", "unixpacket"); err != nil {
			return runtime.Undefined, err
		}
		listener, err := net.Listen(network, address)
		if err != nil {
			return runtime.Undefined, err
		}
		return netListenerValue(listener), nil
	})

	add("listenPacket", func(args []*runtime.Value) (*runtime.Value, error) {
		network, err := netStringArg(args, 0, "listenPacket")
		if err != nil {
			return runtime.Undefined, err
		}
		address, err := netStringArg(args, 1, "listenPacket")
		if err != nil {
			return runtime.Undefined, err
		}
		if _, err := netNormalizeNetwork(network, "udp", "udp4", "udp6", "unixgram"); err != nil {
			return runtime.Undefined, err
		}
		conn, err := net.ListenPacket(network, address)
		if err != nil {
			return runtime.Undefined, err
		}
		return netPacketConnValue(conn), nil
	})

	add("dialTLS", func(args []*runtime.Value) (*runtime.Value, error) {
		network, err := netStringArg(args, 0, "dialTLS")
		if err != nil {
			return runtime.Undefined, err
		}
		address, err := netStringArg(args, 1, "dialTLS")
		if err != nil {
			return runtime.Undefined, err
		}
		if _, err := netNormalizeNetwork(network, "tcp", "tcp4", "tcp6"); err != nil {
			return runtime.Undefined, err
		}
		options, err := netOptionsArg(args, 2, "dialTLS")
		if err != nil {
			return runtime.Undefined, err
		}
		ctx, cancel, err := netContext(options)
		if err != nil {
			return runtime.Undefined, err
		}
		defer cancel()
		config := &tls.Config{MinVersion: tls.VersionTLS12}
		serverName, err := netOptionStringStrict(options, "serverName", "dialTLS")
		if err != nil {
			return runtime.Undefined, err
		}
		if serverName != "" {
			config.ServerName = serverName
		}
		if value, ok := options["insecureSkipVerify"]; ok && value != nil && value.Tag != runtime.TypeBool {
			return runtime.Undefined, fmt.Errorf("std.net.dialTLS: option %q must be a boolean", "insecureSkipVerify")
		}
		config.InsecureSkipVerify = netOptionBool(options, "insecureSkipVerify")
		caFile, err := netOptionStringStrict(options, "caFile", "dialTLS")
		if err != nil {
			return runtime.Undefined, err
		}
		if caFile != "" {
			pemData, err := os.ReadFile(caFile)
			if err != nil {
				return runtime.Undefined, fmt.Errorf("std.net.dialTLS: read CA file: %w", err)
			}
			roots := x509.NewCertPool()
			if !roots.AppendCertsFromPEM(pemData) {
				return runtime.Undefined, fmt.Errorf("std.net.dialTLS: CA file contains no valid certificates")
			}
			config.RootCAs = roots
		}
		clientCertFile, err := netOptionStringStrict(options, "clientCertFile", "dialTLS")
		if err != nil {
			return runtime.Undefined, err
		}
		clientKeyFile, err := netOptionStringStrict(options, "clientKeyFile", "dialTLS")
		if err != nil {
			return runtime.Undefined, err
		}
		if (clientCertFile == "") != (clientKeyFile == "") {
			return runtime.Undefined, fmt.Errorf("std.net.dialTLS: clientCertFile and clientKeyFile must be provided together")
		}
		if clientCertFile != "" {
			certificate, err := tls.LoadX509KeyPair(clientCertFile, clientKeyFile)
			if err != nil {
				return runtime.Undefined, fmt.Errorf("std.net.dialTLS: load client certificate: %w", err)
			}
			config.Certificates = []tls.Certificate{certificate}
		}
		var conn net.Conn
		if deadline, ok := ctx.Deadline(); ok {
			dialer := &net.Dialer{Deadline: deadline}
			conn, err = tls.DialWithDialer(dialer, network, address, config)
		} else {
			conn, err = tls.Dial(network, address, config)
		}
		if err != nil {
			return runtime.Undefined, err
		}
		return netConnValue(conn), nil
	})

	add("listenTLS", func(args []*runtime.Value) (*runtime.Value, error) {
		network, err := netStringArg(args, 0, "listenTLS")
		if err != nil {
			return runtime.Undefined, err
		}
		address, err := netStringArg(args, 1, "listenTLS")
		if err != nil {
			return runtime.Undefined, err
		}
		if _, err := netNormalizeNetwork(network, "tcp", "tcp4", "tcp6"); err != nil {
			return runtime.Undefined, err
		}
		options, err := netOptionsArg(args, 2, "listenTLS")
		if err != nil {
			return runtime.Undefined, err
		}
		certFile, err := netOptionStringStrict(options, "certFile", "listenTLS")
		if err != nil {
			return runtime.Undefined, err
		}
		keyFile, err := netOptionStringStrict(options, "keyFile", "listenTLS")
		if err != nil {
			return runtime.Undefined, err
		}
		if certFile == "" || keyFile == "" {
			return runtime.Undefined, fmt.Errorf("std.net.listenTLS: options.certFile and options.keyFile are required")
		}
		certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return runtime.Undefined, err
		}
		config := &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}
		listener, err := tls.Listen(network, address, config)
		if err != nil {
			return runtime.Undefined, err
		}
		return netListenerValue(listener), nil
	})

	add("lookupIP", func(args []*runtime.Value) (*runtime.Value, error) {
		host, err := netStringArg(args, 0, "lookupIP")
		if err != nil {
			return runtime.Undefined, err
		}
		options, err := netOptionsArg(args, 1, "lookupIP")
		if err != nil {
			return runtime.Undefined, err
		}
		ctx, cancel, err := netContext(options)
		if err != nil {
			return runtime.Undefined, err
		}
		defer cancel()
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return runtime.Undefined, err
		}
		out := make([]*runtime.Value, 0, len(ips))
		seen := make(map[string]bool)
		for _, ip := range ips {
			s := ip.String()
			if !seen[s] {
				seen[s] = true
				out = append(out, runtime.StringVal(s))
			}
		}
		return runtime.ArrayVal(out), nil
	})
	add("lookupHost", func(args []*runtime.Value) (*runtime.Value, error) {
		return netCallModule(mod, "lookupIP", args...)
	})
	add("lookupAddr", func(args []*runtime.Value) (*runtime.Value, error) {
		address, err := netStringArg(args, 0, "lookupAddr")
		if err != nil {
			return runtime.Undefined, err
		}
		if net.ParseIP(address) == nil {
			return runtime.Undefined, fmt.Errorf("std.net.lookupAddr: address must be a valid IP literal")
		}
		options, err := netOptionsArg(args, 1, "lookupCNAME")
		if err != nil {
			return runtime.Undefined, err
		}
		ctx, cancel, err := netContext(options)
		if err != nil {
			return runtime.Undefined, err
		}
		defer cancel()
		names, err := net.DefaultResolver.LookupAddr(ctx, address)
		if err != nil {
			return runtime.Undefined, err
		}
		out := make([]*runtime.Value, 0, len(names))
		for _, name := range names {
			out = append(out, runtime.StringVal(name))
		}
		return runtime.ArrayVal(out), nil
	})
	add("lookupCNAME", func(args []*runtime.Value) (*runtime.Value, error) {
		host, err := netStringArg(args, 0, "lookupCNAME")
		if err != nil {
			return runtime.Undefined, err
		}
		options, err := netOptionsArg(args, 1, "lookupAddr")
		if err != nil {
			return runtime.Undefined, err
		}
		ctx, cancel, err := netContext(options)
		if err != nil {
			return runtime.Undefined, err
		}
		defer cancel()
		name, err := net.DefaultResolver.LookupCNAME(ctx, host)
		if err != nil {
			return runtime.Undefined, err
		}
		return runtime.StringVal(name), nil
	})
	add("lookupPort", func(args []*runtime.Value) (*runtime.Value, error) {
		network, err := netStringArg(args, 0, "lookupPort")
		if err != nil {
			return runtime.Undefined, err
		}
		service, err := netStringArg(args, 1, "lookupPort")
		if err != nil {
			return runtime.Undefined, err
		}
		port, err := net.LookupPort(network, service)
		if err != nil {
			return runtime.Undefined, err
		}
		return runtime.NumberVal(float64(port)), nil
	})
	add("lookupMX", func(args []*runtime.Value) (*runtime.Value, error) {
		host, err := netStringArg(args, 0, "lookupMX")
		if err != nil {
			return runtime.Undefined, err
		}
		options, err := netOptionsArg(args, 1, "lookupMX")
		if err != nil {
			return runtime.Undefined, err
		}
		ctx, cancel, err := netContext(options)
		if err != nil {
			return runtime.Undefined, err
		}
		defer cancel()
		records, err := net.DefaultResolver.LookupMX(ctx, host)
		if err != nil {
			return runtime.Undefined, err
		}
		out := make([]*runtime.Value, 0, len(records))
		for _, record := range records {
			out = append(out, runtime.ObjectVal(map[string]*runtime.Value{
				"host":       runtime.StringVal(record.Host),
				"preference": runtime.NumberVal(float64(record.Pref)),
			}))
		}
		return runtime.ArrayVal(out), nil
	})
	add("lookupNS", func(args []*runtime.Value) (*runtime.Value, error) {
		host, err := netStringArg(args, 0, "lookupNS")
		if err != nil {
			return runtime.Undefined, err
		}
		options, err := netOptionsArg(args, 1, "lookupNS")
		if err != nil {
			return runtime.Undefined, err
		}
		ctx, cancel, err := netContext(options)
		if err != nil {
			return runtime.Undefined, err
		}
		defer cancel()
		records, err := net.DefaultResolver.LookupNS(ctx, host)
		if err != nil {
			return runtime.Undefined, err
		}
		out := make([]*runtime.Value, 0, len(records))
		for _, record := range records {
			out = append(out, runtime.ObjectVal(map[string]*runtime.Value{"host": runtime.StringVal(record.Host)}))
		}
		return runtime.ArrayVal(out), nil
	})
	add("lookupTXT", func(args []*runtime.Value) (*runtime.Value, error) {
		host, err := netStringArg(args, 0, "lookupTXT")
		if err != nil {
			return runtime.Undefined, err
		}
		options, err := netOptionsArg(args, 1, "lookupTXT")
		if err != nil {
			return runtime.Undefined, err
		}
		ctx, cancel, err := netContext(options)
		if err != nil {
			return runtime.Undefined, err
		}
		defer cancel()
		records, err := net.DefaultResolver.LookupTXT(ctx, host)
		if err != nil {
			return runtime.Undefined, err
		}
		out := make([]*runtime.Value, 0, len(records))
		for _, record := range records {
			out = append(out, runtime.StringVal(record))
		}
		return runtime.ArrayVal(out), nil
	})
	add("lookupSRV", func(args []*runtime.Value) (*runtime.Value, error) {
		service, err := netStringArg(args, 0, "lookupSRV")
		if err != nil {
			return runtime.Undefined, err
		}
		proto, err := netStringArg(args, 1, "lookupSRV")
		if err != nil {
			return runtime.Undefined, err
		}
		host, err := netStringArg(args, 2, "lookupSRV")
		if err != nil {
			return runtime.Undefined, err
		}
		options, err := netOptionsArg(args, 3, "lookupSRV")
		if err != nil {
			return runtime.Undefined, err
		}
		ctx, cancel, err := netContext(options)
		if err != nil {
			return runtime.Undefined, err
		}
		defer cancel()
		cname, records, err := net.DefaultResolver.LookupSRV(ctx, service, proto, host)
		if err != nil {
			return runtime.Undefined, err
		}
		out := make([]*runtime.Value, 0, len(records))
		for _, record := range records {
			out = append(out, runtime.ObjectVal(map[string]*runtime.Value{
				"target":   runtime.StringVal(record.Target),
				"port":     runtime.NumberVal(float64(record.Port)),
				"priority": runtime.NumberVal(float64(record.Priority)),
				"weight":   runtime.NumberVal(float64(record.Weight)),
			}))
		}
		return runtime.ObjectVal(map[string]*runtime.Value{"cname": runtime.StringVal(cname), "records": runtime.ArrayVal(out)}), nil
	})

	add("parseIP", func(args []*runtime.Value) (*runtime.Value, error) {
		s, err := netStringArg(args, 0, "parseIP")
		if err != nil {
			return runtime.Undefined, err
		}
		ip := net.ParseIP(s)
		if ip == nil {
			return runtime.Null, nil
		}
		return runtime.StringVal(ip.String()), nil
	})
	add("isIP", func(args []*runtime.Value) (*runtime.Value, error) {
		s, err := netStringArg(args, 0, "isIP")
		if err != nil {
			return runtime.Undefined, err
		}
		if net.ParseIP(s) == nil {
			return runtime.NumberVal(0), nil
		}
		if strings.Contains(s, ":") {
			return runtime.NumberVal(6), nil
		}
		return runtime.NumberVal(4), nil
	})
	add("joinHostPort", func(args []*runtime.Value) (*runtime.Value, error) {
		host, err := netStringArg(args, 0, "joinHostPort")
		if err != nil {
			return runtime.Undefined, err
		}
		port := netArg(args, 1)
		if port.Tag != runtime.TypeString && port.Tag != runtime.TypeNumber {
			return runtime.Undefined, fmt.Errorf("std.net.joinHostPort: port must be a string or integer")
		}
		portText := port.ToString()
		if port.Tag == runtime.TypeNumber {
			if math.Trunc(port.NumVal) != port.NumVal || port.NumVal < 0 || port.NumVal > 65535 {
				return runtime.Undefined, fmt.Errorf("std.net.joinHostPort: numeric port must be an integer from 0 to 65535")
			}
		}
		if portText == "" {
			return runtime.Undefined, fmt.Errorf("std.net.joinHostPort: port cannot be empty")
		}
		return runtime.StringVal(net.JoinHostPort(host, portText)), nil
	})
	add("splitHostPort", func(args []*runtime.Value) (*runtime.Value, error) {
		address, err := netStringArg(args, 0, "splitHostPort")
		if err != nil {
			return runtime.Undefined, err
		}
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return runtime.Undefined, err
		}
		return runtime.ObjectVal(map[string]*runtime.Value{"host": runtime.StringVal(host), "port": runtime.StringVal(port)}), nil
	})
	add("parseCIDR", func(args []*runtime.Value) (*runtime.Value, error) {
		s, err := netStringArg(args, 0, "parseCIDR")
		if err != nil {
			return runtime.Undefined, err
		}
		ip, network, err := net.ParseCIDR(s)
		if err != nil {
			return runtime.Null, nil
		}
		ones, bits := network.Mask.Size()
		out := map[string]*runtime.Value{
			"ip":      runtime.StringVal(ip.String()),
			"network": runtime.StringVal(network.String()),
			"prefix":  runtime.NumberVal(float64(ones)),
			"bits":    runtime.NumberVal(float64(bits)),
		}
		if ip4 := ip.To4(); ip4 != nil {
			mask := network.Mask
			if len(mask) == net.IPv6len {
				mask = mask[net.IPv6len-net.IPv4len:]
			}
			broadcast := make(net.IP, net.IPv4len)
			for i := range broadcast {
				broadcast[i] = ip4[i] | ^mask[i]
			}
			out["broadcast"] = runtime.StringVal(broadcast.String())
		} else {
			out["broadcast"] = runtime.Null
		}
		return runtime.ObjectVal(out), nil
	})
	add("containsCIDR", func(args []*runtime.Value) (*runtime.Value, error) {
		cidr, err := netStringArg(args, 0, "containsCIDR")
		if err != nil {
			return runtime.Undefined, err
		}
		ipText, err := netStringArg(args, 1, "containsCIDR")
		if err != nil {
			return runtime.Undefined, err
		}
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			return runtime.Undefined, fmt.Errorf("std.net.containsCIDR: invalid CIDR %q", cidr)
		}
		ip := net.ParseIP(ipText)
		return runtime.BoolVal(ip != nil && network.Contains(ip)), nil
	})
	add("isPrivate", func(args []*runtime.Value) (*runtime.Value, error) {
		return netIPPredicate(args, "isPrivate", func(ip net.IP) bool { return ip.IsPrivate() })
	})
	add("isLoopback", func(args []*runtime.Value) (*runtime.Value, error) {
		return netIPPredicate(args, "isLoopback", func(ip net.IP) bool { return ip.IsLoopback() })
	})
	add("isGlobalUnicast", func(args []*runtime.Value) (*runtime.Value, error) {
		return netIPPredicate(args, "isGlobalUnicast", func(ip net.IP) bool { return ip.IsGlobalUnicast() })
	})
	add("isMulticast", func(args []*runtime.Value) (*runtime.Value, error) {
		return netIPPredicate(args, "isMulticast", func(ip net.IP) bool { return ip.IsMulticast() })
	})
	add("isUnspecified", func(args []*runtime.Value) (*runtime.Value, error) {
		return netIPPredicate(args, "isUnspecified", func(ip net.IP) bool { return ip.IsUnspecified() })
	})
	add("isLinkLocalUnicast", func(args []*runtime.Value) (*runtime.Value, error) {
		return netIPPredicate(args, "isLinkLocalUnicast", func(ip net.IP) bool { return ip.IsLinkLocalUnicast() })
	})
	add("isLinkLocalMulticast", func(args []*runtime.Value) (*runtime.Value, error) {
		return netIPPredicate(args, "isLinkLocalMulticast", func(ip net.IP) bool { return ip.IsLinkLocalMulticast() })
	})
	add("to4", func(args []*runtime.Value) (*runtime.Value, error) {
		s, err := netStringArg(args, 0, "to4")
		if err != nil {
			return runtime.Undefined, err
		}
		ip := net.ParseIP(s)
		if ip == nil || ip.To4() == nil {
			return runtime.Null, nil
		}
		return runtime.StringVal(ip.To4().String()), nil
	})
	add("to16", func(args []*runtime.Value) (*runtime.Value, error) {
		s, err := netStringArg(args, 0, "to16")
		if err != nil {
			return runtime.Undefined, err
		}
		ip := net.ParseIP(s)
		if ip == nil {
			return runtime.Null, nil
		}
		if ip4 := ip.To4(); ip4 != nil {
			return runtime.StringVal("::ffff:" + ip4.String()), nil
		}
		return runtime.StringVal(ip.To16().String()), nil
	})

	add("resolveTCPAddr", func(args []*runtime.Value) (*runtime.Value, error) {
		network, err := netStringArg(args, 0, "resolveTCPAddr")
		if err != nil {
			return runtime.Undefined, err
		}
		address, err := netStringArg(args, 1, "resolveTCPAddr")
		if err != nil {
			return runtime.Undefined, err
		}
		if _, err := netNormalizeNetwork(network, "tcp", "tcp4", "tcp6"); err != nil {
			return runtime.Undefined, err
		}
		addr, err := net.ResolveTCPAddr(network, address)
		if err != nil {
			return runtime.Undefined, err
		}
		return netHostPortValue(network, addr), nil
	})
	add("resolveUDPAddr", func(args []*runtime.Value) (*runtime.Value, error) {
		network, err := netStringArg(args, 0, "resolveUDPAddr")
		if err != nil {
			return runtime.Undefined, err
		}
		address, err := netStringArg(args, 1, "resolveUDPAddr")
		if err != nil {
			return runtime.Undefined, err
		}
		if _, err := netNormalizeNetwork(network, "udp", "udp4", "udp6"); err != nil {
			return runtime.Undefined, err
		}
		addr, err := net.ResolveUDPAddr(network, address)
		if err != nil {
			return runtime.Undefined, err
		}
		return netHostPortValue(network, addr), nil
	})
	add("resolveIPAddr", func(args []*runtime.Value) (*runtime.Value, error) {
		network, err := netStringArg(args, 0, "resolveIPAddr")
		if err != nil {
			return runtime.Undefined, err
		}
		address, err := netStringArg(args, 1, "resolveIPAddr")
		if err != nil {
			return runtime.Undefined, err
		}
		if _, err := netNormalizeNetwork(network, "ip", "ip4", "ip6"); err != nil {
			return runtime.Undefined, err
		}
		addr, err := net.ResolveIPAddr(network, address)
		if err != nil {
			return runtime.Undefined, err
		}
		return netHostPortValue(network, addr), nil
	})
	add("hostname", func(args []*runtime.Value) (*runtime.Value, error) {
		host, err := os.Hostname()
		if err != nil {
			return runtime.Undefined, err
		}
		return runtime.StringVal(host), nil
	})
	add("interfaces", func(args []*runtime.Value) (*runtime.Value, error) {
		interfaces, err := net.Interfaces()
		if err != nil {
			return runtime.Undefined, err
		}
		out := make([]*runtime.Value, 0, len(interfaces))
		for _, iface := range interfaces {
			addresses, addrErr := iface.Addrs()
			if addrErr != nil {
				return runtime.Undefined, addrErr
			}
			values := make([]*runtime.Value, 0, len(addresses))
			for _, address := range addresses {
				values = append(values, runtime.StringVal(address.String()))
			}
			out = append(out, runtime.ObjectVal(map[string]*runtime.Value{
				"name":         runtime.StringVal(iface.Name),
				"index":        runtime.NumberVal(float64(iface.Index)),
				"mtu":          runtime.NumberVal(float64(iface.MTU)),
				"flags":        runtime.StringVal(iface.Flags.String()),
				"hardwareAddr": runtime.StringVal(iface.HardwareAddr.String()),
				"addresses":    runtime.ArrayVal(values),
			}))
		}
		return runtime.ArrayVal(out), nil
	})
	add("localIPs", func(args []*runtime.Value) (*runtime.Value, error) {
		interfaces, err := net.Interfaces()
		if err != nil {
			return runtime.Undefined, err
		}
		seen := map[string]bool{}
		out := make([]*runtime.Value, 0)
		for _, iface := range interfaces {
			addresses, addrErr := iface.Addrs()
			if addrErr != nil {
				continue
			}
			for _, address := range addresses {
				var ip net.IP
				switch value := address.(type) {
				case *net.IPNet:
					ip = value.IP
				case *net.IPAddr:
					ip = value.IP
				}
				if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
					continue
				}
				s := ip.String()
				if !seen[s] {
					seen[s] = true
					out = append(out, runtime.StringVal(s))
				}
			}
		}
		return runtime.ArrayVal(out), nil
	})

	mod.ObjVal["reverseLookup"] = mod.ObjVal["lookupAddr"]
	mod.ObjVal["connect"] = mod.ObjVal["dial"]
	mod.ObjVal["connectTLS"] = mod.ObjVal["dialTLS"]
	mod.ObjVal["dialTCP"] = netFunction("dialTCP", func(args []*runtime.Value) (*runtime.Value, error) {
		address, options, err := netAddressArgs(args, "dialTCP")
		if err != nil {
			return runtime.Undefined, err
		}
		return netCallModule(mod, "dial", runtime.StringVal("tcp"), runtime.StringVal(address), options)
	})
	mod.ObjVal["dialUDP"] = netFunction("dialUDP", func(args []*runtime.Value) (*runtime.Value, error) {
		address, options, err := netAddressArgs(args, "dialUDP")
		if err != nil {
			return runtime.Undefined, err
		}
		return netCallModule(mod, "dial", runtime.StringVal("udp"), runtime.StringVal(address), options)
	})
	mod.ObjVal["listenTCP"] = netFunction("listenTCP", func(args []*runtime.Value) (*runtime.Value, error) {
		address, _, err := netAddressArgs(args, "listenTCP")
		if err != nil {
			return runtime.Undefined, err
		}
		return netCallModule(mod, "listen", runtime.StringVal("tcp"), runtime.StringVal(address))
	})
	mod.ObjVal["listenUDP"] = netFunction("listenUDP", func(args []*runtime.Value) (*runtime.Value, error) {
		address, _, err := netAddressArgs(args, "listenUDP")
		if err != nil {
			return runtime.Undefined, err
		}
		return netCallModule(mod, "listenPacket", runtime.StringVal("udp"), runtime.StringVal(address))
	})
	mod.ObjVal["tcpAddr"] = mod.ObjVal["resolveTCPAddr"]
	mod.ObjVal["udpAddr"] = mod.ObjVal["resolveUDPAddr"]
	mod.ObjVal["ipAddr"] = mod.ObjVal["resolveIPAddr"]
	return mod
}

func netAddressArgs(args []*runtime.Value, method string) (string, *runtime.Value, error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("std.net.%s: address or host and port are required", method)
	}
	host, err := netStringArg(args, 0, method)
	if err != nil {
		return "", nil, err
	}
	if len(args) > 1 && netArg(args, 1).Tag == runtime.TypeNumber {
		port, err := netIntArg(args, 1, method, 0, 65535)
		if err != nil {
			return "", nil, err
		}
		var options *runtime.Value
		if len(args) > 2 {
			options = netArg(args, 2)
		}
		return net.JoinHostPort(host, fmt.Sprintf("%d", port)), options, nil
	}
	var options *runtime.Value
	if len(args) > 1 && netArg(args, 1).Tag == runtime.TypeObject {
		options = netArg(args, 1)
	}
	return host, options, nil
}

func netIPPredicate(args []*runtime.Value, name string, predicate func(net.IP) bool) (*runtime.Value, error) {
	s, err := netStringArg(args, 0, name)
	if err != nil {
		return runtime.Undefined, err
	}
	ip := net.ParseIP(s)
	return runtime.BoolVal(ip != nil && predicate(ip)), nil
}

func netCallModule(module *runtime.Value, name string, args ...*runtime.Value) (*runtime.Value, error) {
	fn := module.ObjVal[name]
	if fn == nil || fn.Tag != runtime.TypeFunction {
		return runtime.Undefined, fmt.Errorf("std.net.%s is unavailable", name)
	}
	return runtime.CallFunction(fn, args)
}

func netTLSVersionName(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("0x%04x", version)
	}
}
