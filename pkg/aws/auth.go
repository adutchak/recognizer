package aws

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/rekognition"
)

func GetRekognitionClient() (*rekognition.Client, error) {
	defaultConfig, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		return nil, err
	}
	client := rekognition.NewFromConfig(defaultConfig)
	return client, nil
}
