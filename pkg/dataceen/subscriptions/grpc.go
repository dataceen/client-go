package subscriptions

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	pb "github.com/dataceen/client-go/pkg/dataceenevent"
)

// hostFromURL converts a https://host[:port] URL into the host:port form
// google.golang.org/grpc.NewClient expects. https→443, http→80.
func hostFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("parse %q: %w", rawURL, err)
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		switch strings.ToLower(u.Scheme) {
		case "https":
			port = "443"
		default:
			port = "80"
		}
	}
	return host + ":" + port, nil
}

// streamClient is the minimal interface manager.go uses, so the manager can
// be unit-tested with a fake stream that doesn't open a real connection.
type streamClient interface {
	Subscribe(ctx context.Context, req *pb.SubscriptionRequest) (eventStream, error)
}

// eventStream models grpc.ClientStream[Recv] of pb.EventData.
type eventStream interface {
	Recv() (*pb.EventData, error)
}

// realStreamClient is the production implementation backed by a grpc.ClientConn.
type realStreamClient struct {
	conn   *grpc.ClientConn
	client pb.DataceenEventClient
	token  string
}

// dialSubscription opens a gRPC connection to the subscription service. The
// caller is responsible for closing conn when the subscription is done.
func dialSubscription(subscriptionURL, token string) (*grpc.ClientConn, streamClient, error) {
	host, err := hostFromURL(subscriptionURL)
	if err != nil {
		return nil, nil, err
	}
	creds := credentials.NewTLS(nil) // system roots, ALPN h2
	conn, err := grpc.NewClient(host, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, nil, fmt.Errorf("grpc.NewClient(%s): %w", host, err)
	}
	return conn, &realStreamClient{
		conn:   conn,
		client: pb.NewDataceenEventClient(conn),
		token:  token,
	}, nil
}

func (r *realStreamClient) Subscribe(ctx context.Context, req *pb.SubscriptionRequest) (eventStream, error) {
	ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+r.token)
	stream, err := r.client.Subscribe(ctx, req)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

// toProtoRequest builds the protobuf SubscriptionRequest from the public
// Request struct. Empty StartMode defaults to POSITION_BEGINNING.
func toProtoRequest(req Request) *pb.SubscriptionRequest {
	mode := req.StartMode
	if mode == "" {
		mode = StartPositionBeginning
	}
	pbMode, ok := pb.StartMode_value[string(mode)]
	if !ok {
		pbMode = int32(pb.StartMode_POSITION_BEGINNING)
	}
	return &pb.SubscriptionRequest{
		Domain:                     req.Domain,
		Model:                      req.Model,
		Scope:                      req.Scope,
		Baseloadtopics:             append([]string{}, req.BaseloadTopics...),
		Topics:                     append([]string{}, req.Topics...),
		Ids:                        append([]string{}, req.IDs...),
		Startmode:                  pb.StartMode(pbMode),
		Position:                   req.Position,
		Includedelta:               req.IncludeDelta,
		Includecomplete:            req.IncludeComplete,
		Baseloadcontinueobjecttype: req.BaseloadContinueObjectType,
		Positionbeforebaseload:     req.PositionBeforeBaseload,
	}
}
