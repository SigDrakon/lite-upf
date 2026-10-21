// Package pfcp owns the PFCP (N4) control-plane listener.
package pfcp

import (
	"fmt"
	"log/slog"
	"net"
)

// Server is the N4 PFCP listener.
//
// The PFCP message decoding is intentionally not implemented yet; see
// AGENTS.md, "Incremental Protocol Support". The listener exists so the
// process lifecycle and the socket binding can be exercised first.
type Server struct {
	conn *net.UDPConn
	log  *slog.Logger
}

// NewServer binds the UDP socket on addr. The caller must close the server
// to release the socket.
func NewServer(addr string, log *slog.Logger) (*Server, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("pfcp: resolve %q: %w", addr, err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("pfcp: listen on %q: %w", addr, err)
	}

	return &Server{conn: conn, log: log}, nil
}

// Addr returns the address the server is bound to.
func (s *Server) Addr() *net.UDPAddr {
	return s.conn.LocalAddr().(*net.UDPAddr)
}

// Serve reads messages until the connection is closed.
func (s *Server) Serve() error {
	buf := make([]byte, 65535)

	for {
		n, peer, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return err
		}

		s.log.Debug("n4: message received",
			"interface", "N4",
			"remote", peer.String(),
			"length", n,
		)
	}
}

// Close releases the socket and unblocks Serve.
func (s *Server) Close() error {
	return s.conn.Close()
}
