package gizedge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

const iceRelayDialTimeout = 5 * time.Second

type tcpICERelay struct {
	listener net.Listener
	upstream string
}

func newTCPICERelay(listener net.Listener, upstream string) *tcpICERelay {
	return &tcpICERelay{listener: listener, upstream: upstream}
}

func (r *tcpICERelay) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = r.listener.Close()
	}()
	for {
		conn, err := r.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("edge: accept ice tcp: %w", err)
		}
		go r.proxyConn(conn)
	}
}

func (r *tcpICERelay) proxyConn(client net.Conn) {
	defer client.Close()
	upstream, err := (&net.Dialer{Timeout: iceRelayDialTimeout}).Dial("tcp", r.upstream)
	if err != nil {
		return
	}
	defer upstream.Close()
	errCh := make(chan error, 2)
	go func() {
		_, err := io.Copy(upstream, client)
		_ = upstream.SetDeadline(time.Now())
		errCh <- err
	}()
	go func() {
		_, err := io.Copy(client, upstream)
		_ = client.SetDeadline(time.Now())
		errCh <- err
	}()
	<-errCh
}

type udpICERelay struct {
	public   net.PacketConn
	upstream *net.UDPAddr

	mu       sync.Mutex
	sessions map[string]*udpRelaySession
}

type udpRelaySession struct {
	client net.Addr
	conn   *net.UDPConn
}

func newUDPICERelay(listenAddr, upstream string) (*udpICERelay, error) {
	public, err := net.ListenPacket("udp", listenAddr)
	if err != nil {
		return nil, fmt.Errorf("edge: listen ice udp: %w", err)
	}
	upstreamAddr, err := net.ResolveUDPAddr("udp", upstream)
	if err != nil {
		_ = public.Close()
		return nil, fmt.Errorf("edge: resolve upstream ice udp: %w", err)
	}
	return &udpICERelay{
		public:   public,
		upstream: upstreamAddr,
		sessions: make(map[string]*udpRelaySession),
	}, nil
}

func (r *udpICERelay) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = r.public.Close()
	}()
	defer r.closeSessions()

	buf := make([]byte, 64*1024)
	for {
		n, clientAddr, err := r.public.ReadFrom(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) || ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("edge: read ice udp: %w", err)
		}
		session, err := r.session(clientAddr)
		if err != nil {
			continue
		}
		if _, err := session.conn.Write(buf[:n]); err != nil {
			r.dropSession(clientAddr.String())
		}
	}
}

func (r *udpICERelay) session(clientAddr net.Addr) (*udpRelaySession, error) {
	key := clientAddr.String()
	r.mu.Lock()
	if session := r.sessions[key]; session != nil {
		r.mu.Unlock()
		return session, nil
	}
	conn, err := net.DialUDP("udp", nil, r.upstream)
	if err != nil {
		r.mu.Unlock()
		return nil, err
	}
	session := &udpRelaySession{client: clientAddr, conn: conn}
	r.sessions[key] = session
	r.mu.Unlock()

	go r.readUpstream(key, session)
	return session, nil
}

func (r *udpICERelay) readUpstream(key string, session *udpRelaySession) {
	buf := make([]byte, 64*1024)
	for {
		n, _, err := session.conn.ReadFromUDP(buf)
		if err != nil {
			r.dropSession(key)
			return
		}
		if _, err := r.public.WriteTo(buf[:n], session.client); err != nil {
			r.dropSession(key)
			return
		}
	}
}

func (r *udpICERelay) dropSession(key string) {
	r.mu.Lock()
	session := r.sessions[key]
	delete(r.sessions, key)
	r.mu.Unlock()
	if session != nil {
		_ = session.conn.Close()
	}
}

func (r *udpICERelay) closeSessions() {
	r.mu.Lock()
	sessions := r.sessions
	r.sessions = make(map[string]*udpRelaySession)
	r.mu.Unlock()
	for _, session := range sessions {
		_ = session.conn.Close()
	}
}
