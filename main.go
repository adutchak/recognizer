package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/adutchak/recognizer/pkg/aws"
	"github.com/adutchak/recognizer/pkg/config"
	"github.com/adutchak/recognizer/pkg/logging"
	"github.com/adutchak/recognizer/pkg/mqttclient"

	"github.com/aws/aws-sdk-go-v2/service/rekognition"
	"github.com/aws/aws-sdk-go-v2/service/rekognition/types"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	mqtt "github.com/eclipse/paho.mqtt.golang"
)

func main() {
	ctx := context.Background()
	l := logging.WithContext(ctx)
	configuration, err := config.NewConfig()
	if err != nil {
		l.Fatalf("could not load the configuration, %v", err)
	}
	l.Infof("loaded config %s", awsutil.Prettify(configuration))

	recognizeClient, err := aws.GetRekognitionClient()
	if err != nil {
		message := "cannot initialize AWS recognize client"
		l.Fatal(message)
		return
	}

	var rekognitionInputs []map[string]rekognition.CompareFacesInput
	for _, sample := range configuration.SampleImagePaths {
		targeBytes, err := os.ReadFile(sample)
		if err != nil {
			l.Errorf("Error reading file %s: %v", sample, err)
			return
		}

		targetImage := types.Image{
			Bytes: targeBytes,
		}

		compareFacesInput := rekognition.CompareFacesInput{
			TargetImage:         &targetImage,
			SimilarityThreshold: &configuration.SimilarityThreshold,
			QualityFilter:       "AUTO",
		}
		element := map[string]rekognition.CompareFacesInput{
			sample: compareFacesInput,
		}
		rekognitionInputs = append(rekognitionInputs, element)
	}
	l.Info("starting recognizer")

	mqttClient, err := mqttclient.GetMqttClient(configuration)
	if err != nil {
		l.Errorf("Error getting mqttClient %v", err)
	}
	doneChan := make(chan bool)
	go func(doneChan chan bool) {
		defer func() {
			doneChan <- true
		}()
		for {
			err := waitForFile(configuration.TargetImagePath)
			if err != nil {
				l.Error(err)
				continue
			}

			err = mqttclient.ConnectToMqtt(mqttClient)
			if err != nil {
				l.Error("Failed to connect to MQTT", err)
			}
			defer mqttClient.Disconnect(250)

			sourceBytes, err := os.ReadFile(configuration.TargetImagePath)
			if err != nil {
				l.Errorf("Error reading file %s: %v", configuration.TargetImagePath, err)
				removeFile(configuration.TargetImagePath)
				continue
			}
			sourceImage := types.Image{
				Bytes: sourceBytes,
			}
			// should delete the file as soon as possible
			removeFile(configuration.TargetImagePath)
			detectFacesInput := rekognition.DetectFacesInput{
				Image: &sourceImage,
			}
			// recognizeClient.CreateFaceLivenessSession()
			output, err := recognizeClient.DetectFaces(ctx, &detectFacesInput)
			if err != nil {
				l.Error("Error detecting face", err)
				continue
			}
			if len(output.FaceDetails) == 0 {
				l.Errorf("no faces detected in the image: %s", configuration.TargetImagePath)
				if !configuration.DiscoveryMode {
					publishMqttMessage(mqttClient, configuration.MqttTopic, configuration.MqttNotRecognizedMessage)
					continue
				}
			}

			detectLabelsInputs := rekognition.DetectLabelsInput{
				Image: &sourceImage,
			}
			labelsOutput, err := recognizeClient.DetectLabels(ctx, &detectLabelsInputs)
			if err != nil {
				l.Error("Error detecting labels", err)
				continue
			}
			if configuration.DiscoveryMode {
				l.Infof("DetectLabels output:\n%s", awsutil.Prettify(labelsOutput))
				if configuration.DiscoveryLabelsFileOutput != "" {
					l.Infof("writing labels to a file: %s", configuration.DiscoveryLabelsFileOutput)
					err = writeToFile(configuration.DiscoveryLabelsFileOutput, awsutil.Prettify(labelsOutput))
					if err != nil {
						l.Error(err)
						continue
					}
				}
			}

			var labelsPassed bool
		out:
			for _, label := range labelsOutput.Labels {
				for labelName, threshold := range configuration.ConfidencesNotLessThanNormalized {
					labelsPassed, err = verifyLabelConfidenceNotLessThan(label, labelName, threshold)
					if err != nil {
						l.Error(err)
						break out
					}
				}

				for labelName, threshold := range configuration.ConfidencesNotMoreThanNormalized {
					labelsPassed, err = verifyLabelConfidenceNotMoreThan(label, labelName, threshold)
					if err != nil {
						l.Error(err)
						break out
					}
				}
			}

			if !labelsPassed {
				l.Error("some of the labels did not pass confidence level")
				if !configuration.DiscoveryMode {
					publishMqttMessage(mqttClient, configuration.MqttTopic, configuration.MqttNotRecognizedMessage)
					continue
				}
			}

			atLeastOneMatchFound := false
			// use once in order to atomically change atLeastOneMatchFound variable
			var atLeastOneMatchFoundOnce sync.Once
			// use wait groups in order to process images in parallel
			var wg sync.WaitGroup
			wg.Add(len(rekognitionInputs))

			for _, rekognitionInput := range rekognitionInputs {
				go func(rekognitionInput map[string]rekognition.CompareFacesInput) {
					defer wg.Done()
					comparedFileName := ""
					compareFacesInput := rekognition.CompareFacesInput{}
					for filename, input := range rekognitionInput {
						input.SourceImage = &sourceImage
						compareFacesInput = input
						comparedFileName = filename
					}

					output, err := recognizeClient.CompareFaces(ctx, &compareFacesInput)
					if err != nil {
						l.Error("Error comparing faces", err)
						return
					}

					if len(output.FaceMatches) > 0 {
						atLeastOneMatchFoundOnce.Do(func() {
							atLeastOneMatchFound = true
							l.Infof("recognized snapshot as %s", comparedFileName)
							if !configuration.DiscoveryMode {
								publishMqttMessage(mqttClient, configuration.MqttTopic, configuration.MqttRecognizedMessage)
							}
						})
					} else {
						l.Warnf("did not recognize the caller as %s", comparedFileName)
					}
				}(rekognitionInput)
			}
			wg.Wait()

			if !atLeastOneMatchFound {
				if !configuration.DiscoveryMode {
					publishMqttMessage(mqttClient, configuration.MqttTopic, configuration.MqttNotRecognizedMessage)
				}
			}
		}
	}(doneChan)

	<-doneChan
}

