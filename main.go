package main

import (

	// Import the generated protobuf code
	"errors"
	"fmt"
	"log"

	"os"

	"golang.org/x/net/context"

	pb "github.com/EwanValentine/shippy-consignment-service/proto/consignment"
	userService "github.com/EwanValentine/shippy-user-service/proto/auth"
	vesselProto "github.com/EwanValentine/shippy-vessel-service/proto/vessel"
	"github.com/micro/go-micro"
	"github.com/micro/go-micro/metadata"
	"github.com/micro/go-micro/server"
	k8s "github.com/micro/kubernetes/go/micro"
)

const (
	defaultHost = "localhost:27017"
)

var (
	srv micro.Service
)

func main() {

	// Database host from the environment variables
	host := os.Getenv("DB_HOST")

	if host == "" {
		host = defaultHost
	}

	session, err := CreateSession(host)

	// Mgo creates a 'master' session, we need to end that session
	// before the main function closes.
	defer session.Close()

	if err != nil {

		// We're wrapping the error returned from our CreateSession
		// here to add some context to the error.
		log.Panicf("Could not connect to datastore with host %s - %v", host, err)
	}

	// Create a new service. Optionally include some options here.
	srv = k8s.NewService(

		// This name must match the package name given in your protobuf definition
		micro.Name("shippy.consignment"),
		micro.Version("latest"),
		micro.WrapHandler(AuthWrapper),
	)

	vesselClient := vesselProto.NewVesselServiceClient("shippy.vessel", srv.Client())

	// Init will parse the command line flags.
	srv.Init()

	// Register handler
	pb.RegisterConsignmentServiceHandler(srv.Server(), &service{session, vesselClient})

	// Run the server
	if err := srv.Run(); err != nil {
		fmt.Println(err)
	}
}

// AuthWrapper is a high-order function which takes a HandlerFunc
// and returns a function, which takes a context, request and response interface.
// The token is extracted from the context set in our consignment-cli, that
// token is then sent over to the user service to be validated.
// If valid, the call is passed along to the handler. If not,
// an error is returned.
func AuthWrapper(fn server.HandlerFunc) server.HandlerFunc {
	return func(ctx context.Context, req server.Request, resp interface{}) error {
		if os.Getenv("DISABLE_AUTH") == "true" {
			return fn(ctx, req, resp)
		}
		meta, ok := metadata.FromContext(ctx)
		if !ok {
			return errors.New("no auth meta-data found in request")
		}

		// Note this is now uppercase (not entirely sure why this is...)
		token := meta["Token"]
		log.Println("Authenticating with token: ", token)

		// Auth here
		// Really shouldn't be using a global here, find a better way
		// of doing this, since you can't pass it into a wrapper.
		authClient := userService.NewAuthClient("shippy.user", srv.Client())
		_, err := authClient.ValidateToken(ctx, &userService.Token{
			Token: token,
		})
		if err != nil {
			return err
		}
		err = fn(ctx, req, resp)
		return err
	}
}
