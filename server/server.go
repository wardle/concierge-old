package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"github.com/rs/cors"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	health "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// Provider represents a server provider - providing GRPC server implementation
type Provider interface {
	// RegisterServer will be called to register your GRPC service
	RegisterServer(sd *grpc.Server)
	// RegisterHTTPProxy will be called to register your GRPC HTTP reverse proxy
	RegisterHTTPProxy(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) error
	// Close closes any resources associated with this provider
	Close() error
}

// Server represents a combined gRPC and REST server
// Generate self-signed local development certificates using:
// openssl req -newkey rsa:2048 -nodes -keyout domain.key -x509 -days 365 -out domain.crt
// and use "localhost" for host
//
type Server struct {
	Options
	auth      *Auth
	providers map[string]Provider
}

// New creates a new server
func New(opts Options) *Server {
	return &Server{
		Options: opts,
	}
}

// Options defines the options for a server.
type Options struct {
	RPCPort     int // port for main gRPC server
	RESTPort    int // port for a gRPC gateway - switched off if zero
	GRPCWebPort int // port for a gRPC-Web server - switched off if zero

	CertFile string
	KeyFile  string
}

// Close frees up any associated resources
func (sv *Server) Close() error {
	for _, p := range sv.providers {
		if err := p.Close(); err != nil {
			return err
		}
	}
	return nil
}

// RegisterAuthenticator turns on server authentication
func (sv *Server) RegisterAuthenticator(auth *Auth) {
	sv.auth = auth
}

// Register registers a provider with the server.
// This should not be called once server is running.
func (sv *Server) Register(name string, p Provider) {
	if sv.providers == nil {
		sv.providers = make(map[string]Provider)
	}
	sv.providers[name] = p
	log.Printf("server: registered provider: '%s'", name)
}

// RunServer runs a GRPC and a gateway REST server concurrently
func (sv *Server) RunServer() error {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// listen for OS signals for logging and graceful shutdown
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, os.Kill, syscall.SIGTERM)
	defer signal.Stop(sigs)

	// configure main gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", sv.RPCPort))
	if err != nil {
		return fmt.Errorf("failed to initialize TCP listen: %v", err)
	}
	defer lis.Close()
	opts := make([]grpc.ServerOption, 0)
	if sv.auth != nil {
		opts = append(opts, grpc.UnaryInterceptor(sv.unaryAuthInterceptor))
		opts = append(opts, grpc.StreamInterceptor(sv.streamAuthInterceptor))
	}
	if sv.Options.CertFile != "" && sv.Options.KeyFile != "" {
		creds, err := credentials.NewServerTLSFromFile(sv.Options.CertFile, sv.Options.KeyFile)
		if err != nil {
			return err
		}
		opts = append(opts, grpc.Creds(creds))
	}
	grpcServer := grpc.NewServer(opts...)
	health.RegisterHealthServer(grpcServer, sv)
	for name, provider := range sv.providers {
		provider.RegisterServer(grpcServer)
		log.Printf("server: registered '%s' service", name)
	}

	// configure HTTP reverse gateway
	clientAddr := fmt.Sprintf("localhost:%d", sv.RPCPort)
	addr := fmt.Sprintf(":%d", sv.RESTPort)
	var dialOpts []grpc.DialOption
	if sv.Options.CertFile == "" || sv.Options.KeyFile == "" {
		dialOpts = append(dialOpts, grpc.WithInsecure())
	} else {
		creds, err := credentials.NewClientTLSFromFile(sv.Options.CertFile, "")
		if err != nil {
			return err
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	}
	mux := runtime.NewServeMux(
		runtime.WithIncomingHeaderMatcher(headerMatcher),                                    // handle Accept-Language
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{OrigName: false}), // handle JSON camelcase
	)
	for name, provider := range sv.providers {
		if err := provider.RegisterHTTPProxy(ctx, mux, clientAddr, dialOpts); err != nil {
			log.Printf("server: failed to register reverse http proxy for '%s':%s", name, err)
		} else {
			log.Printf("server: registered reverse http proxy for '%s'", name)
		}
	}
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	// add gRPC-Web server
	wrappedGrpc := grpcweb.WrapServer(grpcServer)
	httpServer.Handler = http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		if wrappedGrpc.IsGrpcWebRequest(req) {
			wrappedGrpc.ServeHTTP(resp, req)
		}
		// Fall back to other servers.
		mux.ServeHTTP(resp, req)
	})

	// add CORS configuration
	httpServer.Handler = cors.Default().Handler(httpServer.Handler)

	// and now run the servers
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		log.Printf("server: gRPC Listening on %s\n", lis.Addr().String())
		return grpcServer.Serve(lis)
	})
	g.Go(func() error {
		if sv.Options.CertFile == "" || sv.Options.KeyFile == "" {
			log.Printf("server: http listening on %s (not using https: no certificate or key specified)", addr)
			return httpServer.ListenAndServe()
		}
		log.Printf("server: https listening on %s\n", addr)
		return httpServer.ListenAndServeTLS(sv.Options.CertFile, sv.Options.KeyFile)
	})
	select {
	case sig := <-sigs:
		log.Printf("server: received signal: %v", sig)
		break
	case <-ctx.Done():
		break
	}
	// graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if httpServer != nil {
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Print(err)
		}
	}
	if grpcServer != nil {
		grpcServer.GracefulStop()
		log.Print("server: grpc server shutdown")
	}
	return g.Wait()
}

// ensures GRPC gateway passes through the standard HTTP header Accept-Language as "accept-language"
// rather than munging the name prefixed with grpcgateway.
// delegates to default implementation for other headers.
func headerMatcher(headerName string) (mdName string, ok bool) {
	if headerName == "Accept-Language" {
		return "accept-language", true
	}
	return runtime.DefaultHeaderMatcher(headerName)
}

// Check is a health check, implementing the grpc-health service
// see https://godoc.org/google.golang.org/grpc/health/grpc_health_v1#HealthServer
func (sv *Server) Check(ctx context.Context, r *health.HealthCheckRequest) (*health.HealthCheckResponse, error) {
	response := new(health.HealthCheckResponse)
	response.Status = health.HealthCheckResponse_SERVING
	log.Printf("server: health check received: %s", response.Status)
	return response, nil
}

// Watch is a streaming health check to issue changes in health status
func (sv *Server) Watch(r *health.HealthCheckRequest, w health.Health_WatchServer) error {
	log.Printf("server: health watch request received but not implemented: %+v", r)
	return status.Error(codes.Unimplemented, "grpc health watch operation not implemented")
}
