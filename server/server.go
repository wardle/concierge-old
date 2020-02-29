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

	"github.com/wardle/concierge/apiv1"
	"golang.org/x/sync/errgroup"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	health "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
)

// Server represents a combined gRPC and REST server
type Server struct {
	Options
	// modules supported by this server:
	apiv1.WalesEMPIServer
}

// Options defines the options for a server.
type Options struct {
	RPCPort  int
	RESTPort int
	CertFile string
	KeyFile  string
}

// RunServer runs a GRPC and a gateway REST server concurrently
func (sv *Server) RunServer() error {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigs := make(chan os.Signal, 1) // channel to receive OS termination/kill/interrupt signal so we can log
	signal.Notify(sigs, os.Interrupt, os.Kill, syscall.SIGTERM)
	defer signal.Stop(sigs)

	g, ctx := errgroup.WithContext(ctx)

	// run gRPC server
	var grpcServer *grpc.Server
	g.Go(func() error {
		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", sv.RPCPort))
		if err != nil {
			return fmt.Errorf("failed to initialize TCP listen: %v", err)
		}
		defer lis.Close()
		grpcServer = grpc.NewServer()
		health.RegisterHealthServer(grpcServer, sv)
		if sv.WalesEMPIServer != nil {
			log.Printf("registering Wales EMPI module: %+v", sv.WalesEMPIServer)
			apiv1.RegisterWalesEMPIServer(grpcServer, sv.WalesEMPIServer)
		}
		log.Printf("gRPC Listening on %s\n", lis.Addr().String())
		return grpcServer.Serve(lis)
	})

	// run HTTP gateway
	var httpServer *http.Server
	g.Go(func() error {
		clientAddr := fmt.Sprintf("localhost:%d", sv.RPCPort)
		addr := fmt.Sprintf(":%d", sv.RESTPort)
		dialOpts := []grpc.DialOption{grpc.WithInsecure()} // TODO:use better options
		mux := runtime.NewServeMux(
			runtime.WithIncomingHeaderMatcher(headerMatcher),                                    // handle Accept-Language
			runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{OrigName: false}), // handle JSON camelcase
		)
		if sv.WalesEMPIServer != nil {
			if err := apiv1.RegisterWalesEMPIHandlerFromEndpoint(ctx, mux, clientAddr, dialOpts); err != nil {
				return fmt.Errorf("failed to create http reverse proxy: %v", err)
			}
		}
		httpServer = &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 10 * time.Second,
		}
		if sv.Options.CertFile == "" || sv.Options.KeyFile == "" {
			log.Printf("warning: http listening on %s (no certificate or key specified)", addr)
			return httpServer.ListenAndServe()
		}
		log.Printf("https listening on %s\n", addr)
		return httpServer.ListenAndServeTLS(sv.Options.CertFile, sv.Options.KeyFile)
	})

	select {
	case sig := <-sigs:
		log.Printf("received signal: %v", sig)
		break
	case <-ctx.Done():
		break
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if httpServer != nil {
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Print(err)
		}
	}
	if grpcServer != nil {
		grpcServer.GracefulStop()
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
	log.Printf("service health check received: %s", response.Status)
	return response, nil
}

// Watch is a streaming health check to issue changes in health status
func (sv *Server) Watch(r *health.HealthCheckRequest, w health.Health_WatchServer) error {
	log.Printf("service health watch request received but not implemented: %+v", r)
	return status.Error(codes.Unimplemented, "grpc health watch operation not implemented")
}
