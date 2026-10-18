package std

import (
	"strings"
	"testing"
	"time"

	"lunex/internal/runtime"
)

func callNet(t *testing.T, value *runtime.Value, name string, args ...*runtime.Value) *runtime.Value {
	t.Helper()
	fn := value.ObjVal[name]
	if fn == nil || fn.Tag != runtime.TypeFunction {
		t.Fatalf("missing net.%s", name)
	}
	result, err := runtime.CallFunction(fn, args)
	if err != nil {
		t.Fatalf("net.%s: %v", name, err)
	}
	return result
}

func callNetMethod(t *testing.T, value *runtime.Value, name string, args ...*runtime.Value) *runtime.Value {
	t.Helper()
	return callNet(t, value, name, args...)
}

func TestNetIPUtilities(t *testing.T) {
	module := NetModule()
	if got := callNet(t, module, "parseIP", runtime.StringVal("127.0.0.1")).StrVal; got != "127.0.0.1" {
		t.Fatalf("parseIP returned %q", got)
	}
	if got := callNet(t, module, "parseIP", runtime.StringVal("not-an-ip")); got != runtime.Null {
		t.Fatalf("invalid parseIP should return null, got %#v", got)
	}
	if got := callNet(t, module, "isIP", runtime.StringVal("2001:db8::1")).NumVal; got != 6 {
		t.Fatalf("isIP returned %v, want 6", got)
	}
	if got := callNet(t, module, "isIP", runtime.StringVal("not-an-ip")).NumVal; got != 0 {
		t.Fatalf("isIP returned %v, want 0", got)
	}
	joined := callNet(t, module, "joinHostPort", runtime.StringVal("::1"), runtime.NumberVal(443)).StrVal
	if joined != "[::1]:443" {
		t.Fatalf("joinHostPort returned %q", joined)
	}
	split := callNet(t, module, "splitHostPort", runtime.StringVal("[::1]:443"))
	if split.ObjVal["host"].StrVal != "::1" || split.ObjVal["port"].StrVal != "443" {
		t.Fatalf("unexpected splitHostPort result: %#v", split.ObjVal)
	}
	cidr := callNet(t, module, "parseCIDR", runtime.StringVal("192.168.10.14/24"))
	if cidr == runtime.Null || cidr.ObjVal["network"].StrVal != "192.168.10.0/24" || cidr.ObjVal["broadcast"].StrVal != "192.168.10.255" {
		t.Fatalf("unexpected parseCIDR result: %#v", cidr)
	}
	mappedCIDR := callNet(t, module, "parseCIDR", runtime.StringVal("::ffff:192.0.2.1/120"))
	if mappedCIDR == runtime.Null || mappedCIDR.ObjVal["broadcast"].StrVal != "192.0.2.255" {
		t.Fatalf("unexpected IPv4-mapped CIDR result: %#v", mappedCIDR)
	}
	if !callNet(t, module, "containsCIDR", runtime.StringVal("192.168.10.0/24"), runtime.StringVal("192.168.10.20")).BoolVal {
		t.Fatal("containsCIDR should match a member address")
	}
	if callNet(t, module, "containsCIDR", runtime.StringVal("192.168.10.0/24"), runtime.StringVal("192.168.11.20")).BoolVal {
		t.Fatal("containsCIDR matched an address outside the network")
	}
	if !callNet(t, module, "isPrivate", runtime.StringVal("10.2.3.4")).BoolVal {
		t.Fatal("isPrivate rejected a private address")
	}
	if !callNet(t, module, "isLoopback", runtime.StringVal("::1")).BoolVal {
		t.Fatal("isLoopback rejected IPv6 loopback")
	}
	if got := callNet(t, module, "to4", runtime.StringVal("::ffff:192.0.2.1")); got.Tag != runtime.TypeString || got.StrVal != "192.0.2.1" {
		t.Fatalf("to4 returned %#v, want 192.0.2.1", got)
	}
	if got := callNet(t, module, "to16", runtime.StringVal("127.0.0.1")); got.Tag != runtime.TypeString || got.StrVal != "::ffff:127.0.0.1" {
		t.Fatalf("to16 returned %#v, want ::ffff:127.0.0.1", got)
	}
	if got := callNet(t, module, "to16", runtime.StringVal("2001:db8::1")); got.Tag != runtime.TypeString || got.StrVal != "2001:db8::1" {
		t.Fatalf("to16 changed native IPv6 address: %#v", got)
	}
	if got := callNet(t, module, "to16", runtime.StringVal("not-an-ip")); got != runtime.Null {
		t.Fatalf("to16 accepted invalid IP: %#v", got)
	}
}

