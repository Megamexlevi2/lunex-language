package bytecode

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var naxMagic = [8]byte{'L', 'X', 'N', 'A', 'X', 0x02, 0x00, 0x1A}

const naxVersion uint16 = 0x0701
const naxHeaderSize = 40
const naxKind byte = 0xB2

var legacyNAXMagic = [5]byte{'x', '1', '0', '2', 'c'}

const legacyNAXVersion uint16 = 0x0600
const legacyNAXHeaderSize = 37
const legacyNAXKind byte = 0xA1

var ntlNAXMagic = [4]byte{'n', 't', 'l', 'x'}

const ntlNAXVersion uint16 = 0x0500
const ntlNAXHeaderSize = 64

var ntlNAXInternalMagic = [4]byte{0x3d, 0x7e, 0xa1, 0x02}

const ntlNAXInternalVersion uint16 = 0x0603

type NAXEntry struct {
	Name       string
	Kind       byte
	SourceFile string
	SourceText string
	Data       []byte
}

type NAXArchive struct {
	Version   uint16
	BuildTime int64
	Entries   []NAXEntry
	MainIndex uint32
}

func buildNAXHeader(payloadLen, entryCount, mainIndex uint32, digest [16]byte) [naxHeaderSize]byte {
	var hdr [naxHeaderSize]byte
	copy(hdr[0:8], naxMagic[:])
	binary.LittleEndian.PutUint16(hdr[8:10], naxVersion)
	hdr[10] = naxKind
	hdr[11] = 0x00
	binary.LittleEndian.PutUint32(hdr[12:16], payloadLen)
	binary.LittleEndian.PutUint32(hdr[16:20], entryCount)
	binary.LittleEndian.PutUint32(hdr[20:24], mainIndex)
	copy(hdr[24:40], digest[:])
	return hdr
}

func PackDirectory(dir string, outputFile string) error {
	return PackDirectoryMode(dir, outputFile, false)
}

func PackDirectorySource(dir string, outputFile string) error {
	return PackDirectoryMode(dir, outputFile, true)
}

