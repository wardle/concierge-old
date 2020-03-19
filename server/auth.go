package server

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"strings"
	"time"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/wardle/concierge/apiv1"
	"github.com/wardle/concierge/identifiers"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	// ErrInvalidToken means that there was an invalid or missing authorization token
	ErrInvalidToken = errors.New("invalid authorization token")
)

// Auth is an authentication server
type Auth struct {
	jwtPrivatekey *rsa.PrivateKey
}

// NewAuthenticationServer creates a new authentication server that can issue JWT tokens
func NewAuthenticationServer(rsaPrivateKey string) (*Auth, error) {
	key, err := ioutil.ReadFile(rsaPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("error reading jwt private key: %w", err)
	}
	parsedKey, err := jwt.ParseRSAPrivateKeyFromPEM(key)
	if err != nil {
		return nil, fmt.Errorf("error parsing jwt private key: %w", err)
	}
	return &Auth{jwtPrivatekey: parsedKey}, nil
}

// NewAuthenticationServerWithTemporaryKey creates a new authentication server using an emphemeral private/public key pair
func NewAuthenticationServerWithTemporaryKey() (*Auth, error) {
	auth := new(Auth)
	var err error
	auth.jwtPrivatekey, err = rsa.GenerateKey(rand.Reader, 2048)
	return auth, err
}

var _ apiv1.AuthenticatorServer = (*Auth)(nil)

// RegisterServer registers this server
func (auth *Auth) RegisterServer(s *grpc.Server) {
	apiv1.RegisterAuthenticatorServer(s, auth)
}

// RegisterHTTPProxy registers this as a reverse HTTP proxy
func (auth *Auth) RegisterHTTPProxy(ctx context.Context, mux *runtime.ServeMux, endpoint string, opts []grpc.DialOption) error {
	return apiv1.RegisterAuthenticatorHandlerFromEndpoint(ctx, mux, endpoint, opts)
}

// Login performs an authentication.
// A service user login is currently performed using a user key and secret key, but could itself be from a third-party
// token in the future, depending on the namespace chosen.
func (auth *Auth) Login(ctx context.Context, r *apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
	if auth.jwtPrivatekey == nil {
		return nil, status.Errorf(codes.Internal, "no private key specified for signing jwt token")
	}
	log.Printf("auth: login attempt: %s|%s", r.GetUser().GetSystem(), r.GetUser().GetValue())
	if r.GetUser().GetSystem() == identifiers.ConciergeServiceUser {
		if authenticateServiceUser(r.GetUser().GetValue(), r.GetPassword()) == false { // TODO: use a better check ;)
			log.Printf("auth: failed login for service user '%s' : invalid credentials", r.GetUser().Value)
			return nil, status.Errorf(codes.PermissionDenied, "invalid user key and secret key")
		}
		log.Printf("auth: successful login for service user '%s'", r.GetUser().GetValue())
		ss, err := auth.generateToken(r.GetUser(), time.Hour*72)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "could not generate token: %s", err)
		}
		return &apiv1.LoginResponse{Token: ss}, nil
	}

	return nil, status.Errorf(codes.Unimplemented, "auth: authentication of non-service users not yet implemented")
}

// Refresh refreshes an authenitcation token
func (auth *Auth) Refresh(ctx context.Context, r *apiv1.TokenRefreshRequest) (*apiv1.LoginResponse, error) {
	ucd := GetContextData(ctx)
	// do we really need to refresh token? send old one back if there is plenty of time
	if ucd.GetTokenExpiresAt().After(time.Now().Add(5 * time.Minute)) {
		log.Printf("auth: existing token for %s|%s expires %v so no need to refresh yet", ucd.GetAuthenticatedUser().GetSystem(), ucd.GetAuthenticatedUser().GetValue(), ucd.GetTokenExpiresAt())
		return &apiv1.LoginResponse{Token: ucd.token}, nil
	}
	tokenDuration := 5 * time.Minute
	if ucd.authenticatedUser.GetSystem() == identifiers.ConciergeServiceUser {
		tokenDuration = 72 * time.Hour
	}
	ss, err := auth.generateToken(ucd.authenticatedUser, tokenDuration)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not generate token: %s", err)
	}
	log.Printf("auth: generated refreshed authentication token for %s|%s", ucd.authenticatedUser.GetSystem(), ucd.authenticatedUser.GetValue())
	return &apiv1.LoginResponse{Token: ss}, nil
}

