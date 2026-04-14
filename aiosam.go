package i2plib

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
)

// SAMSocket — аналог (reader, writer)
type SAMSocket struct {
	Conn   net.Conn
	Reader *bufio.Reader
	Writer *bufio.Writer
}

// parseReply — аналог parse_reply()
func parseReply(line []byte) (*SAMMessage, error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return nil, fmt.Errorf("empty response: SAM API went offline")
	}

	msg, err := ParseSAMMessage(line)
	if err != nil {
		return nil, fmt.Errorf("invalid SAM response: %w", err)
	}

	return msg, nil
}

// ошибка SAM по RESULT
func samErrorFromReply(msg *SAMMessage) error {
	if msg == nil {
		return fmt.Errorf("nil SAM reply")
	}
	code, ok := msg.Opts["RESULT"]
	if !ok || code == "" {
		return fmt.Errorf("SAM error: missing RESULT (%s)", strings.TrimSpace(msg.Raw))
	}
	if ctor, ok := SAMExceptionMap[code]; ok {
		return ctor("")
	}
	return fmt.Errorf("SAM error: %s", code)
}

func applyWriteDeadlineFromContext(conn net.Conn, ctx context.Context) {
	if conn == nil {
		return
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetWriteDeadline(deadline)
	}
}

func applyReadDeadlineFromContext(conn net.Conn, ctx context.Context) {
	if conn == nil {
		return
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetReadDeadline(deadline)
	}
}

// GetSAMSocket — аналог get_sam_socket()
func GetSAMSocket(ctx context.Context, samAddr Address) (*SAMSocket, error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(
		ctx,
		"tcp",
		net.JoinHostPort(samAddr.Host, fmt.Sprintf("%d", samAddr.Port)),
	)
	if err != nil {
		return nil, err
	}

	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// HELLO VERSION
	applyWriteDeadlineFromContext(conn, ctx)
	if _, err := writer.Write(HelloMsg(DefaultMinVer, DefaultMaxVer)); err != nil {
		conn.Close()
		return nil, err
	}
	applyWriteDeadlineFromContext(conn, ctx)
	if err := writer.Flush(); err != nil {
		conn.Close()
		return nil, err
	}

	applyReadDeadlineFromContext(conn, ctx)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		conn.Close()
		return nil, err
	}

	reply, err := parseReply(line)
	if err != nil {
		conn.Close()
		return nil, err
	}

	if reply.OK() {
		return &SAMSocket{Conn: conn, Reader: reader, Writer: writer}, nil
	}

	conn.Close()
	return nil, samErrorFromReply(reply)
}

// DestLookup — аналог dest_lookup()
func DestLookup(ctx context.Context, domain string, samAddr Address) (*Destination, error) {
	sock, err := GetSAMSocket(ctx, samAddr)
	if err != nil {
		return nil, err
	}
	defer sock.Conn.Close()

	applyWriteDeadlineFromContext(sock.Conn, ctx)
	if _, err := sock.Writer.Write(NamingLookupMsg(domain)); err != nil {
		return nil, err
	}
	applyWriteDeadlineFromContext(sock.Conn, ctx)
	if err := sock.Writer.Flush(); err != nil {
		return nil, err
	}

	applyReadDeadlineFromContext(sock.Conn, ctx)
	line, err := sock.Reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	reply, err := parseReply(line)
	if err != nil {
		return nil, err
	}

	if !reply.OK() {
		return nil, samErrorFromReply(reply)
	}

	val := reply.Opts["VALUE"]
	return DestinationFromBase64(val, false)
}

// NewDestinationSAM — аналог new_destination()
func NewDestinationSAM(ctx context.Context, samAddr Address, sigType int) (*Destination, error) {
	sock, err := GetSAMSocket(ctx, samAddr)
	if err != nil {
		return nil, err
	}
	defer sock.Conn.Close()

	applyWriteDeadlineFromContext(sock.Conn, ctx)
	if _, err := sock.Writer.Write(DestGenerateMsg(sigType)); err != nil {
		return nil, err
	}
	applyWriteDeadlineFromContext(sock.Conn, ctx)
	if err := sock.Writer.Flush(); err != nil {
		return nil, err
	}

	applyReadDeadlineFromContext(sock.Conn, ctx)
	line, err := sock.Reader.ReadBytes('\n')
	if err != nil {
		return nil, err
	}

	reply, err := parseReply(line)
	if err != nil {
		return nil, err
	}

	priv := reply.Opts["PRIV"]
	return DestinationFromBase64(priv, true)
}

// SAMSession — аналог create_session() + Session context manager
type SAMSession struct {
	ID          string
	Socket      *SAMSocket
	Destination *Destination
}