func PackDirectoryMode(dir string, outputFile string, includeSource bool) error {
	arch := &NAXArchive{
		Version:   naxVersion,
		BuildTime: time.Now().Unix(),
	}
	mainFound := false
	absOutput, _ := filepath.Abs(outputFile)

	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		absPath, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if absPath == absOutput {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		base := strings.ToLower(filepath.Base(rel))

		switch strings.ToLower(filepath.Ext(rel)) {
		case ".lx":
			source, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("compile error for %s: %w", rel, err)
			}
			c := newRuntimeCompiler(nil, nil)
			result := c.CompileSource(string(source), path)
			if !result.Success {
				var msgs []string
				for _, e := range result.Errors {
					msgs = append(msgs, e.Message)
				}
				return fmt.Errorf("compile error for %s: %s", rel, strings.Join(msgs, "; "))
			}
			entry, err := NewNAXCompiledEntry(rel, path, string(source), result.AST, includeSource)
			if err != nil {
				return fmt.Errorf("compile error for %s: %w", rel, err)
			}
			arch.Entries = append(arch.Entries, entry)
			if base == "main.lx" {
				arch.MainIndex = uint32(len(arch.Entries) - 1)
				mainFound = true
			}
		case ".nax":
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("cannot read %s: %w", rel, err)
			}
			arch.Entries = append(arch.Entries, NAXEntry{Name: rel, Kind: NAXEntryArchive, Data: data})
			if base == "main.nax" {
				arch.MainIndex = uint32(len(arch.Entries) - 1)
				mainFound = true
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(arch.Entries) == 0 {
		return fmt.Errorf("no source files found in %s", dir)
	}
	if !mainFound {
		return fmt.Errorf("no main.lx or main.nax found in %s\n  a .nax archive requires main.lx as its entry point\n  create main.lx with your program's entry point first", dir)
	}
	raw, err := encodeNAX(arch)
	if err != nil {
		return err
	}
	return os.WriteFile(outputFile, raw, 0644)
}

func PackNAXArchive(arch *NAXArchive, outputFile string) error {
	if arch == nil {
		return fmt.Errorf("archive is nil")
	}
	if arch.BuildTime == 0 {
		arch.BuildTime = time.Now().Unix()
	}
	if arch.Version == 0 {
		arch.Version = naxVersion
	}
	raw, err := encodeNAX(arch)
	if err != nil {
		return err
	}
	return os.WriteFile(outputFile, raw, 0644)
}

func LoadNAX(path string) (*NAXArchive, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return decodeNAX(data)
}

func NAXGetEntry(arch *NAXArchive, name string) ([]byte, bool) {
	for _, e := range arch.Entries {
		if e.Name == name {
			return e.Data, true
		}
	}
	return nil, false
}

func encodeNAX(arch *NAXArchive) ([]byte, error) {
	return encodeNAXGo(arch)
}

func encodeNAXGo(arch *NAXArchive) ([]byte, error) {
	var payload bytes.Buffer
	writeI64(&payload, arch.BuildTime)
	for _, e := range arch.Entries {
		writeString(&payload, e.Name)
		payload.WriteByte(e.Kind)
		writeString(&payload, e.SourceFile)
		if e.Kind == NAXEntryCompiledSource {
			writeString(&payload, e.SourceText)
		}
		writeU32(&payload, uint32(len(e.Data)))
		payload.Write(e.Data)
	}

	plain := payload.Bytes()
	digest := digest16(plain)
	hdr := buildNAXHeader(uint32(len(plain)), uint32(len(arch.Entries)), arch.MainIndex, digest)

	var buf bytes.Buffer
	buf.Write(hdr[:])
	buf.Write(naxScramble(plain))
	return buf.Bytes(), nil
}

func decodeNAX(data []byte) (*NAXArchive, error) {

	if len(data) >= naxHeaderSize && bytes.Equal(data[0:8], naxMagic[:]) {
		return decodeCurrentNAX(data)
	}

	if len(data) >= legacyNAXHeaderSize && bytes.Equal(data[0:5], legacyNAXMagic[:]) {
		if data[7] == legacyNAXKind {
			return decodeLegacyX102NAX(data)
		}
	}

	if len(data) >= ntlNAXHeaderSize && bytes.Equal(data[0:4], ntlNAXMagic[:]) {
		return decodeNTLNAX(data)
	}
	return nil, fmt.Errorf("invalid archive: not a recognised Lunex archive — use 'lunex run <file>.nax'")
}

func decodeCurrentNAX(data []byte) (*NAXArchive, error) {
	if len(data) < naxHeaderSize+8 {
		return nil, fmt.Errorf("invalid archive: file too short")
	}
	ver := binary.LittleEndian.Uint16(data[8:10])
	if ver != naxVersion {
		return nil, fmt.Errorf("invalid archive: version mismatch (got 0x%04x, want 0x%04x) — rebuild with current lunex", ver, naxVersion)
	}
	if data[10] != naxKind {
		return nil, fmt.Errorf("invalid archive: format kind mismatch (got 0x%02x)", data[10])
	}

	payloadLen := binary.LittleEndian.Uint32(data[12:16])
	entryCount := binary.LittleEndian.Uint32(data[16:20])
	mainIndex := binary.LittleEndian.Uint32(data[20:24])
	available := len(data) - naxHeaderSize

	if int(payloadLen) > available {
		return nil, fmt.Errorf("invalid archive: payload truncated (declared %d, have %d)", payloadLen, available)
	}
	if int(payloadLen) != available {
		return nil, fmt.Errorf("invalid archive: trailing bytes or truncated payload")
	}

	scrambled := data[naxHeaderSize : naxHeaderSize+int(payloadLen)]
	plain := naxUnscramble(scrambled)

	if got := digest16(plain); !bytes.Equal(got[:], data[24:40]) {
		return nil, fmt.Errorf("invalid archive: payload integrity check failed — file may be corrupted")
	}

	r := bytes.NewReader(plain)
	buildTime, err := readI64(r)
	if err != nil {
		return nil, fmt.Errorf("invalid archive: cannot read build time: %w", err)
	}

	if entryCount > 0 && mainIndex >= entryCount {
		return nil, fmt.Errorf("invalid archive: main entry index %d is out of range (have %d entries)", mainIndex, entryCount)
	}

	arch := &NAXArchive{
		Version:   ver,
		BuildTime: buildTime,
		MainIndex: mainIndex,
		Entries:   make([]NAXEntry, 0, entryCount),
	}

	for i := uint32(0); i < entryCount; i++ {
		name, err := readString(r)
		if err != nil {
			return nil, fmt.Errorf("invalid archive: entry %d name: %w", i, err)
		}
		kind, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("invalid archive: entry %d kind: %w", i, err)
		}
		sourceFile, err := readString(r)
		if err != nil {
			return nil, fmt.Errorf("invalid archive: entry %d source file: %w", i, err)
		}
		sourceText := ""
		if kind == NAXEntryCompiledSource {
			sourceText, err = readString(r)
			if err != nil {
				return nil, fmt.Errorf("invalid archive: entry %d source: %w", i, err)
			}
		}
		size, err := readU32(r)
		if err != nil {
			return nil, fmt.Errorf("invalid archive: entry %d size: %w", i, err)
		}
		entryData := make([]byte, size)
		if _, err := io.ReadFull(r, entryData); err != nil {
			return nil, fmt.Errorf("invalid archive: entry %d data truncated: %w", i, err)
		}
		arch.Entries = append(arch.Entries, NAXEntry{Name: name, Kind: kind, SourceFile: sourceFile, SourceText: sourceText, Data: entryData})
	}

	return arch, nil
}

