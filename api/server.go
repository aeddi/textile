package api

import (
	"context"
	"net"
	"time"

	auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"github.com/grpc-ecosystem/go-grpc-middleware/util/metautils"
	logging "github.com/ipfs/go-log"
	iface "github.com/ipfs/interface-go-ipfs-core"
	ma "github.com/multiformats/go-multiaddr"
	fc "github.com/textileio/filecoin/api/client"
	"github.com/textileio/go-threads/broadcast"
	"github.com/textileio/go-threads/util"
	pb "github.com/textileio/textile/api/pb"
	c "github.com/textileio/textile/collections"
	"github.com/textileio/textile/dns"
	"github.com/textileio/textile/email"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	log = logging.Logger("textileapi")

	ignoreMethods = []string{
		"/pb.API/Login",
	}
)

// reqKey provides a concrete type for request context values.
type reqKey string

// Server provides a gRPC API to the textile daemon.
type Server struct {
	rpc     *grpc.Server
	service *service

	gatewayToken string

	ctx    context.Context
	cancel context.CancelFunc
}

// Config specifies server settings.
type Config struct {
	Addr ma.Multiaddr

	Collections *c.Collections

	EmailClient    *email.Client
	IPFSClient     iface.CoreAPI
	FilecoinClient *fc.Client
	DNSManager     *dns.Manager

	GatewayUrl   string
	GatewayToken string

	SessionBus    *broadcast.Broadcaster
	SessionSecret string

	Debug bool
}

// NewServer starts and returns a new server.
func NewServer(ctx context.Context, conf Config) (*Server, error) {
	var err error
	if conf.Debug {
		err = util.SetLogLevels(map[string]logging.LogLevel{
			"textileapi": logging.LevelDebug,
		})
		if err != nil {
			return nil, err
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	s := &Server{
		service: &service{
			collections:    conf.Collections,
			emailClient:    conf.EmailClient,
			ipfsClient:     conf.IPFSClient,
			filecoinClient: conf.FilecoinClient,
			dnsManager:     conf.DNSManager,
			gatewayUrl:     conf.GatewayUrl,
			sessionBus:     conf.SessionBus,
			sessionSecret:  conf.SessionSecret,
		},
		gatewayToken: conf.GatewayToken,
		ctx:          ctx,
		cancel:       cancel,
	}
	s.rpc = grpc.NewServer(
		grpc.UnaryInterceptor(auth.UnaryServerInterceptor(s.authFunc)),
		grpc.StreamInterceptor(auth.StreamServerInterceptor(s.authFunc)),
	)

	addr, err := util.TCPAddrFromMultiAddr(conf.Addr)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	go func() {
		pb.RegisterAPIServer(s.rpc, s.service)
		if err := s.rpc.Serve(listener); err != nil {
			log.Errorf("error registering server: %v", err)
		}
	}()

	return s, nil
}

// Close the server.
func (s *Server) Close() error {
	s.rpc.GracefulStop()
	if s.service.filecoinClient != nil {
		if err := s.service.filecoinClient.Close(); err != nil {
			return err
		}
	}
	s.cancel()
	return nil
}

func (s *Server) authFunc(ctx context.Context) (context.Context, error) {
	method, _ := grpc.Method(ctx)
	for _, ignored := range ignoreMethods {
		if method == ignored {
			return ctx, nil
		}
	}

	token, err := auth.AuthFromMD(ctx, "bearer")
	if err != nil {
		return nil, err
	}
	if token == s.gatewayToken {
		return context.WithValue(ctx, reqKey("scope"), "*"), nil
	}

	session, err := s.service.collections.Sessions.Get(ctx, token)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "Invalid auth token")
	}
	if session.Expiry < int(time.Now().Unix()) {
		return nil, status.Error(codes.Unauthenticated, "Expired auth token")
	}
	user, err := s.service.collections.Developers.Get(ctx, session.UserID)
	if err != nil {
		return nil, status.Error(codes.PermissionDenied, "User not found")
	}

	scope := metautils.ExtractIncoming(ctx).Get("X-Scope")
	if scope != "" {
		if scope != user.ID {
			if _, err := s.service.getTeamForUser(ctx, scope, user); err != nil {
				return nil, err
			}
		}
	} else {
		scope = session.Scope
	}

	if err := s.service.collections.Sessions.Touch(ctx, session); err != nil {
		return nil, err
	}

	newCtx := context.WithValue(ctx, reqKey("session"), session)
	newCtx = context.WithValue(newCtx, reqKey("user"), user)
	newCtx = context.WithValue(newCtx, reqKey("scope"), scope)
	return newCtx, nil
}
