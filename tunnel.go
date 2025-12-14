package i2plib

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"
)

const BufferSize = 65536

// SAMClient — абстракция поверх SAM API.
// Реализацию делаешь отдельно.
type SAMClient interface {
	NewDestination(ctx context.Context, dest *Destination) (*Destination, error)
	CreateStreamSession(ctx context.Context, name, style string, opts map[string]string, dest *Destination, samAddr Address) (StreamSession, error)
	StreamConnect(ctx context.Context, sess StreamSession, remoteDest string, samAddr Address) (net.Conn, error)

	// для серверного туннеля: принять входящее соединение
	StreamAccept(ctx context.Context, sess StreamSession, samAddr Address) (*IncomingStream, error)
}

// StreamSession — сессия STREAM в SAM.
type StreamSession interface {
	Name() string
	Close() error
}

// IncomingStream — входящее соединение с I2P.
type IncomingStream struct {
	Conn      net.Conn
	DestB64   string // исходный dest как строка
	FirstData []byte // первые считанные байты (data после dest\n)
}

// TunnelStatus — аналог self.status из Python.
type TunnelStatus struct {
	SetupRan    bool
	SetupFailed bool
	Err         error
}

// базовый туннель

type I2PTunnel struct {
	LocalAddress Address

	Destination *Destination
	SessionName string
	Options     map[string]string

	SAMAddress Address
	SAM        SAMClient

	Session StreamSession
}

func NewI2PTunnel(local Address, samAddr Address, samClient SAMClient, dest *Destination, sessionName string, opts map[string]string) *I2PTunnel {
	if opts == nil {
		opts = make(map[string]string)
	}
	if sessionName == "" {
		sessionName = GenerateSessionID(6)
	}

	return &I2PTunnel{
		LocalAddress: local,
		Destination:  dest,
		SessionName:  sessionName,
		Options:      opts,
		SAMAddress:   samAddr,
		SAM:          samClient,
	}
}

func (t *I2PTunnel) preRun(ctx context.Context, style string) error {
	var err error

	if t.Destination == nil {
		t.Destination, err = t.SAM.NewDestination(ctx, nil)
		if err != nil {
			return err
		}
	}

	t.Session, err = t.SAM.CreateStreamSession(ctx, t.SessionName, style, t.Options, t.Destination, t.SAMAddress)
	return err
}

func (t *I2PTunnel) Stop() {
	if t.Session != nil {
		if err := t.Session.Close(); err != nil {
			log.Printf("i2ptunnel: session close error: %v", err)
		}
	}
}

// проксирование данных

type closeWriter interface {
	CloseWrite() error
}

func tryCloseWrite(c net.Conn) {
	if cw, ok := c.(closeWriter); ok {
		_ = cw.CloseWrite()
	}
}

func proxyOneWay(dst, src net.Conn) {
	_, _ = io.Copy(dst, src)
	tryCloseWrite(dst)
	_ = dst.Close()
}

// proxyBidirectionalAsync mimics the Python implementation:
// it starts background copy tasks and returns immediately.
func proxyBidirectionalAsync(a, b net.Conn) {
	go proxyOneWay(a, b)
	go proxyOneWay(b, a)
}

// ==================== ClientTunnel ====================

type ClientTunnel struct {
	*I2PTunnel
	RemoteDestination string

	listener net.Listener
	Status   TunnelStatus
	statusMu sync.Mutex
}

func NewClientTunnel(local Address, remoteDest string, samAddr Address, samClient SAMClient, dest *Destination, sessionName string, opts map[string]string) *ClientTunnel {
	return &ClientTunnel{
		I2PTunnel:         NewI2PTunnel(local, samAddr, samClient, dest, sessionName, opts),
		RemoteDestination: remoteDest,
	}
}