func decodeLegacyX102NAX(data []byte) (*NAXArchive, error) {
	if len(data) < legacyNAXHeaderSize+8 {
		return nil, fmt.Errorf("invalid archive: file too short")
	}
	ver := binary.LittleEndian.Uint16(data[5:7])
	if ver != legacyNAXVersion {
		return nil, fmt.Errorf("invalid archive: version mismatch (got 0x%04x)", ver)
	}
	if data[7] != legacyNAXKind {
		return nil, fmt.Errorf("invalid archive: format kind mismatch")
	}

	payloadLen := binary.LittleEndian.Uint32(data[9:13])
	entryCount := binary.LittleEndian.Uint32(data[13:17])
	mainIndex := binary.LittleEndian.Uint32(data[17:21])
	available := len(data) - legacyNAXHeaderSize

	if int(payloadLen) > available {
		return nil, fmt.Errorf("invalid archive: payload truncated")
	}
	if int(payloadLen) != available {
		return nil, fmt.Errorf("invalid archive: trailing bytes or truncated payload")
	}

	plain := naxUnscramble(data[legacyNAXHeaderSize : legacyNAXHeaderSize+int(payloadLen)])
	if got := digest16(plain); !bytes.Equal(got[:], data[21:37]) {
		return nil, fmt.Errorf("invalid archive: payload checksum mismatch")
	}

	r := bytes.NewReader(plain)
	buildTime, err := readI64(r)
	if err != nil {
		return nil, err
	}

	if entryCount > 0 && mainIndex >= entryCount {
		return nil, fmt.Errorf("invalid archive: main entry index out of range")
	}

	arch := &NAXArchive{
		Version:   ver,
		BuildTime: buildTime,
		MainIndex: mainIndex,
		Entries:   make([]NAXEntry, 0, entryCount),
	}

	for i := uint32(0); i < entryCount; i++ {
		name, err := readString(r)
		if err != nil {
			return nil, err
		}
		size, err := readU32(r)
		if err != nil {
			return nil, err
		}
		entryData := make([]byte, size)
		if _, err := io.ReadFull(r, entryData); err != nil {
			return nil, err
		}
		arch.Entries = append(arch.Entries, NAXEntry{Name: name, Data: entryData})
	}

	return arch, nil
}

func decodeNTLNAX(data []byte) (*NAXArchive, error) {
	if len(data) < ntlNAXHeaderSize+8 {
		return nil, fmt.Errorf("invalid archive: file too short")
	}

	ver := binary.LittleEndian.Uint16(data[4:6])
	if ver != ntlNAXVersion {
		return nil, fmt.Errorf("invalid archive: version mismatch (got 0x%04x)", ver)
	}

	payload := naxUnscramble(data[ntlNAXHeaderSize:])
	if len(payload) < 8 {
		return nil, fmt.Errorf("invalid archive: corrupt payload")
	}

	var magic [4]byte
	copy(magic[:], payload[:4])
	if magic != ntlNAXInternalMagic {
		return nil, fmt.Errorf("invalid archive: bad inner signature")
	}

	innerVer := binary.LittleEndian.Uint16(payload[4:6])
	if innerVer != ntlNAXInternalVersion {
		return nil, fmt.Errorf("invalid archive: inner version mismatch")
	}

	r := bytes.NewReader(payload[8:])

	buildTime, err := readI64(r)
	if err != nil {
		return nil, err
	}
	mainIndex, err := readU32(r)
	if err != nil {
		return nil, err
	}
	count, err := readU32(r)
	if err != nil {
		return nil, err
	}
	if count > 0 && mainIndex >= count {
		return nil, fmt.Errorf("invalid archive: main entry index out of range")
	}

	arch := &NAXArchive{
		Version:   innerVer,
		BuildTime: buildTime,
		MainIndex: mainIndex,
	}

	for i := uint32(0); i < count; i++ {
		name, err := readString(r)
		if err != nil {
			return nil, err
		}
		size, err := readU32(r)
		if err != nil {
			return nil, err
		}
		entryData := make([]byte, size)
		if _, err := io.ReadFull(r, entryData); err != nil {
			return nil, err
		}
		arch.Entries = append(arch.Entries, NAXEntry{Name: name, Data: entryData})
	}
	return arch, nil
}

