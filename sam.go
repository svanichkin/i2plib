package i2plib

import (
	"bytes"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	I2PAltChars    = "-~"
	SAMBufSize     = 4096
	DefaultSAMHost = "127.0.0.1"
	DefaultSAMPort = 7656
	DefaultMinVer  = "3.1"
	DefaultMaxVer  = "3.1"
	TransientDest  = "TRANSIENT"

	SigTypeEd25519 = 7
	DefaultSigType = SigTypeEd25519
)

var (
	ValidBase32Address = regexp.MustCompile(`^([a-zA-Z0-9]{52}).b32\.i2p$`)
	ValidBase64Address = regexp.MustCompile(`^([a-zA-Z0-9\-~=]{516,528})$`)
)

/* ---------------------------------------------------------
 *   I2P Base64 Encoding
 * --------------------------------------------------------- */

func I2PBase64Encode(b []byte) string {
	// Python version uses padded base64 (with altchars -~).
	s := base64.StdEncoding.EncodeToString(b)
	return strings.NewReplacer("+", "-", "/", "~").Replace(s)
}

func I2PBase64Decode(s string) ([]byte, error) {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("-", "+", "~", "/").Replace(s)

	// Python version uses validate=True; Strict() matches that behavior closely.
	// Accept both padded and unpadded inputs.
	if strings.HasSuffix(s, "=") {
		return base64.StdEncoding.Strict().DecodeString(s)
	}
	return base64.RawStdEncoding.Strict().DecodeString(s)
}

/* ---------------------------------------------------------
 *   SAM Message Parser
 * --------------------------------------------------------- */

type SAMMessage struct {
	Cmd    string
	Action string
	Opts   map[string]string
	Raw    string
}

func ParseSAMMessage(b []byte) (*SAMMessage, error) {
	s := string(bytes.TrimSpace(b))
	parts := bytes.SplitN([]byte(s), []byte(" "), 3)
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid SAM message: %s", s)
	}

	m := &SAMMessage{
		Cmd:    string(parts[0]),
		Action: string(parts[1]),
		Raw:    s,
		Opts:   map[string]string{},
	}

	opts := bytes.Split(parts[2], []byte(" "))
	for _, o := range opts {
		if len(o) == 0 {
			continue
		}
		kv := bytes.SplitN(o, []byte("="), 2)
		if len(kv) == 2 {
			m.Opts[string(kv[0])] = string(kv[1])
		} else {
			m.Opts[string(kv[0])] = "true"
		}
	}

	return m, nil
}

func (m *SAMMessage) OK() bool {
	return m.Opts["RESULT"] == "OK"
}

func (m *SAMMessage) String() string {
	return m.Raw
}

/* ---------------------------------------------------------
 *   SAM Commands
 * --------------------------------------------------------- */

func HelloMsg(minVer, maxVer string) []byte {
	return []byte(fmt.Sprintf(
		"HELLO VERSION MIN=%s MAX=%s\n",
		minVer, maxVer,
	))
}

func SessionCreateMsg(style, id, dest, opts string) []byte {
	return []byte(fmt.Sprintf(
		"SESSION CREATE STYLE=%s ID=%s DESTINATION=%s %s\n",
		style, id, dest, opts,
	))
}

func StreamConnectMsg(id, dest, silent string) []byte {
	return []byte(fmt.Sprintf(
		"STREAM CONNECT ID=%s DESTINATION=%s SILENT=%s\n",
		id, dest, silent,
	))
}

func StreamAcceptMsg(id, silent string) []byte {
	return []byte(fmt.Sprintf(
		"STREAM ACCEPT ID=%s SILENT=%s\n",
		id, silent,
	))
}

func StreamForwardMsg(id string, port int, opts string) []byte {
	return []byte(fmt.Sprintf(
		"STREAM FORWARD ID=%s PORT=%d %s\n",
		id, port, opts,
	))
}

func NamingLookupMsg(name string) []byte {
	return []byte(fmt.Sprintf("NAMING LOOKUP NAME=%s\n", name))
}

func DestGenerateMsg(sigType int) []byte {
	if sigType == 0 {
		sigType = DefaultSigType
	}
	return []byte(fmt.Sprintf("DEST GENERATE SIGNATURE_TYPE=%d\n", sigType))
}

/* ---------------------------------------------------------
 *   Destination
 * --------------------------------------------------------- */

type Destination struct {
	Data    []byte
	Base64  string
	PrivKey *PrivateKey
}

func NewDestination(data []byte, hasPriv bool) (*Destination, error) {
	if len(data) == 0 {
		return nil, errors.New("empty destination data")
	}

	dest := &Destination{}

	if hasPriv {
		pk, err := NewPrivateKey(data)
		if err != nil {
			return nil, err
		}
		dest.PrivKey = pk

		// strip private keys from destination: first 387 + cert
		if len(pk.Data) < 387 {
			return nil, errors.New("private key data too short")
		}
		certLen := int(pk.Data[385])<<8 | int(pk.Data[386])
		if certLen < 0 || 387+certLen > len(pk.Data) {
			return nil, errors.New("invalid certificate length in private key data")
		}
		data = pk.Data[:387+certLen]
	}

	dest.Data = data
	dest.Base64 = I2PBase64Encode(data)
	return dest, nil
}

func DestinationFromBase64(b64 string, hasPriv bool) (*Destination, error) {
	data, err := I2PBase64Decode(b64)
	if err != nil {
		return nil, err
	}
	return NewDestination(data, hasPriv)
}

func (d *Destination) Base32() string {
	h := sha256.Sum256(d.Data)
	b32 := base32.StdEncoding.EncodeToString(h[:])
	return strings.ToLower(b32[:52])
}

func (d *Destination) String() string {
	return fmt.Sprintf("<Destination: %s>", d.Base32())
}

/* ---------------------------------------------------------
 *   PrivateKey
 * --------------------------------------------------------- */

type PrivateKey struct {
	Data   []byte
	Base64 string
}

func NewPrivateKey(data []byte) (*PrivateKey, error) {
	if len(data) == 0 {
		return nil, errors.New("empty private key data")
	}

	return &PrivateKey{
		Data:   data,
		Base64: I2PBase64Encode(data),
	}, nil
}
