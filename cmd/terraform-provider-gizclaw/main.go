// Command terraform-provider-gizclaw is the GizClaw Terraform provider plugin.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/GizClaw/gizclaw-go/cmd/terraform-provider-gizclaw/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
)

// version is set to the GizClaw release version at build time.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with debugger support")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: provider.Address,
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}
