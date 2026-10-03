package bytecode

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"lunex/internal/ast"
)

var useSelfHosted = false

func SetSelfHosted(_ bool) {}

var x102Magic = [5]byte{'x', '1', '0', '2', 'c'}

const x96Version uint16 = 0x0600
const x102HeaderSize = 32
const x102FlagSource = byte(0x01)
const x102FlagAST = byte(0x04)

var legacyObjectMagic = [4]byte{'n', 't', 'l', 'i'}

const legacyObjectVersion uint16 = 0x0500
const legacyObjectHeaderSize = 48

var legacyInternalMagic = [4]byte{0x1c, 0x9a, 0x4e, 0x03}

const legacyInternalVersion uint16 = 0x0603

func buildX102Header(payloadLen uint32, ntzLen uint32, digest [16]byte) [x102HeaderSize]byte {
	var hdr [x102HeaderSize]byte
	copy(hdr[0:5], x102Magic[:])
	binary.LittleEndian.PutUint16(hdr[5:7], x96Version)
	hdr[7] = 0x01
	if ntzLen > 0 {
		hdr[7] |= 0x02
	}
	hdr[8] = 0x00
	binary.LittleEndian.PutUint32(hdr[8:12], payloadLen)
	binary.LittleEndian.PutUint32(hdr[12:16], ntzLen)
	copy(hdr[16:32], digest[:])
	return hdr
}

func EncodeObject(chunk *Chunk) ([]byte, error) {
	return EncodeObjectWithNTZ(chunk, nil)
}

func EncodeObjectWithNTZ(chunk *Chunk, ntzOpcodes []byte) ([]byte, error) {
	return encodeNCWithNTZ(chunk, ntzOpcodes)
}

func encodeNCWithNTZ(chunk *Chunk, ntzOpcodes []byte) ([]byte, error) {
	var payload bytes.Buffer
	writeString(&payload, chunk.Name)
	writeString(&payload, chunk.SourceFile)
	writeString(&payload, chunk.SourceText)
	writeU32(&payload, uint32(len(chunk.SubChunks)))
	for _, sub := range chunk.SubChunks {
		writeString(&payload, sub.Name)
		writeString(&payload, sub.SourceFile)
		writeString(&payload, sub.SourceText)
	}

	plain := payload.Bytes()
	digest := digest16(plain)
	hdr := buildX102Header(uint32(len(plain)), uint32(len(ntzOpcodes)), digest)

	var buf bytes.Buffer
	buf.Write(hdr[:])
	buf.Write(xorScramble(plain, objectKey))
	if len(ntzOpcodes) > 0 {
		buf.Write(ntzOpcodes)
	}
	return buf.Bytes(), nil
}

func DecodeObject(data []byte) (*Chunk, error) {
	if len(data) >= x102HeaderSize && bytes.Equal(data[0:5], x102Magic[:]) {
		return decodeX96Object(data)
	}
	if len(data) >= legacyObjectHeaderSize && bytes.Equal(data[0:4], legacyObjectMagic[:]) {
		return decodeLegacyObject(data)
	}
	return nil, fmt.Errorf("invalid object: not a recognized Lunex format")
}

