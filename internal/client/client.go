package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/rakyll/ate-watch/pkg/proto/ateapipb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// ActorLister defines the interface for querying actors from the control plane.
type ActorLister interface {
	ListActors(ctx context.Context, in *ateapipb.ListActorsRequest, opts ...grpc.CallOption) (*ateapipb.ListActorsResponse, error)
	ReadActor(ctx context.Context, in *ateapipb.GetActorRequest, opts ...grpc.CallOption) (*ateapipb.Actor, error)
	ListAllActors(ctx context.Context, atespace string) ([]*ateapipb.Actor, error)
	Endpoint() string
	Close() error
}

// Options configures the connection to the Substrate control plane.
type Options struct {
	// KubeconfigPath is the path to the kubeconfig file.
	KubeconfigPath string

	// KubeContext is the Kubernetes context name.
	KubeContext string

	// Endpoint is a manual override for the gRPC control plane address.
	// If omitted, ate-watch automatically port-forwards to the in-cluster
	// ate-api-server pod using the Kubernetes configuration.
	Endpoint string

	// SkipVerify skips TLS certificate verification.
	SkipVerify bool

	// Insecure connects over plaintext HTTP/2 without TLS.
	Insecure bool

	// TLSConfig overrides the TLS configuration for the control plane connection.
	TLSConfig *tls.Config

	// BearerTokenFile is a path to a file containing a bearer token.
	BearerTokenFile string

	// BearerToken is a direct bearer token string.
	BearerToken string
}

// Client manages the connection (direct or port-forwarded) to the Substrate control plane.
type Client struct {
	opts     Options
	conn     *grpc.ClientConn
	control  ateapipb.ControlClient
	cancel   func()
	endpoint string
}

// New creates a Client connected to the Substrate control plane.
// If opts.Endpoint is specified, it dials directly.
// Otherwise, it uses the Kubernetes configuration (and kubecontext) to port-forward
// to the ate-api-server pod in the ate-system namespace.
func New(ctx context.Context, opts Options) (*Client, error) {
	if opts.Endpoint != "" {
		return dialDirect(opts)
	}
	return dialPortForward(ctx, opts)
}

func dialDirect(opts Options) (*Client, error) {
	var dialOpts []grpc.DialOption

	if opts.Insecure {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		var creds credentials.TransportCredentials
		if opts.TLSConfig != nil {
			creds = credentials.NewTLS(opts.TLSConfig)
		} else {
			creds = credentials.NewTLS(&tls.Config{InsecureSkipVerify: opts.SkipVerify})
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	}

	if opts.BearerTokenFile != "" {
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(fileTokenCreds{path: opts.BearerTokenFile}))
	} else if opts.BearerToken != "" {
		dialOpts = append(dialOpts, grpc.WithPerRPCCredentials(staticTokenCreds{token: opts.BearerToken}))
	}

	conn, err := grpc.NewClient(opts.Endpoint, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("ate-watch: dialing control plane at %s: %w", opts.Endpoint, err)
	}

	return &Client{
		opts:     opts,
		conn:     conn,
		control:  ateapipb.NewControlClient(conn),
		cancel:   func() {},
		endpoint: opts.Endpoint,
	}, nil
}

// LoadConfig loads the Kubernetes client configuration for a given path and context.
func LoadConfig(kubeconfigPath, k8sContext string) (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.ExplicitPath = kubeconfigPath
	configOverrides := &clientcmd.ConfigOverrides{CurrentContext: k8sContext}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides).ClientConfig()
}

func dialPortForward(ctx context.Context, opts Options) (*Client, error) {
	config, err := LoadConfig(opts.KubeconfigPath, opts.KubeContext)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create k8s client: %w", err)
	}

	// Look up the 'api' Service in ate-system to get its selector
	svc, err := clientset.CoreV1().Services("ate-system").Get(ctx, "api", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get api service in ate-system namespace: %w", err)
	}
	selector := labels.SelectorFromSet(svc.Spec.Selector).String()

	// Find the pods backing the service
	pods, err := clientset.CoreV1().Pods("ate-system").List(ctx, metav1.ListOptions{
		LabelSelector: selector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list ateapi pods: %w", err)
	}
	if len(pods.Items) == 0 {
		return nil, fmt.Errorf("no ate-api-server pods found in ate-system namespace")
	}
	targetPod := pods.Items[0]

	// Setup port-forwarding
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(targetPod.Namespace).
		Name(targetPod.Name).
		SubResource("portforward")

	transport, upgrader, err := spdy.RoundTripperFor(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create SPDY transport: %w", err)
	}

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, req.URL())

	stopCh := make(chan struct{})
	readyCh := make(chan struct{})

	ports := []string{"0:443"} // Port 0 asks OS for random available local port

	fw, err := portforward.New(dialer, ports, stopCh, readyCh, io.Discard, io.Discard)
	if err != nil {
		return nil, fmt.Errorf("failed to create port forwarder: %w", err)
	}

	errCh := make(chan error, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := fw.ForwardPorts(); err != nil {
			errCh <- fmt.Errorf("port forwarding failed: %w", err)
		}
	}()

	select {
	case <-readyCh:
	case err := <-errCh:
		return nil, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	forwardedPorts, err := fw.GetPorts()
	if err != nil || len(forwardedPorts) == 0 {
		close(stopCh)
		return nil, fmt.Errorf("failed to get forwarded ports: %w", err)
	}

	localPort := forwardedPorts[0].Local
	localEndpoint := fmt.Sprintf("127.0.0.1:%d", localPort)

	transportCreds := credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})
	grpcOpts := []grpc.DialOption{grpc.WithTransportCredentials(transportCreds)}

	jwtOpts, err := jwtDialOptions(ctx, clientset)
	if err != nil {
		close(stopCh)
		return nil, err
	}
	grpcOpts = append(grpcOpts, jwtOpts...)

	conn, err := grpc.NewClient(localEndpoint, grpcOpts...)
	if err != nil {
		close(stopCh)
		return nil, fmt.Errorf("failed to dial gRPC over tunnel: %w", err)
	}

	displayEndpoint := localEndpoint
	if opts.KubeContext != "" {
		displayEndpoint = fmt.Sprintf("%s (context: %s)", localEndpoint, opts.KubeContext)
	}

	return &Client{
		opts:     opts,
		conn:     conn,
		control:  ateapipb.NewControlClient(conn),
		endpoint: displayEndpoint,
		cancel: func() {
			close(stopCh)
			wg.Wait()
		},
	}, nil
}

