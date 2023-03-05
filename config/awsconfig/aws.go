package awsconfig

import (
	"context"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

func LoadConfig() aws.Config {
	awsConfig, err := config.LoadDefaultConfig(context.Background(), config.WithSharedConfigProfile(os.Getenv("TEMPORAL_TF_DEMO_AWS_PROFILE")))
	if err != nil {
		log.Fatal("unable to load aws config")
	}
	return awsConfig
}