func publishMqttMessage(client mqtt.Client, topic string, message interface{}) {
	ctx := context.Background()
	l := logging.WithContext(ctx)
	token := client.Publish(topic, 0, false, message)
	token.Wait()
	l.Infof("published message (%s) to MQTT topic %s", message, topic)
}

func waitForFile(filePath string) error {
	for {
		_, err := os.Stat(filePath)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		break
	}

	return nil
}

func removeFile(filename string) {
	ctx := context.Background()
	l := logging.WithContext(ctx)
	err := os.Remove(filename)
	if err != nil {
		l.Error(err)
	}
	l.Infof("removed file %s", filename)
}

func verifyLabelConfidenceNotLessThan(label types.Label, labelName string, confidence string) (bool, error) {
	var confidenceFloat64 float64

	confidenceFloat64, err := strconv.ParseFloat(confidence, 32)
	if err != nil {
		return false, err
	}

	if *label.Name == labelName && *label.Confidence < float32(confidenceFloat64) {
		return false, fmt.Errorf("label %s has confidence less than %s (%f)", labelName, confidence, *label.Confidence)
	}
	return true, nil
}

func verifyLabelConfidenceNotMoreThan(label types.Label, labelName string, confidence string) (bool, error) {
	var confidenceFloat64 float64

	confidenceFloat64, err := strconv.ParseFloat(confidence, 32)
	if err != nil {
		return false, err
	}

	if *label.Name == labelName && *label.Confidence > float32(confidenceFloat64) {
		return false, fmt.Errorf("label %s has confidence more than %s (%f)", labelName, confidence, *label.Confidence)
	}
	return true, nil
}

func writeToFile(filename string, text string) error {
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}

	defer f.Close()

	if _, err = f.WriteString(text); err != nil {
		return err
	}
	return nil
}
