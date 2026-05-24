// Spike 3: open a gRPC subscription stream and print the first events.
// Mirrors the C# DataceenClient.Subscribe + HandleSubscriptionEvent and the
// TypeScript spike 03-grpc-subscription.ts at the wire level.
//
// Prerequisite: run `make gen` first to produce the generated proto package.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/AzureAD/microsoft-authentication-library-for-go/apps/confidential"

	"github.com/dataceen/client-go/internal/spikecfg"
	pb "github.com/dataceen/client-go/pkg/dataceenevent"
)

const (
	maxEvents      = 10
	streamDeadline = 30 * time.Second
)

func acquireToken(ctx context.Context, cfg spikecfg.Config) (string, error) {
	cred, err := confidential.NewCredFromSecret(cfg.ClientSecret)
	if err != nil {
		return "", fmt.Errorf("cred: %w", err)
	}
	app, err := confidential.New(cfg.Authority(), cfg.ClientID, cred)
	if err != nil {
		return "", fmt.Errorf("confidential.New: %w", err)
	}
	res, err := app.AcquireTokenByCredential(ctx, []string{cfg.SubscriptionScope})
	if err != nil {
		return "", fmt.Errorf("AcquireTokenByCredential: %w", err)
	}
	return res.AccessToken, nil
}

func main() {
	cfg, err := spikecfg.Load()
	if err != nil {
		log.Fatalf("[spike-03] config: %v", err)
	}

	host, err := cfg.SubscriptionHost()
	if err != nil {
		log.Fatalf("[spike-03] host: %v", err)
	}

	tokenCtx, tokenCancel := context.WithTimeout(context.Background(), 30*time.Second)
	token, err := acquireToken(tokenCtx, cfg)
	tokenCancel()
	if err != nil {
		log.Fatalf("[spike-03] token: %v", err)
	}
	fmt.Println("[spike-03] Got token")

	tlsCreds := credentials.NewTLS(nil) // system roots, ALPN h2
	conn, err := grpc.NewClient(host, grpc.WithTransportCredentials(tlsCreds))
	if err != nil {
		log.Fatalf("[spike-03] dial: %v", err)
	}
	defer conn.Close()

	client := pb.NewDataceenEventClient(conn)

	streamCtx, cancel := context.WithTimeout(context.Background(), streamDeadline)
	defer cancel()
	streamCtx = metadata.AppendToOutgoingContext(streamCtx, "authorization", "Bearer "+token)

	req := &pb.SubscriptionRequest{
		Domain:                     cfg.Domain,
		Model:                      cfg.Model,
		Scope:                      cfg.Scope,
		Baseloadtopics:             []string{"Customer"},
		Topics:                     []string{"Customer"},
		Ids:                        []string{},
		Startmode:                  pb.StartMode_POSITION_BEGINNING,
		Position:                   "",
		Includedelta:               true,
		Includecomplete:            true,
		Baseloadcontinueobjecttype: "",
		Positionbeforebaseload:     "",
	}

	fmt.Printf("[spike-03] Connecting to %s\n", host)
	fmt.Printf("[spike-03] Subscribing to topics=%v\n", req.Topics)

	stream, err := client.Subscribe(streamCtx, req)
	if err != nil {
		log.Fatalf("[spike-03] Subscribe: %v", err)
	}

	received := 0
	for {
		evt, err := stream.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) {
				fmt.Printf("[spike-03] Stream ended. Events received: %d\n", received)
				break
			}
			if s, ok := status.FromError(err); ok && s.Code() == codes.Canceled {
				fmt.Printf("[spike-03] Stream cancelled (expected). Events received: %d\n", received)
				break
			}
			if s, ok := status.FromError(err); ok && s.Code() == codes.DeadlineExceeded {
				fmt.Printf("[spike-03] Timed out — received %d events\n", received)
				break
			}
			log.Fatalf("[spike-03] recv: %v", err)
		}

		received++
		fmt.Printf("[spike-03] event %d: type=%s msg=%s topic=%s id=%s pos=%s\n",
			received, evt.GetEventtype(), evt.GetMessagetype(), evt.GetTopic(), evt.GetId(), evt.GetPosition())

		if received >= maxEvents {
			cancel()
			break
		}
	}

	fmt.Println("[spike-03] OK")
}
