package i2plib

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type contract struct {
	I2PBase64 struct {
		RawHex    string `json:"raw_hex"`
		B64Padded string `json:"b64_padded"`
	} `json:"i2p_base64"`
	SAMMessage struct {
		Raw    string            `json:"raw"`
		Cmd    string            `json:"cmd"`
		Action string            `json:"action"`
		Opts   map[string]string `json:"opts"`
	} `json:"sam_message"`
	SAMCommands struct {
		Hello             string `json:"hello"`
		NamingLookup      string `json:"naming_lookup"`
		DestGenerate      string `json:"dest_generate"`
		SessionCreatePref string `json:"session_create_prefix"`
		StreamConnect     string `json:"stream_connect"`
		StreamAccept      string `json:"stream_accept"`
	} `json:"sam_commands"`
}

func loadContract(t *testing.T) contract {
	t.Helper()
	path := filepath.Join("testdata", "contract.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read contract: %v", err)
	}
	var c contract
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatalf("parse contract: %v", err)
	}
	return c
}

func TestSAM_Contract_I2PBase64(t *testing.T) {
	c := loadContract(t)
	raw, err := hex.DecodeString(c.I2PBase64.RawHex)
	if err != nil {
		t.Fatalf("raw hex: %v", err)
	}

	enc := I2PBase64Encode(raw)
	if enc != c.I2PBase64.B64Padded {
		t.Fatalf("encode mismatch: got %q want %q", enc, c.I2PBase64.B64Padded)
	}

	dec, err := I2PBase64Decode(c.I2PBase64.B64Padded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if hex.EncodeToString(dec) != strings.ToLower(c.I2PBase64.RawHex) {
		t.Fatalf("decode mismatch: got %x want %s", dec, c.I2PBase64.RawHex)
	}
}

func TestSAM_Contract_ParseMessage(t *testing.T) {
	c := loadContract(t)
	m, err := ParseSAMMessage([]byte(c.SAMMessage.Raw))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.Cmd != c.SAMMessage.Cmd || m.Action != c.SAMMessage.Action {
		t.Fatalf("cmd/action mismatch: got %s %s want %s %s", m.Cmd, m.Action, c.SAMMessage.Cmd, c.SAMMessage.Action)
	}
	for k, v := range c.SAMMessage.Opts {
		if m.Opts[k] != v {
			t.Fatalf("opt %s mismatch: got %q want %q", k, m.Opts[k], v)
		}
	}
}

func TestSAM_Contract_Commands(t *testing.T) {
	c := loadContract(t)
	if string(HelloMsg(DefaultMinVer, DefaultMaxVer)) != c.SAMCommands.Hello {
		t.Fatalf("hello mismatch")
	}
	if string(NamingLookupMsg("example.i2p")) != c.SAMCommands.NamingLookup {
		t.Fatalf("naming lookup mismatch")
	}
	if string(DestGenerateMsg(DefaultSigType)) != c.SAMCommands.DestGenerate {
		t.Fatalf("dest generate mismatch")
	}

	msg := string(SessionCreateMsg("STREAM", "test", "TRANSIENT", ""))
	if !strings.HasPrefix(msg, c.SAMCommands.SessionCreatePref) {
		t.Fatalf("session create prefix mismatch: %q", msg)
	}
	if string(StreamConnectMsg("test", "DEST_B64", "false")) != c.SAMCommands.StreamConnect {
		t.Fatalf("stream connect mismatch")
	}
	if string(StreamAcceptMsg("test", "false")) != c.SAMCommands.StreamAccept {
		t.Fatalf("stream accept mismatch")
	}
}