var naxKey = []byte{
	0x9b, 0xe4, 0x2c, 0x71, 0xd3, 0x5a, 0x08, 0xf6,
	0x47, 0xbc, 0x1e, 0x83, 0x60, 0xad, 0x35, 0x7f,
	0xc2, 0x58, 0x0d, 0xea, 0x91, 0x3c, 0x76, 0xb4,
	0x2a, 0x69, 0xf0, 0x14, 0xa7, 0x5e, 0xcd, 0x42,
	0x88, 0x1b, 0x64, 0xd9, 0x37, 0xfc, 0x55, 0xa0,
	0x0c, 0x73, 0xbe, 0x29, 0x8d, 0xe1, 0x4f, 0x96,
	0x23, 0xda, 0x61, 0xb7, 0x3e, 0x80, 0xc5, 0x1d,
	0x52, 0xaf, 0x04, 0x99, 0x6b, 0xd8, 0x20, 0x7c,
}

func naxScramble(data []byte) []byte {
	out := make([]byte, len(data))
	kl := len(naxKey)
	for i, b := range data {
		ki := i % kl
		prev := byte(0)
		if i > 0 {
			prev = out[i-1]
		}
		out[i] = ((b ^ naxKey[ki]) + byte(i*7+ki*3)) ^ prev
	}
	return out
}

func naxUnscramble(data []byte) []byte {
	out := make([]byte, len(data))
	kl := len(naxKey)
	for i, b := range data {
		ki := i % kl
		prev := byte(0)
		if i > 0 {
			prev = data[i-1]
		}
		out[i] = ((b ^ prev) - byte(i*7+ki*3)) ^ naxKey[ki]
	}
	return out
}

func unpackNAXData(data []byte, outDir string) (int, error) {
	arch, err := decodeNAX(data)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, entry := range arch.Entries {
		if entry.Name == "" {
			continue
		}
		name := entry.Name
		content := entry.Data
		if IsNAXCompiledEntry(entry) {
			recovered, err := RecoverNAXEntry(entry)
			if err != nil {
				return count, fmt.Errorf("recovering %q: %w", entry.Name, err)
			}
			name = strings.TrimSuffix(name, filepath.Ext(name)) + ".lx"
			content = []byte(recovered)
		} else if entry.Kind == NAXEntrySource {
			content = entry.Data
		} else if entry.Kind == NAXEntryArchive {
			continue
		}
		clean := filepath.Clean(filepath.Join(outDir, filepath.FromSlash(name)))
		if !strings.HasPrefix(clean, outDir+string(filepath.Separator)) {
			return count, fmt.Errorf("archive entry %q has an unsafe path — skipping", entry.Name)
		}
		if err := os.MkdirAll(filepath.Dir(clean), 0755); err != nil {
			return count, fmt.Errorf("creating directory for %q: %w", name, err)
		}
		if err := os.WriteFile(clean, content, 0644); err != nil {
			return count, fmt.Errorf("writing %q: %w", name, err)
		}
		if arch.BuildTime > 0 {
			mtime := time.Unix(arch.BuildTime, 0)
			_ = os.Chtimes(clean, mtime, mtime)
		}
		count++
	}
	return count, nil
}

func recoverNAXData(data []byte, outDir string) (int, error) {
	return unpackNAXData(data, outDir)
}

func ExtractNAXEntry(data []byte) ([]byte, error) {
	arch, err := decodeNAX(data)
	if err != nil {
		return nil, fmt.Errorf("cannot decode .nax: %w", err)
	}
	if len(arch.Entries) == 0 {
		return nil, fmt.Errorf("entry not found in archive: <empty>")
	}
	idx := int(arch.MainIndex)
	if idx < 0 || idx >= len(arch.Entries) {
		idx = 0
	}
	return arch.Entries[idx].Data, nil
}