func TestNetTCPConnectionLifecycle(t *testing.T) {
	module := NetModule()
	listener := callNet(t, module, "listenTCP", runtime.StringVal("127.0.0.1"), runtime.NumberVal(0))
	address := callNetMethod(t, listener, "address").StrVal
	accepted := make(chan *runtime.Value, 1)
	acceptErr := make(chan error, 1)
	go func() {
		fn := listener.ObjVal["accept"]
		value, err := runtime.CallFunction(fn, nil)
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- value
	}()

	client, err := runtime.CallFunction(module.ObjVal["dialTCP"], []*runtime.Value{runtime.StringVal(address), runtime.ObjectVal(map[string]*runtime.Value{"timeoutMs": runtime.NumberVal(2000)})})
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer callNetMethod(t, client, "close")
	var server *runtime.Value
	select {
	case server = <-accepted:
	case err := <-acceptErr:
		t.Fatalf("accept failed: %v", err)
	case <-time.After(2 * time.Second):
		t.Fatal("accept timed out")
	}
	defer callNetMethod(t, server, "close")
	callNetMethod(t, client, "setDeadline", runtime.NumberVal(2000))
	callNetMethod(t, server, "setDeadline", runtime.NumberVal(2000))
	written := callNetMethod(t, client, "write", runtime.StringVal("lunex-net"))
	if written.NumVal != float64(len("lunex-net")) {
		t.Fatalf("write returned %v", written.NumVal)
	}
	read := callNetMethod(t, server, "readFull", runtime.NumberVal(float64(len("lunex-net"))))
	if read.StrVal != "lunex-net" {
		t.Fatalf("readFull returned %q", read.StrVal)
	}
	if callNetMethod(t, client, "remoteAddr").StrVal == "" || callNetMethod(t, server, "localAddr").StrVal == "" {
		t.Fatal("address methods returned empty values")
	}
	callNetMethod(t, client, "close")
	if !callNetMethod(t, client, "isClosed").BoolVal {
		t.Fatal("connection should report closed")
	}
	callNetMethod(t, listener, "close")
	if !callNetMethod(t, listener, "isClosed").BoolVal {
		t.Fatal("listener should report closed")
	}
}

func TestNetUDPDatagrams(t *testing.T) {
	module := NetModule()
	server := callNet(t, module, "listenUDP", runtime.StringVal("127.0.0.1"), runtime.NumberVal(0))
	address := callNetMethod(t, server, "localAddr").StrVal
	client, err := runtime.CallFunction(module.ObjVal["dialUDP"], []*runtime.Value{runtime.StringVal(address), runtime.ObjectVal(map[string]*runtime.Value{"timeoutMs": runtime.NumberVal(2000)})})
	if err != nil {
		t.Fatalf("UDP dial failed: %v", err)
	}
	defer callNetMethod(t, client, "close")
	callNetMethod(t, server, "setDeadline", runtime.NumberVal(2000))
	callNetMethod(t, client, "setDeadline", runtime.NumberVal(2000))
	if got := callNetMethod(t, client, "write", runtime.StringVal("datagram")).NumVal; got != 8 {
		t.Fatalf("UDP write returned %v", got)
	}
	result := callNetMethod(t, server, "readFrom", runtime.NumberVal(128))
	if result.ObjVal["data"].StrVal != "datagram" || result.ObjVal["bytesRead"].NumVal != 8 || result.ObjVal["port"].NumVal <= 0 {
		t.Fatalf("unexpected UDP read result: %#v", result.ObjVal)
	}
	if got := callNetMethod(t, client, "writeBytes", runtime.ArrayVal([]*runtime.Value{runtime.NumberVal(0), runtime.NumberVal(1), runtime.NumberVal(255)})).NumVal; got != 3 {
		t.Fatalf("UDP byte write returned %v", got)
	}
	binary := callNetMethod(t, server, "readBytesFrom")
	if binary.ObjVal["bytesRead"].NumVal != 3 || binary.ObjVal["data"].ArrVal[0].NumVal != 0 || binary.ObjVal["data"].ArrVal[1].NumVal != 1 || binary.ObjVal["data"].ArrVal[2].NumVal != 255 {
		t.Fatalf("unexpected binary UDP result: %#v", binary.ObjVal)
	}
	callNetMethod(t, server, "close")
	if !callNetMethod(t, server, "isClosed").BoolVal {
		t.Fatal("packet connection should report closed")
	}
}

