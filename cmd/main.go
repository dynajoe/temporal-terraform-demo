package main

import (
	"log"
	"os"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/dynajoe/temporal-terraform-demo/workflows"
)

func main() {
	serviceClient, err := client.NewClient(client.Options{
		Namespace: "default",
		HostPort:  os.Getenv("TEMPORAL_TF_DEMO_ADDR"),
	})
	if err != nil {
		log.Fatal(err.Error())
	}

	temporalWorker := worker.New(serviceClient, "temporal-terraform-demo", worker.Options{
		WorkerStopTimeout: 30 * time.Second,
	})

	log.Print("registering workflows")
	workflows.Register(temporalWorker)

	if err := temporalWorker.Run(worker.InterruptCh()); err != nil {
		log.Fatalln("unable to start Worker", err)
	}
}
