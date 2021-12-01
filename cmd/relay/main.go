package main

// import (
// 	"context"
// 	"fmt"
// 	"log"
// 	"os"
// 	"os/signal"

// 	"github.com/smartcontractkit/chainlink-relay/core"
// 	"github.com/smartcontractkit/chainlink-relay/core/config"
// 	"github.com/smartcontractkit/chainlink-relay/core/server"
// 	"github.com/smartcontractkit/chainlink-relay/core/store"
// 	"github.com/smartcontractkit/solana-integration/pkg/solana"
// )

// func main() {
// 	// get configs from environment variables
// 	cfg, err := config.GetConfig()
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	// Initialize database connection
// 	var db store.Store
// 	if err := db.Connect(cfg.DatabaseURL()); err != nil {
// 		log.Fatal(err)
// 	}

// 	// Create a keystore
// 	fmt.Println("------------------------------------------")
// 	keys, pubKeys, err := store.KeystoreInit(db.DB, cfg.KeystorePassword())
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	// Load Solana components
// 	solClient, err := solana.NewConnectedClient(context.Background(), cfg.EthereumHTTPURL().String(), cfg.EthereumURL())
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	// // add solana keys to pubkeys struct
// 	// pubKeys["OCROnchainPublicKey"] = hex.EncodeToString(terraClient.Keys.OCR.PublicKey())
// 	// pubKeys["NodeAddress"] = terraClient.Keys.TX.Address()
// 	fmt.Println("------------------------------------------")

// 	// // start head tracker
// 	// if err := solClient.HeadTracker(); err != nil {
// 	// 	log.Fatal(err)
// 	// }

// 	// If empty OCR2_P2P_PEER_ID, set it using default pair
// 	if _, err := cfg.OCR2P2PPeerID(); err != nil {
// 		p2pKeys, err := keys.P2P().GetAll()
// 		if err != nil {
// 			log.Fatal(err)
// 		}
// 		os.Setenv("OCR2_P2P_PEER_ID", p2pKeys[0].ID())
// 	}

// 	// Load existing jobs from DB
// 	jobs, err := db.LoadJobs()
// 	if err != nil {
// 		log.Fatal(err)
// 	}

// 	// Start existing jobs (if present in db)
// 	services, err := core.NewServices(db.DB, cfg, keys, solClient)
// 	if err != nil {
// 		log.Fatal(err)
// 	}
// 	for _, j := range jobs {
// 		// start service from DB
// 		if err := services.Start(j); err != nil {
// 			services.Log.Errorf("[%s] Failed to start: %s", j.JobID, err)
// 		}
// 	}

// 	// start webserver connection
// 	go server.RunWebserver(cfg, &db, int(cfg.Port()), &services, &pubKeys)

// 	// exit gracefully (make sure services are stopped)
// 	sig := make(chan os.Signal, 1)
// 	signal.Notify(sig, os.Interrupt)
// 	<-sig
// 	services.Log.Info("Stopping services and exiting")
// 	services.StopAll() // close all services
// 	solClient.Close()  // close client
// 	os.Exit(0)
// }
