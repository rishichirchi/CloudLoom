package config

import (
	"context"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

var AWSConfig aws.Config

func InitAWS() {
	cfg, err := config.LoadDefaultConfig(context.TODO(), config.WithRegion("ap-south-1"))
	if err != nil {
		panic("unable to load SDK config, " + err.Error())
	}

	AWSConfig = cfg

	log.Println("AWS SDK Config loaded successfully")

}
