// Package gtpu owns the GTP-U (N3) listener and the user-plane packet path.
package gtpu

import (
	"fmt"
	"log/slog"
	"net"
	"sync"
)

// Server is the N3 GTP-U listener.
type Server struct {
	conn *net.UDPConn
	log  *slog.Logger
	wg   sync.WaitGroup
}

// NewServer binds the UDP socket on addr. The caller must close the server
// to release the socket.
func NewServer(addr string, log *slog.Logger) (*Server, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return nil, fmt.Errorf("gtpu: resolve %q: %w", addr, err)
	}

	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("gtpu: listen on %q: %w", addr, err)
	}

	return &Server{conn: conn, log: log}, nil
}

// Addr returns the address the server is bound to.
func (s *Server) Addr() *net.UDPAddr {
	return s.conn.LocalAddr().(*net.UDPAddr)
}

// Serve reads packets until the connection is closed.
//
// Every packet is parsed and logged with its TEID. A malformed packet is
// counted and dropped; it never takes the listener down.
func (s *Server) Serve() error {
	buf := make([]byte, 65535)

	for {
		n, peer, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return err
		}

		header, payload, err := Parse(buf[:n])
		if err != nil {
			s.log.Warn("n3: dropping malformed packet",
				"interface", "N3",
				"remote", peer.String(),
				"error", err.Error(),
			)
			continue
		}

		s.log.Debug("n3: packet received",
			"interface", "N3",
			"remote", peer.String(),
			"message_type", header.MessageType,
			"teid", header.TEID,
			"payload_len", len(payload),
		)
	}
}

// Close releases the socket and unblocks Serve.
func (s *Server) Close() error {
	return s.conn.Close()
}
