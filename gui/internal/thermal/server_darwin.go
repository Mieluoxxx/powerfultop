//go:build darwin

package thermal

import (
	"errors"
	"net"
	"sync"
	"time"
)

// requestDeadline covers the slowest request: engaging can take EngageTimeout.
const requestDeadline = EngageTimeout + 10*time.Second

// HelperServer answers the app's requests, one JSON line per connection, and serializes every SMC
// access through one lock as the Swift helper's serial queue did.
type HelperServer struct {
	Controller  *FanController
	Requirement string
	// Verify authenticates a connection; nil checks the peer's audit token against Requirement.
	Verify func(conn *net.UnixConn, requirement string) error

	mu sync.Mutex
}

// Serve accepts connections until the listener closes.
func (s *HelperServer) Serve(l *net.UnixListener) error {
	for {
		conn, err := l.AcceptUnix()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		go s.handle(conn)
	}
}

func (s *HelperServer) handle(conn *net.UnixConn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(requestDeadline))
	verify := s.Verify
	if verify == nil {
		verify = verifyPeer
	}
	// Refuse before reading anything: an unknown process gets no say over the fans.
	if err := verify(conn, s.Requirement); err != nil {
		_ = WriteMessage(conn, Response{Error: "unauthorized client"})
		return
	}
	var req Request
	if err := ReadMessage(conn, &req); err != nil {
		return
	}
	if err := req.Validate(); err != nil {
		_ = WriteMessage(conn, Response{Error: err.Error()})
		return
	}
	_ = WriteMessage(conn, s.dispatch(req))
}

func (s *HelperServer) dispatch(req Request) Response {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch req.Op {
	case OpVersion:
		return Response{Version: HelperProtocolVersion}
	case OpSetTargets:
		if err := s.Controller.Apply(req.RPMs); err != nil {
			return Response{Error: err.Error()}
		}
	case OpRestore:
		s.Controller.RestoreAutomatic()
	}
	return Response{}
}

// Tick runs the controller's watchdog.
func (s *HelperServer) Tick() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Controller.Tick()
}

// Shutdown hands the fans back if this helper holds them; another fan tool may own them otherwise.
func (s *HelperServer) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Controller.Engaged() {
		s.Controller.RestoreAutomatic()
	}
}

func verifyPeer(conn *net.UnixConn, requirement string) error {
	token, err := PeerAuditToken(conn)
	if err != nil {
		return err
	}
	return VerifyAuditToken(token, requirement)
}