func (auth *Auth) generateToken(id *apiv1.Identifier, duration time.Duration) (string, error) {
	claims := &jwt.StandardClaims{
		ExpiresAt: time.Now().Add(duration).Unix(),
		IssuedAt:  time.Now().Unix(),
		Subject:   id.GetSystem() + "|" + id.GetValue(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	return token.SignedString(auth.jwtPrivatekey)
}

func (auth *Auth) parseToken(token string) (*UserContextData, error) {
	const bearerSchema = "Bearer "
	if strings.HasPrefix(token, bearerSchema) {
		token = token[len(bearerSchema):]
	}
	jwtToken, err := jwt.ParseWithClaims(token, &jwt.StandardClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			log.Printf("auth: unexpected signing method: %v", t.Header["alg"])
			return nil, ErrInvalidToken
		}
		return &auth.jwtPrivatekey.PublicKey, nil
	})
	if err == nil && jwtToken.Valid {
		claims := jwtToken.Claims.(*jwt.StandardClaims)
		cd := new(UserContextData)
		ids := strings.Split(claims.Subject, "|")
		if len(ids) != 2 {
			return nil, ErrInvalidToken
		}
		cd.authenticatedUser = &apiv1.Identifier{System: ids[0], Value: ids[1]}
		cd.token = token
		cd.tokenExpiresAt = time.Unix(claims.ExpiresAt, 0)
		return cd, nil
	}
	return nil, err
}

// contextKey is a concierge server key for values in a context
type contextKey string

const (
	userContextKey = contextKey("user")
)

// UserContextData is stored in the context
type UserContextData struct {
	authenticatedUser *apiv1.Identifier
	token             string
	tokenExpiresAt    time.Time
}

// GetAuthenticatedUser returns the authenticated user, guarding against nils
func (ucd *UserContextData) GetAuthenticatedUser() *apiv1.Identifier {
	if ucd == nil {
		return nil
	}
	return ucd.authenticatedUser
}

// GetTokenExpiresAt returns the token expiry time, guarding against nils
func (ucd *UserContextData) GetTokenExpiresAt() time.Time {
	if ucd == nil {
		return time.Time{}
	}
	return ucd.tokenExpiresAt
}

var noAuthEndpoints = map[string]struct{}{
	"/apiv1.Authenticator/Login":   struct{}{},
	"/grpc.health.v1.Health/Check": struct{}{},
}

// unaryAuthInterceptor provides an interceptor that ensures we have an authenticated user
func (sv *Server) unaryAuthInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	ctx, err := sv.Auth.addContextData(ctx)
	if err == nil {
		return handler(ctx, req)
	}
	if _, found := noAuthEndpoints[info.FullMethod]; found {
		return handler(ctx, req)
	}
	log.Printf("server: unauthenticated call to '%s': %s", info.FullMethod, err)
	return nil, status.Errorf(codes.Unauthenticated, "unauthenticated: %s", err)
}

func (sv *Server) streamAuthInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	return fmt.Errorf("not implemented")
}

// addContextData returns a new context containing UserContextData specifically
//  returning the old context in the event of an error
func (auth *Auth) addContextData(ctx context.Context) (context.Context, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ctx, fmt.Errorf("invalid token")
	}
	tokenString, ok := md["authorization"]
	if !ok {
		return ctx, fmt.Errorf("invalid token")
	}
	user, err := auth.parseToken(tokenString[0])
	if err != nil {
		return ctx, err
	}
	return context.WithValue(ctx, userContextKey, user), nil
}

// GetContextData is a convenience function to get injected contextual data
func GetContextData(ctx context.Context) *UserContextData {
	if v := ctx.Value(userContextKey); v != nil {
		if ucd, ok := v.(*UserContextData); ok {
			return ucd
		}
	}
	return nil
}

func authenticateServiceUser(userKey string, secretKey string) bool {
	if userKey == secretKey {
		return true
	}
	return false
}
