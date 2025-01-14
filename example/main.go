// The example program shows how to use the auth package.
//
// Before usage, credentials needs to be created.
// Go to the https://console.cloud.google.com/apis/credentials page and create an "OAuth 2.0 Client
// ID". The OAuth 2.0 client ID and secret should be passed using the 'client-id' and
// 'client-secret' flags.
// In the client ID configuration, the "Authorized Javascript origins" should contain
// http://localhost:8080 (or another URL address that this server is running at). And the
// "Authorized redirect URIs" should contain the same address with a "/auth" suffix - according to
// where the `auth.RedirectHandler()` is mounted in this code, and see that
// `auth.Config.OAuth2.RedirectURL` is configured accordingly.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/posener/auth"
	"golang.org/x/oauth2"
)

const (
	HttpReadTimeout     = 30 * time.Second
	HttpWriteTimeout    = 30 * time.Second
	HttpShutdownTimeout = 30 * time.Second
)

var (
	scheme       = flag.String("scheme", "http", "Server scheme of http or https")
	host         = flag.String("host", "localhost", "Server host")
	port         = flag.Int("port", 8080, "Server port")
	clientID     = flag.String("client-id", "", "Google OAuth 2.0 Client ID.")
	clientSecret = flag.String("client-secret", "", "Google OAuth 2.0 Client secret.")
	callbackPath = flag.String("callback-path", "auth", "Google OAuth 2.0 authorized redirect URI path.")
	authorized   = flag.String("authorized", "", "Authorized user.")
)

func main() {
	flag.Parse()

	// Create auth object.
	config := auth.Config{
		// Client credentials. As configured in
		// from https://console.cloud.google.com/apis/credentials at the "OAuth 2.0 Client IDs"
		// section.
		Config: oauth2.Config{
			// The redirect URL should be configured in the client config in google cloud console.
			RedirectURL:  fmt.Sprintf("%s://%s:%d/%s", *scheme, *host, *port, *callbackPath),
			ClientID:     *clientID,
			ClientSecret: *clientSecret,
			// https://developers.google.com/identity/protocols/oauth2/scopes#oauth2
			Scopes: []string{"https://www.googleapis.com/auth/userinfo.email", "https://www.googleapis.com/auth/userinfo.profile", "openid"},
		},
		Log:      log.Printf,
		Unsecure: true,
		Path:     "/",
	}
	a, err := auth.New(context.Background(), config)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Redirect URL: %v", config.Config.RedirectURL)
	log.Printf("Authorized user email: %q", *authorized)

	mux := http.NewServeMux()
	mux.Handle("/", a.Authenticate(http.HandlerFunc(handler)))
	mux.Handle("/"+*callbackPath, a.RedirectHandler())

	addr := fmt.Sprintf(":%d", *port)
	errC, err := run(mux, addr)
	if err != nil {
		log.Fatalf("run error: %s", err)
	}
	err = <-errC
	if err != nil {
		log.Fatalf("run server error: %s", err)
	}
}

// run http server and graceful shutdown
//
// resources:
// - https://mariocarrion.com/2021/05/21/golang-microservices-graceful-shutdown.html
func run(r http.Handler, address string) (<-chan error, error) {
	errC := make(chan error, 1)
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
		syscall.SIGQUIT)
	srv := &http.Server{
		Handler:      r,
		Addr:         address,
		ReadTimeout:  HttpReadTimeout,
		WriteTimeout: HttpWriteTimeout,
	}

	go func() {
		// wait for server termination and gracefully shutdown
		<-ctx.Done()
		log.Printf("Shutdown signal received")
		ctxTimeout, cancel := context.WithTimeout(context.Background(), HttpShutdownTimeout)
		defer func() {
			stop()
			cancel()
			close(errC)
		}()
		srv.SetKeepAlivesEnabled(false)
		err := srv.Shutdown(ctxTimeout)
		if err != nil {
			errC <- err
		}
		log.Printf("Shutdown completed")
	}()

	go func() {
		// run http server
		log.Printf("Listening and serving: %q", address)
		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			errC <- err
		}
		log.Printf("Exited server")
	}()

	return errC, nil
}

// handler is an example for http handler that is protected using Google authorization.
func handler(w http.ResponseWriter, r *http.Request) {
	// Get the authenticated user from the request context.
	user := auth.User(r.Context())

	if user == nil {
		// No user is logged in. This can only happen when the handler is not wrapped with
		// `auth.Authorize`.
		http.Error(w, "Not authorized", http.StatusUnauthorized)
		return
	}

	// The authenticated user can be authorized according to the email, which identifies the
	// account.
	if *authorized != "" && *authorized != user.Email {
		// The logged in user is not allowed for this page.
		http.Error(w, fmt.Sprintf("User %s not allowed", user.Email), http.StatusForbidden)
		return
	}

	// User is allowed, greet them.
	fmt.Fprintf(w, "Hello, %s", user.Name)
}
