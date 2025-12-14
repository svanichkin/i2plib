package i2plib

import (
	"crypto/rand"
	"math/big"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type Address struct {
	Host string
	Port int
}

// DefaultSAMAddress — аналог sam.DEFAULT_ADDRESS ("127.0.0.1", 7656)
var DefaultSAMAddress = Address{
	Host: "127.0.0.1",
	Port: 7656,
}

// GetFreePort — аналог get_free_port()
func GetFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()

	addr := l.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

// IsAddressAccessible — аналог is_address_accessible()
func IsAddressAccessible(addr Address) bool {
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).Dial("tcp", net.JoinHostPort(addr.Host, strconv.Itoa(addr.Port)))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// AddressFromString — аналог address_from_string("host:port")
func AddressFromString(s string) (Address, error) {
	s = strings.TrimSpace(s)
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		// allow host:port without brackets for IPv6-less inputs
		parts := strings.Split(s, ":")
		if len(parts) != 2 {
			return Address{}, net.InvalidAddrError("invalid address: " + s)
		}
		host = parts[0]
		portStr = parts[1]
	}
	p, err := strconv.Atoi(portStr)
	if err != nil {
		return Address{}, err
	}
	return Address{
		Host: host,
		Port: p,
	}, nil
}

// GetSAMAddress — аналог get_sam_address()
func GetSAMAddress() Address {
	if v := os.Getenv("I2P_SAM_ADDRESS"); v != "" {
		if addr, err := AddressFromString(v); err == nil {
			return addr
		}
	}
	return DefaultSAMAddress
}

// GenerateSessionID — аналог generate_session_id(), length по умолчанию 6
func GenerateSessionID(length int) string {
	if length <= 0 {
		length = 6
	}

	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	var b strings.Builder
	b.Grow(len("reticulum-") + length)
	b.WriteString("reticulum-")

	for i := 0; i < length; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(letters))))
		if err != nil {
			// в крайне маловероятном случае ошибки просто берём первый символ
			b.WriteByte(letters[0])
			continue
		}
		b.WriteByte(letters[n.Int64()])
	}

	return b.String()
}