func decodeX96Object(data []byte) (*Chunk, error) {
	if len(data) < x102HeaderSize+8 {
		return nil, fmt.Errorf("invalid object: file too short")
	}

	ver := binary.LittleEndian.Uint16(data[5:7])
	if ver != x96Version {
		return nil, fmt.Errorf("invalid object: version mismatch (got 0x%04x)", ver)
	}

	payloadLen := binary.LittleEndian.Uint32(data[8:12])
	ntzLen := binary.LittleEndian.Uint32(data[12:16])
	available := len(data) - x102HeaderSize
	if payloadLen > uint32(available) {
		return nil, fmt.Errorf("invalid object: payload truncated")
	}
	if ntzLen > uint32(available)-payloadLen {
		return nil, fmt.Errorf("invalid object: NTZ section length is invalid")
	}
	if int(payloadLen)+int(ntzLen) != available {
		return nil, fmt.Errorf("invalid object: trailing bytes or truncated payload")
	}

	encPayload := data[x102HeaderSize : x102HeaderSize+int(payloadLen)]
	plain := xorUnscramble(encPayload, objectKey)
	got := digest16(plain)
	if !bytes.Equal(got[:], data[16:32]) {
		return nil, fmt.Errorf("invalid object: payload checksum mismatch")
	}

	r := bytes.NewReader(plain)
	name, err := readString(r)
	if err != nil {
		return nil, err
	}
	srcFile, err := readString(r)
	if err != nil {
		return nil, err
	}
	srcText, err := readString(r)
	if err != nil {
		return nil, err
	}
	flags := data[7]
	var tree *ast.Node
	if flags&x102FlagAST != 0 {
		astLen, err := readU32(r)
		if err != nil {
			return nil, err
		}
		astData := make([]byte, astLen)
		if _, err := io.ReadFull(r, astData); err != nil {
			return nil, err
		}
		tree, err = decodeAST(astData)
		if err != nil {
			return nil, fmt.Errorf("invalid object: AST section decode failed: %w", err)
		}
	}
	subCount, err := readU32(r)
	if err != nil {
		return nil, err
	}

	chunk := &Chunk{
		Name:       name,
		SourceFile: srcFile,
		SourceText: srcText,
		AST:        tree,
		SubChunks:  make([]*Chunk, 0, subCount),
	}

	for i := uint32(0); i < subCount; i++ {
		sName, err := readString(r)
		if err != nil {
			return nil, err
		}
		sSrc, err := readString(r)
		if err != nil {
			return nil, err
		}
		sText, err := readString(r)
		if err != nil {
			return nil, err
		}
		chunk.SubChunks = append(chunk.SubChunks, &Chunk{
			Name:       sName,
			SourceFile: sSrc,
			SourceText: sText,
		})
	}

	return chunk, nil
}

func decodeLegacyObject(data []byte) (*Chunk, error) {
	if len(data) < legacyObjectHeaderSize+8 {
		return nil, fmt.Errorf("invalid object: file too short")
	}
	if data[0] != 'n' || data[1] != 't' || data[2] != 'l' || data[3] != 'i' {
		return nil, fmt.Errorf("invalid object: not a recognized Lunex format")
	}

	ver := binary.LittleEndian.Uint16(data[5:7])
	if ver != legacyObjectVersion {
		return nil, fmt.Errorf("invalid object: version mismatch (got 0x%04x)", ver)
	}

	ntzLen := binary.LittleEndian.Uint32(data[40:44])
	payloadEnd := len(data)
	if ntzLen > 0 {
		if int(ntzLen) > payloadEnd-legacyObjectHeaderSize {
			return nil, fmt.Errorf("invalid object: NTZ section length is invalid")
		}
		payloadEnd -= int(ntzLen)
	}
	if payloadEnd < legacyObjectHeaderSize {
		return nil, fmt.Errorf("invalid object: payload truncated")
	}

	payload := xorUnscramble(data[legacyObjectHeaderSize:payloadEnd], objectKey)
	if len(payload) < 8 {
		return nil, fmt.Errorf("invalid object: corrupt payload")
	}

	var magic [4]byte
	copy(magic[:], payload[:4])
	if magic != legacyInternalMagic {
		return nil, fmt.Errorf("invalid object: bad inner signature")
	}

	innerVer := binary.LittleEndian.Uint16(payload[4:6])
	if innerVer != legacyInternalVersion {
		return nil, fmt.Errorf("invalid object: inner version mismatch (got 0x%04x)", innerVer)
	}

	r := bytes.NewReader(payload[8:])
	name, err := readString(r)
	if err != nil {
		return nil, err
	}
	srcFile, err := readString(r)
	if err != nil {
		return nil, err
	}
	srcText, err := readString(r)
	if err != nil {
		return nil, err
	}
	subCount, err := readU32(r)
	if err != nil {
		return nil, err
	}

	chunk := &Chunk{
		Name:       name,
		SourceFile: srcFile,
		SourceText: srcText,
		SubChunks:  make([]*Chunk, 0, subCount),
	}

	for i := uint32(0); i < subCount; i++ {
		sName, err := readString(r)
		if err != nil {
			return nil, err
		}
		sSrc, err := readString(r)
		if err != nil {
			return nil, err
		}
		sText, err := readString(r)
		if err != nil {
			return nil, err
		}
		chunk.SubChunks = append(chunk.SubChunks, &Chunk{
			Name:       sName,
			SourceFile: sSrc,
			SourceText: sText,
		})
	}

	return chunk, nil
}

