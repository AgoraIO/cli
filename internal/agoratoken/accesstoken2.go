package agoratoken

import (
	"bytes"
	"compress/zlib"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"math/big"
	"sort"
	"strconv"
)

const version2 = "007"

const (
	serviceTypeRtc uint16 = 1
	serviceTypeRtm uint16 = 2
)

// RTC privileges.
const (
	privJoinChannel        uint16 = 1
	privPublishAudioStream uint16 = 2
	privPublishVideoStream uint16 = 3
	privPublishDataStream  uint16 = 4
)

// RTM privileges.
const privRtmLogin uint16 = 1

type service struct {
	typ        uint16
	privileges map[uint16]uint32
	packExtra  func(buf *bytes.Buffer)
}

func (s *service) pack(buf *bytes.Buffer) {
	packUint16(buf, s.typ)
	packMapUint32(buf, s.privileges)
	if s.packExtra != nil {
		s.packExtra(buf)
	}
}

// AccessToken2 builds a version-007 Agora token. salt/issueTs are fields so
// tests can pin them; production callers use NewAccessToken2 which seeds a
// cryptographically-random salt.
type AccessToken2 struct {
	appID    string
	appCert  string
	issueTs  uint32
	expire   uint32
	salt     uint32
	services map[uint16]*service
}

// NewAccessToken2 seeds a random salt. issueTs is the token issue time (unix
// seconds); expire is seconds-from-issue for the token envelope.
func NewAccessToken2(appID, appCert string, issueTs, expire uint32) *AccessToken2 {
	n, err := rand.Int(rand.Reader, big.NewInt(0xFFFFFFFF))
	if err != nil {
		panic("agoratoken: crypto/rand failed: " + err.Error())
	}
	return &AccessToken2{
		appID:    appID,
		appCert:  appCert,
		issueTs:  issueTs,
		expire:   expire,
		salt:     uint32(n.Int64()) + 1,
		services: map[uint16]*service{},
	}
}

// AddRtcService registers RTC join+publish privileges for uid on channel, each
// expiring at privilegeExpire (unix seconds).
func (t *AccessToken2) AddRtcService(channel string, uid uint32, privilegeExpire uint32) {
	uidStr := ""
	if uid != 0 {
		uidStr = strconv.FormatUint(uint64(uid), 10)
	}
	svc := &service{
		typ: serviceTypeRtc,
		privileges: map[uint16]uint32{
			privJoinChannel:        privilegeExpire,
			privPublishAudioStream: privilegeExpire,
			privPublishVideoStream: privilegeExpire,
			privPublishDataStream:  privilegeExpire,
		},
	}
	svc.packExtra = func(buf *bytes.Buffer) {
		packString(buf, channel)
		packString(buf, uidStr)
	}
	t.services[serviceTypeRtc] = svc
}

// AddRtmService registers an RTM login privilege for userID.
func (t *AccessToken2) AddRtmService(userID string, privilegeExpire uint32) {
	svc := &service{
		typ:        serviceTypeRtm,
		privileges: map[uint16]uint32{privRtmLogin: privilegeExpire},
	}
	svc.packExtra = func(buf *bytes.Buffer) {
		packString(buf, userID)
	}
	t.services[serviceTypeRtm] = svc
}

func hmacSha256(key, msg []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write(msg)
	return m.Sum(nil)
}

func uint32LE(v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return b
}

// sign derives the per-token signing key: HMAC(key=issueTs, msg=cert) then
// HMAC(key=salt, msg=prev).
func (t *AccessToken2) sign() []byte {
	h1 := hmacSha256(uint32LE(t.issueTs), []byte(t.appCert))
	return hmacSha256(uint32LE(t.salt), h1)
}

// Build returns the "007"-prefixed base64 token.
func (t *AccessToken2) Build() (string, error) {
	signing := t.sign()

	var m bytes.Buffer
	packString(&m, t.appID)
	packUint32(&m, t.issueTs)
	packUint32(&m, t.expire)
	packUint32(&m, t.salt)
	packUint16(&m, uint16(len(t.services)))
	keys := make([]int, 0, len(t.services))
	for k := range t.services {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	for _, k := range keys {
		t.services[uint16(k)].pack(&m)
	}

	signature := hmacSha256(signing, m.Bytes())

	var content bytes.Buffer
	packString(&content, string(signature))
	content.Write(m.Bytes())

	var zbuf bytes.Buffer
	zw := zlib.NewWriter(&zbuf)
	if _, err := zw.Write(content.Bytes()); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}

	return version2 + base64.StdEncoding.EncodeToString(zbuf.Bytes()), nil
}