func TestNetDNSAndInterfaces(t *testing.T) {
	module := NetModule()
	ips := callNet(t, module, "lookupIP", runtime.StringVal("localhost"), runtime.ObjectVal(map[string]*runtime.Value{"timeoutMs": runtime.NumberVal(2000)}))
	if len(ips.ArrVal) == 0 {
		t.Fatal("localhost should resolve to at least one IP address")
	}
	for _, item := range ips.ArrVal {
		if netIP := callNet(t, module, "parseIP", item); netIP == runtime.Null {
			t.Fatalf("lookupIP returned invalid address %q", item.StrVal)
		}
	}
	if got := callNet(t, module, "lookupPort", runtime.StringVal("tcp"), runtime.StringVal("http")).NumVal; got != 80 {
		t.Fatalf("lookupPort returned %v, want 80", got)
	}
	if got := callNet(t, module, "hostname"); got.Tag != runtime.TypeString || strings.TrimSpace(got.StrVal) == "" {
		t.Fatalf("hostname returned invalid value: %#v", got)
	}
	interfaces := callNet(t, module, "interfaces")
	if interfaces.Tag != runtime.TypeArray || len(interfaces.ArrVal) == 0 {
		t.Fatal("interfaces should return a non-empty array")
	}
}

func TestNetReadDeadline(t *testing.T) {
	module := NetModule()
	listener := callNet(t, module, "listenTCP", runtime.StringVal("127.0.0.1"), runtime.NumberVal(0))
	address := callNetMethod(t, listener, "address").StrVal
	client, err := runtime.CallFunction(module.ObjVal["dialTCP"], []*runtime.Value{runtime.StringVal(address)})
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer callNetMethod(t, client, "close")
	server := callNetMethod(t, listener, "accept")
	defer callNetMethod(t, server, "close")
	callNetMethod(t, server, "setReadDeadline", runtime.NumberVal(20))
	if _, err := runtime.CallFunction(server.ObjVal["readFull"], []*runtime.Value{runtime.NumberVal(1)}); err == nil {
		t.Fatal("readFull did not time out without incoming bytes")
	}
	callNetMethod(t, listener, "close")
}

func TestNetRejectsInvalidArguments(t *testing.T) {
	module := NetModule()
	if _, err := runtime.CallFunction(module.ObjVal["listen"], []*runtime.Value{runtime.StringVal("bad-network"), runtime.StringVal("127.0.0.1:0")}); err == nil {
		t.Fatal("listen accepted an invalid network")
	}
	if _, err := runtime.CallFunction(module.ObjVal["joinHostPort"], []*runtime.Value{runtime.StringVal("host"), runtime.NumberVal(70000)}); err == nil {
		t.Fatal("joinHostPort accepted an out-of-range numeric port")
	}
	if _, err := runtime.CallFunction(module.ObjVal["dial"], []*runtime.Value{runtime.StringVal("tcp"), runtime.StringVal("127.0.0.1:1"), runtime.StringVal("invalid-options")}); err == nil {
		t.Fatal("dial accepted a non-object options argument")
	}
	if _, err := runtime.CallFunction(module.ObjVal["dialTLS"], []*runtime.Value{runtime.StringVal("tcp"), runtime.StringVal("127.0.0.1:1"), runtime.ObjectVal(map[string]*runtime.Value{"insecureSkipVerify": runtime.StringVal("yes")})}); err == nil {
		t.Fatal("dialTLS accepted a non-boolean insecureSkipVerify option")
	}
	if _, err := runtime.CallFunction(module.ObjVal["dialTLS"], []*runtime.Value{runtime.StringVal("tcp"), runtime.StringVal("127.0.0.1:1"), runtime.ObjectVal(map[string]*runtime.Value{"clientCertFile": runtime.StringVal("client.crt")})}); err == nil {
		t.Fatal("dialTLS accepted a client certificate without its private key")
	}
	if _, err := netBytesArg(runtime.ArrayVal([]*runtime.Value{runtime.NumberVal(256)}), "writeBytes"); err == nil {
		t.Fatal("writeBytes accepted an invalid byte")
	}
}