func NTZSection(data []byte) []byte {
	if len(data) < 4 {
		return nil
	}
	if bytes.Equal(data[0:5], x102Magic[:]) {
		if len(data) < x102HeaderSize {
			return nil
		}
		payloadLen := binary.LittleEndian.Uint32(data[8:12])
		ntzLen := binary.LittleEndian.Uint32(data[12:16])
		if ntzLen == 0 || int(payloadLen)+int(ntzLen) > len(data)-x102HeaderSize {
			return nil
		}
		return data[len(data)-int(ntzLen):]
	}
	if len(data) < legacyObjectHeaderSize || !bytes.Equal(data[0:4], legacyObjectMagic[:]) {
		return nil
	}
	ntzLen := binary.LittleEndian.Uint32(data[40:44])
	if ntzLen == 0 || int(ntzLen) > len(data)-legacyObjectHeaderSize {
		return nil
	}
	return data[len(data)-int(ntzLen):]
}

var objectKey = []byte{
	0xc7, 0x3b, 0xa2, 0x58, 0xf1, 0x0d, 0x6e, 0x94,
	0x27, 0xbb, 0x43, 0xe0, 0x7c, 0x15, 0xd8, 0xa9,
	0x52, 0xfe, 0x30, 0x8d, 0x61, 0x1a, 0xc4, 0x77,
	0xeb, 0x09, 0x56, 0xf3, 0xae, 0x2b, 0x84, 0x5d,
	0x13, 0x98, 0xe7, 0x4c, 0xb0, 0x2f, 0x71, 0xda,
	0x3e, 0x95, 0xc0, 0x68, 0xf7, 0x19, 0x82, 0x4b,
	0xa4, 0x5e, 0x07, 0xcd, 0x36, 0xb9, 0x7f, 0xe2,
	0x0c, 0xd5, 0x88, 0x41, 0xfa, 0x23, 0x6a, 0x9e,
}

func xorScramble(data, key []byte) []byte {
	out := make([]byte, len(data))
	kl := len(key)
	for i, b := range data {
		ki := i % kl
		prev := byte(0)
		if i > 0 {
			prev = out[i-1]
		}
		out[i] = (b ^ key[ki] ^ byte(i*3+ki*7)) + prev&0x1f
	}
	return out
}

func xorUnscramble(data, key []byte) []byte {
	out := make([]byte, len(data))
	kl := len(key)
	for i, b := range data {
		ki := i % kl
		prev := byte(0)
		if i > 0 {
			prev = data[i-1]
		}
		out[i] = (b - prev&0x1f) ^ key[ki] ^ byte(i*3+ki*7)
	}
	return out
}

func writeU16(w *bytes.Buffer, v uint16) {
	b := [2]byte{}
	binary.LittleEndian.PutUint16(b[:], v)
	w.Write(b[:])
}

func writeU32(w *bytes.Buffer, v uint32) {
	b := [4]byte{}
	binary.LittleEndian.PutUint32(b[:], v)
	w.Write(b[:])
}

func writeString(w *bytes.Buffer, s string) {
	writeU32(w, uint32(len(s)))
	w.WriteString(s)
}

func readU16(r *bytes.Reader) (uint16, error) {
	b := [2]byte{}
	if _, err := r.Read(b[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint16(b[:]), nil
}

func readU32(r *bytes.Reader) (uint32, error) {
	b := [4]byte{}
	if _, err := r.Read(b[:]); err != nil {
		return 0, err
	}
	return binary.LittleEndian.Uint32(b[:]), nil
}

func writeI64(w *bytes.Buffer, v int64) {
	b := [8]byte{}
	binary.LittleEndian.PutUint64(b[:], uint64(v))
	w.Write(b[:])
}

func readI64(r *bytes.Reader) (int64, error) {
	b := [8]byte{}
	if _, err := r.Read(b[:]); err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint64(b[:])), nil
}

func readString(r *bytes.Reader) (string, error) {
	ln, err := readU32(r)
	if err != nil {
		return "", err
	}
	if ln == 0 {
		return "", nil
	}
	if ln > uint32(r.Len()) {
		return "", io.ErrUnexpectedEOF
	}
	buf := make([]byte, ln)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func digest16(data []byte) [16]byte {
	sum := sha256.Sum256(data)
	var out [16]byte
	copy(out[:], sum[:16])
	return out
}
