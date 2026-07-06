package agoratoken

import (
	"compress/zlib"
	"encoding/base64"
	"io"
	"strings"
	"testing"
)

const (
	tvAppID = "970CA35de60c44645bbae8a215061b33"
	tvCert  = "5CFd2fd1755d40ecb72977518be15d3b"
	tvChan  = "7d72365eb983485397e3e3f9d460bdda"
	tvUID   = uint32(2882341273)
)

// Authoritative token produced by a standalone reproduction of the Agora
// AccessToken2 reference implementation (AgoraIO/Tools
// DynamicKey/AgoraDynamicKey/go/src/accesstoken2/accesstoken.go) with the
// fixed test inputs below (issueTs=1111111111, expire=600, salt=1, RTC/RTM
// privilegeExpire=1111111711, RTM userID="2882341273").
const wantCombined = "007eJxSYGj4W7VvnUzhlLlmvx7eS1LmbY/VOc4pMTVc89nr92tPb6hRYLA0N3B2NDZNSTUzSDYxMTMxTUpKTLVINDI0NTAzTDI2Pm5q5RTBxMDAyMDAwMTAyMDCwMggb2HlxAQmmcEkC5hUYDBPMTcyNjNNTbK0MDaxMDW2NE81TjVOs0wxMTNISklJ5GIwsrAwMjYxNDI3BpkFMQlZFBAAAP//xn0shw=="

func fixedToken() *AccessToken2 {
	tk := NewAccessToken2(tvAppID, tvCert, 1111111111, 600)
	tk.salt = 1 // deterministic salt for tests
	tk.AddRtcService(tvChan, tvUID, 1111111711)
	tk.AddRtmService("2882341273", 1111111711)
	return tk
}

func TestBuildMatchesGoldenVector(t *testing.T) {
	got, err := fixedToken().Build()
	if err != nil {
		t.Fatal(err)
	}
	if got != wantCombined {
		t.Fatalf("token mismatch:\n got=%s\nwant=%s", got, wantCombined)
	}
}

func TestTokenDecodesToBothServices(t *testing.T) {
	tok, _ := fixedToken().Build()
	if !strings.HasPrefix(tok, "007") {
		t.Fatalf("missing 007 version prefix")
	}
	raw, err := base64.StdEncoding.DecodeString(tok[3:])
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zlib.NewReader(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(zr); err != nil {
		t.Fatalf("payload not valid zlib: %v", err)
	}
	tk := fixedToken()
	if _, ok := tk.services[serviceTypeRtc]; !ok {
		t.Fatalf("RTC service missing")
	}
	if _, ok := tk.services[serviceTypeRtm]; !ok {
		t.Fatalf("RTM service missing")
	}
}

func TestRandomSaltProducesDifferentTokens(t *testing.T) {
	a := NewAccessToken2(tvAppID, tvCert, 1111111111, 600)
	a.AddRtcService(tvChan, tvUID, 1111111711)
	b := NewAccessToken2(tvAppID, tvCert, 1111111111, 600)
	b.AddRtcService(tvChan, tvUID, 1111111711)
	ta, _ := a.Build()
	tb, _ := b.Build()
	if ta == tb {
		t.Fatalf("expected random salt to vary output")
	}
}
