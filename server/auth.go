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
	"github.com/sethvargo/go-password/password"
	"github.com/wardle/concierge/apiv1"
	"github.com/wardle/concierge/identifiers"
	"golang.org/x/crypto/bcrypt"
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
	jwtPrivatekey   *rsa.PrivateKey
	authProviders   map[string]AuthProvider
	serviceAccounts map[string]struct{}
}

// AuthProvider is a mechanism for plugging in modular authentication schemes
// for different namespaces.
type AuthProvider interface {
	Authenticate(id *apiv1.Identifier, credential string) (bool, error)
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
	return &Auth{
		jwtPrivatekey: parsedKey,
		authProviders: make(map[string]AuthProvider),
	}, nil
}

// NewAuthenticationServerWithTemporaryKey creates a new authentication server using an emphemeral private/public key pair
func NewAuthenticationServerWithTemporaryKey() (*Auth, error) {
	auth := new(Auth)
	var err error
	auth.jwtPrivatekey, err = rsa.GenerateKey(rand.Reader, 2048)
	auth.authProviders = make(map[string]AuthProvider)
	auth.serviceAccounts = make(map[string]struct{})
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

// Close closes any linked resources
func (auth *Auth) Close() error { return nil }

// RegisterAuthProvider registers an authentication provider for the given
func (auth *Auth) RegisterAuthProvider(uri string, name string, ap AuthProvider, service bool) {
	if _, exists := auth.authProviders[uri]; exists {
		panic("authentication provider already registered for uri: " + uri)
	}
	auth.authProviders[uri] = ap
	if service {
		auth.serviceAccounts[uri] = struct{}{}
	}
	log.Printf("auth: registered authentication provider for namespace uri: '%s': %s", uri, name)
}

var defaultTokenDuration = 5 * time.Minute
var serviceAccountTokenDuration = 72 * time.Hour

// Login performs an authentication.
// User account login can only be performed with an already logged in service account
// A service user login is currently performed using a user key and secret key, but could itself be from a third-party
// token in the future, depending on the namespace chosen.
func (auth *Auth) Login(ctx context.Context, r *apiv1.LoginRequest) (*apiv1.LoginResponse, error) {
	if auth.jwtPrivatekey == nil {
		return nil, status.Errorf(codes.Internal, "no private key specified for signing jwt token")
	}
	if _, found := auth.authProviders[r.GetUser().GetSystem()]; !found {
		log.Printf("auth: failed login attempt: unsupported namespace: '%s|%s'", r.GetUser().GetSystem(), r.GetUser().GetValue())
		return nil, status.Errorf(codes.Unauthenticated, "auth: unable to provide authentication for namespace uri '%s'", r.GetUser().GetSystem())
	}
	ap := auth.authProviders[r.GetUser().GetSystem()]
	log.Printf("auth: login attempt for '%s|%s'", r.GetUser().GetSystem(), r.GetUser().GetValue())
	if _, isService := auth.serviceAccounts[r.GetUser().GetSystem()]; !isService {
		ucd := GetContextData(ctx) // if ucd is nil, the next statement will still return false
		if _, isService = auth.serviceAccounts[ucd.GetAuthenticatedUser().GetSystem()]; !isService {
			log.Printf("auth: attempt to login without service account")
			return nil, status.Errorf(codes.Unauthenticated, "need service account login before logging in using normal user account")
		}
	}
	success, err := ap.Authenticate(r.GetUser(), r.GetPassword())
	if err != nil {
		log.Printf("auth: failed to authenticate: %s", err)
		return nil, status.Errorf(codes.Unauthenticated, "failed to authenticate: %s", err)
	}
	if !success {
		log.Printf("auth: invalid credentials for '%s|%s'", r.GetUser().GetSystem(), r.GetUser().GetValue())
		return nil, status.Errorf(codes.Unauthenticated, "invalid credentials")
	}
	tokenDuration := defaultTokenDuration
	if r.GetUser().GetSystem() == identifiers.ConciergeServiceUser {
		tokenDuration = serviceAccountTokenDuration
	}
	log.Printf("auth: generated authentication token for %s|%s: %v", r.GetUser().GetSystem(), r.GetUser().GetValue(), tokenDuration)
	ss, err := auth.generateToken(r.GetUser(), tokenDuration)
	if err != nil {
		log.Printf("auth: failed to generate token: %s", err)
		return nil, status.Errorf(codes.Internal, "could not generate token: %s", err)
	}
	return &apiv1.LoginResponse{Token: ss}, nil

}

// Refresh refreshes an authenitcation token
func (auth *Auth) Refresh(ctx context.Context, r *apiv1.TokenRefreshRequest) (*apiv1.LoginResponse, error) {
	ucd := GetContextData(ctx)
	// do we really need to refresh token? send old one back if there is plenty of time
	remaining := ucd.GetTokenExpiresAt().Sub(time.Now())
	if remaining > 5*time.Minute {
		log.Printf("auth: re-issuing still active token for '%s|%s' expiry:%v ", ucd.GetAuthenticatedUser().GetSystem(), ucd.GetAuthenticatedUser().GetValue(), ucd.GetTokenExpiresAt())
		return &apiv1.LoginResponse{Token: ucd.token}, nil
	}
	tokenDuration := defaultTokenDuration
	if ucd.authenticatedUser.GetSystem() == identifiers.ConciergeServiceUser {
		tokenDuration = serviceAccountTokenDuration
	}
	ss, err := auth.generateToken(ucd.authenticatedUser, tokenDuration)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "could not generate token: %s", err)
	}
	log.Printf("auth: generated refreshed authentication token for %s|%s (%v)", ucd.authenticatedUser.GetSystem(), ucd.authenticatedUser.GetValue(), tokenDuration)
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
	log.Printf("auth: invalid token: %s", err)
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
	ctx, err := sv.auth.addContextData(ctx)
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

// GenerateCredentials generates random credentials
// TODO: make it work a bit like https://docs.aws.amazon.com/cli/latest/reference/secretsmanager/get-random-password.html
func GenerateCredentials() (string, string, error) {
	p, err := password.Generate(64, 10, 0, false, true)
	if err != nil {
		return "", "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(p), bcrypt.DefaultCost)
	if err != nil {
		return "", "", err
	}
	return p, string(hash), nil
}

type singleAuthProvider struct {
	hash string
}

// NewSingleAuthProvider creates an authprovider for a static single password
func NewSingleAuthProvider(hash string) AuthProvider {
	return &singleAuthProvider{hash: hash}
}

func (ap *singleAuthProvider) Authenticate(id *apiv1.Identifier, credential string) (bool, error) {
	if err := bcrypt.CompareHashAndPassword([]byte(ap.hash), []byte(credential)); err != nil {
		return false, err
	}
	return true, nil
}
