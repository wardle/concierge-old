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

	"github.com/wardle/concierge/apiv1"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"
	health "google.golang.org/grpc/health/grpc_health_v1"
)

// Server represents a combined gRPC and REST server
type Server struct {
	Options
	apiv1.WalesEMPIServer
}

// Options defines the options for a server.
type Options struct {
	RPCPort  int
	RESTPort int
}

// RunServer runs a GRPC and a gateway REST server concurrently
func (sv *Server) RunServer() error {
	sigs := make(chan os.Signal, 1) // channel to receive OS termination/kill/interrupt signal
	signal.Notify(sigs, os.Interrupt, os.Kill, syscall.SIGTERM)
	go func() {
		s := <-sigs
		log.Printf("RECEIVED SIGNAL: %s", s)
		os.Exit(1)
	}()
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", sv.RPCPort))
	if err != nil {
		return fmt.Errorf("failed to initializa TCP listen: %v", err)
	}
	defer lis.Close()

	go func() {
		server := grpc.NewServer()
		health.RegisterHealthServer(server, sv)
		if sv.WalesEMPIServer != nil {
			log.Printf("registering Wales EMPI server implementation: %+v", sv.WalesEMPIServer)
			apiv1.RegisterWalesEMPIServer(server, sv.WalesEMPIServer)
		}
		log.Printf("gRPC Listening on %s\n", lis.Addr().String())
		log.Print(server.Serve(lis))
		os.Exit(1)
	}()
	clientAddr := fmt.Sprintf("localhost:%d", sv.RPCPort)
	addr := fmt.Sprintf(":%d", sv.RESTPort)
	dialOpts := []grpc.DialOption{grpc.WithInsecure()} // TODO:use better options
	mux := runtime.NewServeMux(runtime.WithIncomingHeaderMatcher(headerMatcher))
	if sv.WalesEMPIServer != nil {
		if apiv1.RegisterWalesEMPIHandlerFromEndpoint(ctx, mux, clientAddr, dialOpts); err != nil {
			return fmt.Errorf("failed to create HTTP reverse proxy: %v", err)
		}
	}
	log.Printf("HTTP Listening on %s\n", addr)
	return http.ListenAndServe(addr, mux)
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
	return response, nil
}

func (sv *Server) Watch(r *health.HealthCheckRequest, w health.Health_WatchServer) error {
	return nil
}
