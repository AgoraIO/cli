package agoratoken

import (
	"bytes"
	"encoding/binary"
	"sort"
)

func packUint16(buf *bytes.Buffer, v uint16) {
	_ = binary.Write(buf, binary.LittleEndian, v)
}

func packUint32(buf *bytes.Buffer, v uint32) {
	_ = binary.Write(buf, binary.LittleEndian, v)
}

func packString(buf *bytes.Buffer, s string) {
	packUint16(buf, uint16(len(s)))
	buf.WriteString(s)
}

func packMapUint32(buf *bytes.Buffer, m map[uint16]uint32) {
	packUint16(buf, uint16(len(m)))
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	for _, k := range keys {
		packUint16(buf, uint16(k))
		packUint32(buf, m[uint16(k)])
	}
}
