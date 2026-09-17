package agoratoken

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestPackPrimitives(t *testing.T) {
	var buf bytes.Buffer
	packUint16(&buf, 0x0102)
	packUint32(&buf, 0x03040506)
	packString(&buf, "ab")
	// uint16 LE: 02 01 | uint32 LE: 06 05 04 03 | len16 LE: 02 00 | "ab": 61 62
	if got := hex.EncodeToString(buf.Bytes()); got != "0201060504030200"+"6162" {
		t.Fatalf("packing mismatch: %s", got)
	}
}

func TestPackMapUint32SortedByKey(t *testing.T) {
	var buf bytes.Buffer
	packMapUint32(&buf, map[uint16]uint32{2: 200, 1: 100})
	// count 02 00 | key1 01 00 val 64 00 00 00 | key2 02 00 val C8 00 00 00
	if got := hex.EncodeToString(buf.Bytes()); got != "0200"+"0100"+"64000000"+"0200"+"c8000000" {
		t.Fatalf("map packing must be key-sorted, got: %s", got)
	}
}
