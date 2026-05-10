// Minimal pb-synadia example.
//
// Set SYNADIA_SYSTEM_ID and SYNADIA_API_TOKEN in the environment, run:
//
//	go run ./examples/basic serve
//
// then visit http://localhost:8090/_/ to log in as a superuser. Create an
// account record in nats_accounts; pb-synadia will provision it on Synadia
// and populate synadia_account_id. Create a role and a user; the user
// record will be populated with creds_file you can download.
package main

import (
	"log"
	"os"

	"github.com/pocketbase/pocketbase"
	pbsynadia "github.com/skeeeon/pb-synadia"
)

func main() {
	app := pocketbase.New()

	opts := pbsynadia.DefaultOptions()
	opts.SystemID = os.Getenv("SYNADIA_SYSTEM_ID")
	opts.APIToken = os.Getenv("SYNADIA_API_TOKEN")

	if err := pbsynadia.Setup(app, opts); err != nil {
		log.Fatalf("pb-synadia setup failed: %v", err)
	}

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
