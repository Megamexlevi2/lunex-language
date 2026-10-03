package errfmt

import (
	"strings"
	"testing"
)

func TestSuggestionsUseLunexSyntax(t *testing.T) {
	cases := []struct {
		code string
		want []string
		bad  []string
	}{
		{"E0002", []string{"typeof(value)"}, []string{"@inspect", "@typeOf", "@debug"}},
		{"E0020", []string{"typeof(value)"}, []string{"@typeOf"}},
		{"E0061", []string{"io.log(value)"}, []string{"@debug"}},
		{"S0001", []string{"typeof(value)"}, []string{"@typeOf"}},
		{"S0003", []string{"typeof(x)"}, []string{"@typeOf"}},
		{"E0024", []string{"typeof(x)"}, []string{"@typeOf"}},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			got := strings.Join(buildSuggestions(tc.code, "", "", nil), "\n")
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Fatalf("suggestions do not contain %q: %s", want, got)
				}
			}
			for _, bad := range tc.bad {
				if strings.Contains(got, bad) {
					t.Fatalf("suggestions contain invalid syntax %q: %s", bad, got)
				}
			}
		})
	}
}