// CreateSession — аналог create_session()
func CreateSession(
	ctx context.Context,
	sessionName string,
	samAddr Address,
	style string,
	sigType int,
	dest *Destination,
	options map[string]string,
) (*SAMSession, error) {
	if style == "" {
		style = "STREAM"
	}
	if sessionName == "" {
		sessionName = GenerateSessionID(6)
	}

	var destString string
	if dest != nil && dest.PrivKey != nil {
		destString = dest.PrivKey.Base64
	} else if dest != nil {
		// если вдруг без приватного ключа
		destString = dest.Base64
	} else {
		destString = TransientDest
	}

	optsSlice := make([]string, 0, len(options))
	keys := make([]string, 0, len(options))
	for k := range options {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		optsSlice = append(optsSlice, fmt.Sprintf("%s=%s", k, options[k]))
	}
	optsStr := strings.Join(optsSlice, " ")

	sock, err := GetSAMSocket(ctx, samAddr)
	if err != nil {
		return nil, err
	}

	applyWriteDeadlineFromContext(sock.Conn, ctx)
	if _, err := sock.Writer.Write(SessionCreateMsg(style, sessionName, destString, optsStr)); err != nil {
		sock.Conn.Close()
		return nil, err
	}
	applyWriteDeadlineFromContext(sock.Conn, ctx)
	if err := sock.Writer.Flush(); err != nil {
		sock.Conn.Close()
		return nil, err
	}

	applyReadDeadlineFromContext(sock.Conn, ctx)
	line, err := sock.Reader.ReadBytes('\n')
	if err != nil {
		sock.Conn.Close()
		return nil, err
	}

	reply, err := parseReply(line)
	if err != nil {
		sock.Conn.Close()
		return nil, err
	}

	if !reply.OK() {
		sock.Conn.Close()
		return nil, samErrorFromReply(reply)
	}

	if dest == nil {
		dstStr := reply.Opts["DESTINATION"]
		dest, err = DestinationFromBase64(dstStr, true)
		if err != nil {
			sock.Conn.Close()
			return nil, err
		}
	}

	return &SAMSession{
		ID:          sessionName,
		Socket:      sock,
		Destination: dest,
	}, nil
}

func (s *SAMSession) Close() error {
	if s == nil || s.Socket == nil || s.Socket.Conn == nil {
		return nil
	}
	return s.Socket.Conn.Close()
}

func (s *SAMSession) Name() string {
	if s == nil {
		return ""
	}
	return s.ID
}

// StreamConnect — аналог stream_connect()
func StreamConnect(
	ctx context.Context,
	sessionName string,
	destStr string,
	samAddr Address,
) (*SAMSocket, *Destination, error) {
	var (
		dest *Destination
		err  error
	)

	if strings.HasSuffix(destStr, ".i2p") {
		// домен или .b32.i2p
		dest, err = DestLookup(ctx, destStr, samAddr)
		if err != nil {
			return nil, nil, err
		}
	} else {
		// base64 destination
		dest, err = DestinationFromBase64(destStr, false)
		if err != nil {
			return nil, nil, err
		}
	}

	sock, err := GetSAMSocket(ctx, samAddr)
	if err != nil {
		return nil, nil, err
	}

	if _, err := sock.Writer.Write(StreamConnectMsg(sessionName, dest.Base64, "false")); err != nil {
		sock.Conn.Close()
		return nil, nil, err
	}
	applyWriteDeadlineFromContext(sock.Conn, ctx)
	if err := sock.Writer.Flush(); err != nil {
		sock.Conn.Close()
		return nil, nil, err
	}

	applyReadDeadlineFromContext(sock.Conn, ctx)
	line, err := sock.Reader.ReadBytes('\n')
	if err != nil {
		sock.Conn.Close()
		return nil, nil, err
	}

	reply, err := parseReply(line)
	if err != nil {
		sock.Conn.Close()
		return nil, nil, err
	}

	if !reply.OK() {
		sock.Conn.Close()
		return nil, nil, samErrorFromReply(reply)
	}

	return sock, dest, nil
}

// StreamAccept — аналог stream_accept()
func StreamAccept(
	ctx context.Context,
	sessionName string,
	samAddr Address,
) (*SAMSocket, error) {
	sock, err := GetSAMSocket(ctx, samAddr)
	if err != nil {
		return nil, err
	}

	applyWriteDeadlineFromContext(sock.Conn, ctx)
	if _, err := sock.Writer.Write(StreamAcceptMsg(sessionName, "false")); err != nil {
		sock.Conn.Close()
		return nil, err
	}
	applyWriteDeadlineFromContext(sock.Conn, ctx)
	if err := sock.Writer.Flush(); err != nil {
		sock.Conn.Close()
		return nil, err
	}

	applyReadDeadlineFromContext(sock.Conn, ctx)
	line, err := sock.Reader.ReadBytes('\n')
	if err != nil {
		sock.Conn.Close()
		return nil, err
	}

	reply, err := parseReply(line)
	if err != nil {
		sock.Conn.Close()
		return nil, err
	}

	if !reply.OK() {
		sock.Conn.Close()
		return nil, samErrorFromReply(reply)
	}

	return sock, nil
}
