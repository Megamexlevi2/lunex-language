package checker

import (
	"os"
	"path/filepath"
	"testing"
)

func checkTrySource(source string) Result {
	return New(Options{}).Check(source, "try_checker_test.lx")
}

func TestCatchBindingIsAvailableInsideCatch(t *testing.T) {
	source := `fn main() {
  try {
    throw "failure"
  } catch (err) {
    val message = err
  }
}`
	result := checkTrySource(source)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
}

func TestCatchBindingDoesNotEscapeCatch(t *testing.T) {
	source := `fn main() {
  try {
    throw "failure"
  } catch (err) {
    val message = err
  }
  val outside = err
}`
	result := checkTrySource(source)
	found := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "E0001" && diagnostic.Message == "variable `err` was not defined" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected out-of-scope catch binding diagnostic, got %#v", result.Diagnostics)
	}
}

func TestCatchBindingCanBeReassigned(t *testing.T) {
	source := `fn main() {
  try {
    throw "failure"
  } catch (err) {
    err = "changed"
  }
}`
	result := checkTrySource(source)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
}

func TestNetStandardLibraryFixturesPassChecker(t *testing.T) {
	for _, name := range []string{
		"01_net_ip.lx",
		"02_net_tcp.lx",
		"03_net_udp.lx",
		"04_net_dns.lx",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "tests", "stdlib", "net", name)
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			result := New(Options{}).Check(string(source), path)
			if len(result.Diagnostics) != 0 {
				t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
			}
		})
	}
}
