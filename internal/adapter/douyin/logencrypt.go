package douyin

// Port of companion 12.7.3 index.js logEncrypt (device_register body).

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"hash/crc32"
	"time"
)

const (
	leMagic  = 29795
	leKey    = "I+D&*76:j27kVH<us9&d"
	leBlock  = 16
	leGzipOS = 3
)

func leWriteUTF(s string) []byte {
	var out []byte
	for _, ch := range s {
		t := int(ch)
		switch {
		case t <= 127:
			out = append(out, byte(t))
		case t <= 2047:
			out = append(out, byte(0xC0|(31&(t>>6))), byte(0x80|(63&t)))
		default:
			out = append(out, byte(0xE0|(15&(t>>12))), byte(0x80|(63&(t>>6))), byte(0x80|(63&t)))
		}
	}
	return out
}

// leGzipOS3 pako-like: flate level 6, gzip header os=3, crc32+isize trailer.
func leGzipOS3(data []byte, mtime uint32) []byte {
	var buf bytes.Buffer
	fw, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		return nil
	}
	_, _ = fw.Write(data)
	_ = fw.Close()
	comp := buf.Bytes()

	hdr := []byte{0x1F, 0x8B, 8, 0,
		byte(mtime), byte(mtime >> 8), byte(mtime >> 16), byte(mtime >> 24),
		0, leGzipOS}
	tail := make([]byte, 8)
	binary.LittleEndian.PutUint32(tail[0:], crc32.ChecksumIEEE(data))
	binary.LittleEndian.PutUint32(tail[4:], uint32(len(data)))
	out := make([]byte, 0, len(hdr)+len(comp)+8)
	out = append(out, hdr...)
	out = append(out, comp...)
	out = append(out, tail...)
	return out
}

func leStdKey(key string) []uint32 {
	r := leWriteUTF(key)
	n := len(key)
	if n > 16 {
		n = 16
	}
	var t [16]byte
	for i := 0; i < n; i++ {
		t[i] = r[i]
	}
	for i := n; i < 16; i++ {
		t[i] = sbox1[t[i-n]]
	}
	for i := 0; i < 16; i++ {
		t[i] = sbox0[t[i]]
	}
	var words [4]uint32
	for off := 0; off < 4; off++ {
		words[off] = uint32(t[off*4])<<24 | uint32(t[off*4+1])<<16 | uint32(t[off*4+2])<<8 | uint32(t[off*4+3])
	}
	return words[:]
}

func leXorKeyBlock(kw []uint32, block []byte) []byte {
	var w [4]uint32
	for i := 0; i < 4; i++ {
		w[i] = uint32(block[i*4])<<24 | uint32(block[i*4+1])<<16 | uint32(block[i*4+2])<<8 | uint32(block[i*4+3])
		w[i] ^= kw[i]
	}
	out := make([]byte, 0, 16)
	for _, x := range w {
		out = append(out, byte(x>>24), byte(x>>16), byte(x>>8), byte(x))
	}
	return out
}

// leEncrypt produces the device_register body bytes (6-byte header + blocks).
func leEncrypt(plain string) []byte {
	plainB := leWriteUTF(plain)
	kw := leStdKey(leKey)
	mtime := uint32(time.Now().Unix())

	r := []byte{
		byte(leMagic >> 8), byte(leMagic & 0xFF), 3, 0,
		0, 3, // sub_version 3
	}
	u := leGzipOS3(plainB, mtime)
	if u == nil {
		return nil
	}

	pad := leBlock - (len(u) % leBlock)
	var f []byte
	if pad == leBlock {
		r[3] = 0
		f = u
	} else {
		r[3] = byte(pad)
		f = make([]byte, 0, len(u)+pad)
		f = append(f, u...)
		for i := 0; i < pad; i++ {
			f = append(f, byte(pad))
		}
	}
	for i := range f {
		f[i] = sbox0[f[i]]
	}

	blocks := (len(u) + leBlock - 1) / leBlock
	var e []byte
	for b := 0; b < blocks; b++ {
		w := f[b*16 : (b+1)*16]
		h := uint32(w[0])<<24 | uint32(w[1])<<16 | uint32(w[2])<<8 | uint32(w[3])
		g := uint32(w[4])<<24 | uint32(w[5])<<16 | uint32(w[6])<<8 | uint32(w[7])
		u2 := uint32(w[8])<<24 | uint32(w[9])<<16 | uint32(w[10])<<8 | uint32(w[11])
		v := uint32(w[12])<<24 | uint32(w[13])<<16 | uint32(w[14])<<8 | uint32(w[15])
		g = (g << 8) | (g >> 24)
		u2 = (u2 << 16) | (u2 >> 16)
		v = (v << 24) | (v >> 8)
		w2 := make([]byte, 0, 16)
		for _, x := range []uint32{h, g, u2, v} {
			w2 = append(w2, byte(x>>24), byte(x>>16), byte(x>>8), byte(x))
		}
		e = append(e, leXorKeyBlock(kw, w2)...)
	}
	return append(r, e...)
}
