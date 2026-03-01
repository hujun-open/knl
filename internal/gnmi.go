package internal

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/netip"
	"strings"
	"time"

	"github.com/openconfig/gnmi/proto/gnmi"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

const (
	DefaultgNMIPort = 57400
)

type GNMIRPC struct {
	clnt gnmi.GNMIClient
	conn *grpc.ClientConn
	user string
	pass string
}

func (gnmirpc *GNMIRPC) Close() {
	gnmirpc.conn.Close()
}

func NewGNMIRPC(target netip.AddrPort, user, pass string, useTLS bool) *GNMIRPC {
	var conn *grpc.ClientConn
	var err error
	if useTLS {
		creds := credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})
		conn, err = grpc.NewClient(target.String(), grpc.WithTransportCredentials(creds))
	} else {
		conn, err = grpc.NewClient(target.String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	client := gnmi.NewGNMIClient(conn)
	return &GNMIRPC{
		clnt: client,
		conn: conn,
		user: user,
		pass: pass,
	}
}

func (gnmirpc *GNMIRPC) GetConfig() (string, error) {
	req := &gnmi.GetRequest{
		Path: []*gnmi.Path{
			{
				Elem: []*gnmi.PathElem{},
			},
		},
		Type:     gnmi.GetRequest_CONFIG, // Requesting running config only
		Encoding: gnmi.Encoding_JSON_IETF,
	}

	// 4. Add Authentication Metadata
	ctx := metadata.AppendToOutgoingContext(context.Background(),
		"username", gnmirpc.user,
		"password", gnmirpc.pass)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	res, err := gnmirpc.clnt.Get(ctx, req)
	if err != nil {
		return "", fmt.Errorf("Error getting config: %w", err)
	}
	if len(res.Notification) == 0 {
		return "", fmt.Errorf("invalid response contain zero notification")
	}
	if len(res.Notification[0].Update) == 0 {
		return "", fmt.Errorf("invalid response 1st notificaton contains zero update")
	}
	rawJson := res.Notification[0].Update[0].Val.GetJsonIetfVal()
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, rawJson, "", "  "); err != nil {
		return "", err
	}
	return prettyJSON.String(), nil
}

func (gnmirpc *GNMIRPC) LoadJsonCfg(root, input string) error {
	paths := []*gnmi.PathElem{}
	if strings.TrimSpace(root) != "" {
		flist := strings.Fields(strings.TrimSpace(root))
		for _, f := range flist {
			paths = append(paths, &gnmi.PathElem{Name: f})
		}
	}
	req := &gnmi.SetRequest{
		Update: []*gnmi.Update{
			{
				Path: &gnmi.Path{Elem: paths},
				Val: &gnmi.TypedValue{
					Value: &gnmi.TypedValue_JsonIetfVal{
						JsonIetfVal: []byte(input),
					},
				},
			},
		},
	}
	ctx := metadata.AppendToOutgoingContext(context.Background(),
		"username", gnmirpc.user, "password", gnmirpc.pass)
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	_, err := gnmirpc.clnt.Set(ctx, req)
	return err
}