// Run — аналог async def run(self) у ClientTunnel.
func (t *ClientTunnel) Run(ctx context.Context) error {
	if err := t.preRun(ctx, "STREAM"); err != nil {
		t.setStatus(true, true, err)
		return err
	}

	addr := net.JoinHostPort(t.LocalAddress.Host, strconv.Itoa(t.LocalAddress.Port))
	l, err := net.Listen("tcp", addr)
	if err != nil {
		t.setStatus(true, true, err)
		return err
	}
	t.listener = l
	t.setStatus(true, false, nil)

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}
				select {
				case <-ctx.Done():
					return
				default:
					log.Printf("i2ptunnel client: accept error: %v", err)
					return
				}
			}
			go t.handleClient(ctx, conn)
		}
	}()

	return nil
}

func (t *ClientTunnel) handleClient(ctx context.Context, client net.Conn) {
	remote, err := t.SAM.StreamConnect(ctx, t.Session, t.RemoteDestination, t.SAMAddress)
	if err != nil {
		_ = client.Close()
		t.setStatus(true, true, err)
		log.Printf("i2ptunnel client: stream_connect error: %v", err)
		return
	}

	proxyBidirectionalAsync(client, remote)
}

func (t *ClientTunnel) setStatus(setupRan, setupFailed bool, err error) {
	t.statusMu.Lock()
	defer t.statusMu.Unlock()
	t.Status.SetupRan = setupRan
	t.Status.SetupFailed = setupFailed
	t.Status.Err = err
}

func (t *ClientTunnel) Stop() {
	t.I2PTunnel.Stop()
	if t.listener != nil {
		_ = t.listener.Close()
	}
}

// ==================== ServerTunnel ====================

type ServerTunnel struct {
	*I2PTunnel

	Status   TunnelStatus
	statusMu sync.Mutex
	cancelFn context.CancelFunc
}

func NewServerTunnel(local Address, samAddr Address, samClient SAMClient, dest *Destination, sessionName string, opts map[string]string) *ServerTunnel {
	return &ServerTunnel{
		I2PTunnel: NewI2PTunnel(local, samAddr, samClient, dest, sessionName, opts),
	}
}

func (t *ServerTunnel) Run(ctx context.Context) error {
	if err := t.preRun(ctx, "STREAM"); err != nil {
		t.setStatus(true, true, err)
		return err
	}

	ctx, cancel := context.WithCancel(ctx)
	t.cancelFn = cancel
	t.setStatus(true, false, nil)

	go t.serverLoop(ctx)
	return nil
}

func (t *ServerTunnel) serverLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		incoming, err := t.SAM.StreamAccept(ctx, t.Session, t.SAMAddress)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Printf("i2ptunnel server: stream_accept error: %v", err)
			t.setStatus(true, true, err)
			return
		}

		go t.handleIncoming(ctx, incoming)
	}
}

func (t *ServerTunnel) handleIncoming(ctx context.Context, inc *IncomingStream) {
	if inc == nil {
		return
	}
	client := inc.Conn

	if inc.DestB64 != "" {
		if dest, err := DestinationFromBase64(inc.DestB64, false); err == nil {
			log.Printf("i2ptunnel server: client connected: %s.b32.i2p", dest.Base32())
		}
	}

	addr := net.JoinHostPort(t.LocalAddress.Host, strconv.Itoa(t.LocalAddress.Port))

	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	localConn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	if err != nil {
		_ = client.Close()
		log.Printf("i2ptunnel server: local connect error: %v", err)
		t.setStatus(true, true, err)
		return
	}

	// если были данные помимо dest\n — отправляем их в локальный сервис
	if len(inc.FirstData) > 0 {
		_, _ = localConn.Write(inc.FirstData)
	}

	proxyBidirectionalAsync(client, localConn)
}

func (t *ServerTunnel) setStatus(setupRan, setupFailed bool, err error) {
	t.statusMu.Lock()
	defer t.statusMu.Unlock()
	t.Status.SetupRan = setupRan
	t.Status.SetupFailed = setupFailed
	t.Status.Err = err
}

func (t *ServerTunnel) Stop() {
	t.I2PTunnel.Stop()
	if t.cancelFn != nil {
		t.cancelFn()
	}
}
