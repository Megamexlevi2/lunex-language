package runtime

import (
	"crypto/sha256"
	"fmt"
)

const (
	NTLAuthor    = "David Dev"
	NTLGitHub    = "https://github.com/Megamexlevi2"
	NTLCopyright = "(c) David Dev 2026"
	NTLLicense   = "Mozilla Public License, Version 2.0 — https://mozilla.org/MPL/2.0/"
)

var AuthorFingerprint = buildFingerprint()

func buildFingerprint() string {
	h := sha256.Sum256([]byte(NTLAuthor + "|" + NTLGitHub + "|" + NTLCopyright))
	return fmt.Sprintf("lunex-fp:%x", h[:8])
}

func WatermarkHeader() []byte {
	fp := AuthorFingerprint
	return []byte(fmt.Sprintf(
		"#!lunex-bytecode\n#author:%s\n#github:%s\n#fp:%s\n",
		NTLAuthor, NTLGitHub, fp,
	))
}

func VerifyWatermark(data []byte) bool {
	return len(data) > 15 && string(data[:14]) == "#!lunex-bytecode"
}

func AttributionBanner() string {
	return fmt.Sprintf(
		"Lunex Language Runtime\n%s  %s\nFingerprint: %s\n%s",
		NTLCopyright, NTLGitHub, AuthorFingerprint, NTLLicense,
	)
}