func jwtDialOptions(ctx context.Context, clientset *kubernetes.Clientset) ([]grpc.DialOption, error) {
	jwtMode, err := isJWTMode(ctx, clientset)
	if err != nil {
		return nil, err
	}
	if !jwtMode {
		return nil, nil
	}

	expirationSeconds := int64(3600)
	tokenRequest := &authv1.TokenRequest{
		Spec: authv1.TokenRequestSpec{
			Audiences:         []string{"api.ate-system.svc"},
			ExpirationSeconds: &expirationSeconds,
		},
	}
	token, err := clientset.CoreV1().ServiceAccounts("ate-system").CreateToken(ctx, "ate-client", tokenRequest, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to request ateapi bearer token: %w", err)
	}
	if token.Status.Token == "" {
		return nil, fmt.Errorf("failed to request ateapi bearer token: token response was empty")
	}
	return []grpc.DialOption{grpc.WithPerRPCCredentials(staticTokenCreds{token: token.Status.Token})}, nil
}

func isJWTMode(ctx context.Context, clientset *kubernetes.Clientset) (bool, error) {
	deployment, err := clientset.AppsV1().Deployments("ate-system").Get(ctx, "ate-api-server", metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to get ate-api-server deployment: %w", err)
	}
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name != "ate-api-server" {
			continue
		}
		return isJWTAuthModeArg(container.Args), nil
	}
	return false, fmt.Errorf("failed to find ate-api-server container in deployment")
}

func isJWTAuthModeArg(args []string) bool {
	for i, arg := range args {
		if arg == "--auth-mode=jwt" {
			return true
		}
		if strings.HasPrefix(arg, "--auth-mode=") {
			return strings.TrimPrefix(arg, "--auth-mode=") == "jwt"
		}
		if arg == "--auth-mode" && i+1 < len(args) {
			return args[i+1] == "jwt"
		}
	}
	return false
}

// Endpoint returns the connected endpoint description.
func (c *Client) Endpoint() string {
	return c.endpoint
}

// Close closes the underlying gRPC connection and stops any port-forwarding.
func (c *Client) Close() error {
	var err error
	if c.conn != nil {
		err = c.conn.Close()
	}
	if c.cancel != nil {
		c.cancel()
	}
	return err
}

// Control returns the raw gRPC ControlClient.
func (c *Client) Control() ateapipb.ControlClient {
	return c.control
}

// ListActors forwards to the control plane ListActors RPC.
func (c *Client) ListActors(ctx context.Context, in *ateapipb.ListActorsRequest, opts ...grpc.CallOption) (*ateapipb.ListActorsResponse, error) {
	return c.control.ListActors(ctx, in, opts...)
}

// ReadActor forwards to the control plane GetActor RPC.
func (c *Client) ReadActor(ctx context.Context, in *ateapipb.GetActorRequest, opts ...grpc.CallOption) (*ateapipb.Actor, error) {
	return c.control.GetActor(ctx, in, opts...)
}

// ListAllActors retrieves all actors matching the given atespace, handling pagination automatically.
func (c *Client) ListAllActors(ctx context.Context, atespace string) ([]*ateapipb.Actor, error) {
	var allActors []*ateapipb.Actor
	pageToken := ""

	for {
		resp, err := c.control.ListActors(ctx, &ateapipb.ListActorsRequest{
			Atespace:  atespace,
			PageSize:  1000,
			PageToken: pageToken,
		})
		if err != nil {
			return nil, err
		}

		allActors = append(allActors, resp.GetActors()...)
		pageToken = resp.GetNextPageToken()
		if pageToken == "" {
			break
		}
	}

	return allActors, nil
}

// fileTokenCreds sends the contents of a file as a bearer token on each RPC.
type fileTokenCreds struct {
	path string
}

func (c fileTokenCreds) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	b, err := os.ReadFile(c.path)
	if err != nil {
		return nil, fmt.Errorf("reading bearer token file: %w", err)
	}
	token := strings.TrimSpace(string(b))
	if token == "" {
		return nil, errors.New("bearer token file is empty")
	}
	return map[string]string{"authorization": "Bearer " + token}, nil
}

func (c fileTokenCreds) RequireTransportSecurity() bool {
	return false
}

// staticTokenCreds sends a static bearer token on each RPC.
type staticTokenCreds struct {
	token string
}

func (c staticTokenCreds) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	token := strings.TrimSpace(c.token)
	if token == "" {
		return nil, errors.New("bearer token is empty")
	}
	return map[string]string{"authorization": "Bearer " + token}, nil
}

func (c staticTokenCreds) RequireTransportSecurity() bool {
	return false
}
