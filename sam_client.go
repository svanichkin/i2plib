package i2plib

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
)

// DefaultSAMClient реализует SAMClient поверх функций из aiosam.go.
type DefaultSAMClient struct {
	Address Address
	SigType int
}

func NewDefaultSAMClient(addr Address) *DefaultSAMClient {
	return &DefaultSAMClient{
		Address: addr,
		SigType: DefaultSigType,
	}
}

func (c *DefaultSAMClient) resolveAddr(candidate Address) Address {
	if candidate.Host != "" && candidate.Port != 0 {
		return candidate
	}
	if c.Address.Host != "" && c.Address.Port != 0 {
		return c.Address
	}
	return DefaultSAMAddress
}

func (c *DefaultSAMClient) sigType() int {
	if c.SigType == 0 {
		return DefaultSigType
	}
	return c.SigType
}

func (c *DefaultSAMClient) NewDestination(ctx context.Context, dest *Destination) (*Destination, error) {
	if dest != nil {
		return dest, nil
	}
	addr := c.resolveAddr(Address{})
	return NewDestinationSAM(ctx, addr, c.sigType())
}

func (c *DefaultSAMClient) CreateStreamSession(ctx context.Context, name, style string, opts map[string]string, dest *Destination, samAddr Address) (StreamSession, error) {
	addr := c.resolveAddr(samAddr)
	return CreateSession(ctx, name, addr, style, c.sigType(), dest, opts)
}

func (c *DefaultSAMClient) StreamConnect(ctx context.Context, sess StreamSession, remoteDest string, samAddr Address) (net.Conn, error) {
	addr := c.resolveAddr(samAddr)
	sessionID := ""
	if sess != nil {
		sessionID = sess.Name()
	} else {
		return nil, errors.New("stream session is nil")
	}
	sock, _, err := StreamConnect(ctx, sessionID, remoteDest, addr)
	if err != nil {
		return nil, err
	}
	return sock.Conn, nil
}

func (c *DefaultSAMClient) StreamAccept(ctx context.Context, sess StreamSession, samAddr Address) (*IncomingStream, error) {
	addr := c.resolveAddr(samAddr)
	sessionID := ""
	if sess != nil {
		sessionID = sess.Name()
	} else {
		return nil, errors.New("stream session is nil")
	}
	sock, err := StreamAccept(ctx, sessionID, addr)
	if err != nil {
		return nil, err
	}

	line, err := sock.Reader.ReadBytes('\n')
	if err != nil {
		_ = sock.Conn.Close()
		return nil, err
	}
	destB64 := strings.TrimSpace(string(line))

	var firstData []byte
	if buffered := sock.Reader.Buffered(); buffered > 0 {
		firstData = make([]byte, buffered)
		if _, err := io.ReadFull(sock.Reader, firstData); err != nil {
			_ = sock.Conn.Close()
			return nil, err
		}
	}

	return &IncomingStream{
		Conn:      sock.Conn,
		DestB64:   destB64,
		FirstData: firstData,
	}, nil
}
