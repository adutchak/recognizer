package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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
	"github.com/gorilla/mux"
	"gocv.io/x/gocv"
)

type recognizer struct {
	configuration     *config.Config
	recognizeClient   *rekognition.Client
	mqttClient        mqtt.Client
	rekognitionInputs []map[string]rekognition.CompareFacesInput
}

type RecognizeApiInput struct {
	WebRtcUrl string `json:"webrtc_url"`
}

type Response struct {
	Message string `json:"message"`
}

func (r *recognizer) new() {
	ctx := context.Background()
	log := logging.WithContext(ctx)
	// load configuration
	configuration, err := config.NewConfig()
	if err != nil {
		log.Fatalf("Could not load the configuration, %v", err)
	}
	log.Infof("Loaded config %s", awsutil.Prettify(configuration))
	r.configuration = configuration

	// initialize mqtt client
	mqttClient := mqttclient.GetMqttClient(configuration)
	r.mqttClient = mqttClient

	// initialize AWS recognize client
	recognizeClient, err := aws.GetRekognitionClient()
	if err != nil {
		message := "Cannot initialize AWS recognize client"
		log.Fatal(message)
	}
	r.recognizeClient = recognizeClient

	// get rekognition inputs
	rekognitionInputs, err := getRekognitionInputs(ctx, configuration)
	if err != nil {
		log.Fatal("Cannot get rekognition inputs")
	}
	r.rekognitionInputs = rekognitionInputs
}

func main() {
	ctx := context.Background()
	log := logging.WithContext(ctx)
	configuration, err := config.NewConfig()
	if err != nil {
		log.Fatalf("Could not load the configuration, %v", err)
	}
	switch configuration.RunMode {
	case "file_watcher":
		runFileWatcher()
	case "api":
		runApi()
	}
}

func runApi() {
	ctx := context.Background()
	log := logging.WithContext(ctx)
	r := mux.NewRouter()
	v1 := r.PathPrefix("/v1").Subrouter()
	recognizer := recognizer{}
	recognizer.new()

	// register all the handlers here
	v1.HandleFunc("/recognize", recognizer.RecognizeWebRtcApiHandler).Methods("POST")

	server := &http.Server{
		Addr:         ":8082",
		Handler:      r,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  10 * time.Second,
	}

	log.Info("Starting recognizer API server on :8082")
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("Error starting Storm API server: %v", err)
	}

	select {}
}

func (r *recognizer) RecognizeWebRtcApiHandler(writer http.ResponseWriter, request *http.Request) {
	log := logging.WithContext(context.TODO())
	log.Info("Received API request to recognize")
	ctx := context.Background()
	var recognizeInput RecognizeApiInput
	err := json.NewDecoder(request.Body).Decode(&recognizeInput)
	if err != nil {
		message := "Invalid request payload"
		log.Error(message)
		respondWithError(writer, http.StatusBadRequest, message)
		return
	}

	webcam, err := gocv.OpenVideoCapture(recognizeInput.WebRtcUrl)
	if err != nil {
		message := fmt.Sprintf("Error opening video capture device: %v\n", recognizeInput.WebRtcUrl)
		respondWithError(writer, http.StatusBadRequest, message)
		return
	}
	defer webcam.Close()

	img := gocv.NewMat()
	defer img.Close()

	if ok := webcam.Read(&img); !ok {
		message := fmt.Sprintf("Cannot read device %v\n", recognizeInput.WebRtcUrl)
		respondWithError(writer, http.StatusBadRequest, message)
		return
	}
	if img.Empty() {
		message := fmt.Sprintf("No image on device %v\n", recognizeInput.WebRtcUrl)
		respondWithError(writer, http.StatusBadRequest, message)
		return
	}
	sourceBuff, err := gocv.IMEncode(gocv.JPEGFileExt, img)
	if err != nil {
		message := fmt.Sprint("Cannot IMEncode image")
		respondWithError(writer, http.StatusBadRequest, message)
		return
	}
	err = r.processImage(ctx, sourceBuff.GetBytes())
	if err != nil {
		log.Error(err)
		respondWithError(writer, http.StatusBadRequest, err.Error())
		return
	}

	respondWithJSON(writer, http.StatusOK, Response{
		Message: "Processed image successfully",
	})
}

func runFileWatcher() {
	ctx := context.Background()
	log := logging.WithContext(ctx)

	recognizer := recognizer{}
	recognizer.new()

	log.Info("Starting recognizer")

	doneChan := make(chan bool)
	go func(doneChan chan bool) {
		defer func() {
			doneChan <- true
		}()
		for {
			err := waitForFile(recognizer.configuration.TargetImagePath, recognizer.configuration.TargetImageVerifyEveryMilliseconds)
			if err != nil {
				log.Error(err)
				continue
			}
			sourceBytes, err := os.ReadFile(recognizer.configuration.TargetImagePath)
			if err != nil {
				log.Errorf("Error reading file %s: %v", recognizer.configuration.TargetImagePath, err)
				removeFile(recognizer.configuration.TargetImagePath)
				continue
			}
			// should delete the file as soon as possible
			err = removeFile(recognizer.configuration.TargetImagePath)
			if err != nil {
				log.Error(err)
				continue
			}
			err = recognizer.processImage(ctx, sourceBytes)
			if err != nil {
				log.Error(err)
			}
		}
	}(doneChan)

	<-doneChan
}

func (r *recognizer) processImage(ctx context.Context, sourceBytes []byte) error {
	log := logging.WithContext(ctx)

	sourceImage := types.Image{
		Bytes: sourceBytes,
	}
	detectFacesInput := rekognition.DetectFacesInput{
		Image: &sourceImage,
	}
	// recognizeClient.CreateFaceLivenessSession()
	output, err := r.recognizeClient.DetectFaces(ctx, &detectFacesInput)
	if err != nil {
		log.Error("Error detecting face", err)
		return err
	}
	if len(output.FaceDetails) == 0 {
		message := fmt.Sprintf("No faces detected in the image: %s", r.configuration.TargetImagePath)
		log.Errorf(message)
		if !r.configuration.DiscoveryMode {
			publishMqttMessage(r.mqttClient, r.configuration.MqttTopic, r.configuration.MqttNotRecognizedMessage)
			return fmt.Errorf(message)
		}
	}

	detectLabelsInputs := rekognition.DetectLabelsInput{
		Image: &sourceImage,
	}
	labelsOutput, err := r.recognizeClient.DetectLabels(ctx, &detectLabelsInputs)
	if err != nil {
		log.Error("Error detecting labels", err)
		return err
	}
	if r.configuration.DiscoveryMode {
		log.Infof("DetectLabels output:\n%s", awsutil.Prettify(labelsOutput))
		if r.configuration.DiscoveryLabelsFileOutput != "" {
			log.Infof("Writing labels to a file: %s", r.configuration.DiscoveryLabelsFileOutput)
			err = writeToFile(r.configuration.DiscoveryLabelsFileOutput, awsutil.Prettify(labelsOutput))
			if err != nil {
				log.Error(err)
				return err
			}
		}
	}

	var labelsPassed bool
	for _, label := range labelsOutput.Labels {
		for labelName, threshold := range r.configuration.ConfidencesNotLessThanNormalized {
			labelsPassed, err = verifyLabelConfidenceNotLessThan(label, labelName, threshold)
			if err != nil {
				log.Error(err)
				return err
			}
		}

		for labelName, threshold := range r.configuration.ConfidencesNotMoreThanNormalized {
			labelsPassed, err = verifyLabelConfidenceNotMoreThan(label, labelName, threshold)
			if err != nil {
				log.Error(err)
				return err
			}
		}
	}

	if !labelsPassed {
		message := "Some of the labels did not pass confidence level"
		log.Error(message)
		if !r.configuration.DiscoveryMode {
			publishMqttMessage(r.mqttClient, r.configuration.MqttTopic, r.configuration.MqttNotRecognizedMessage)
			return fmt.Errorf(message)
		}
	}

	atLeastOneMatchFound := false
	// use once in order to atomically change atLeastOneMatchFound variable
	var atLeastOneMatchFoundOnce sync.Once
	// use wait groups in order to process images in parallel
	var wg sync.WaitGroup
	wg.Add(len(r.rekognitionInputs))

	for _, rekognitionInput := range r.rekognitionInputs {
		go func(rekognitionInput map[string]rekognition.CompareFacesInput) {
			defer wg.Done()
			comparedFileName := ""
			compareFacesInput := rekognition.CompareFacesInput{}
			for filename, input := range rekognitionInput {
				input.SourceImage = &sourceImage
				compareFacesInput = input
				comparedFileName = filename
			}

			output, err := r.recognizeClient.CompareFaces(ctx, &compareFacesInput)
			if err != nil {
				log.Error("Error comparing faces", err)
				return
			}

			if len(output.FaceMatches) > 0 {
				atLeastOneMatchFoundOnce.Do(func() {
					atLeastOneMatchFound = true
					log.Infof("recognized snapshot as %s", comparedFileName)
					if !r.configuration.DiscoveryMode {
						publishMqttMessage(r.mqttClient, r.configuration.MqttTopic, r.configuration.MqttRecognizedMessage)
					}
				})
			} else {
				log.Warnf("Did not recognize the caller as %s", comparedFileName)
			}
		}(rekognitionInput)
	}
	wg.Wait()

	if !atLeastOneMatchFound {
		if !r.configuration.DiscoveryMode {
			publishMqttMessage(r.mqttClient, r.configuration.MqttTopic, r.configuration.MqttNotRecognizedMessage)
			return fmt.Errorf("Did not recognize the caller")
		}
	}
	return nil
}

func getRekognitionInputs(ctx context.Context, configuration *config.Config) ([]map[string]rekognition.CompareFacesInput, error) {
	log := logging.WithContext(ctx)
	var rekognitionInputs []map[string]rekognition.CompareFacesInput
	for _, sample := range configuration.SampleImagePaths {
		targeBytes, err := os.ReadFile(sample)
		if err != nil {
			log.Errorf("Error reading file %s: %v", sample, err)
			return rekognitionInputs, err
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
	return rekognitionInputs, nil
}

func publishMqttMessage(client mqtt.Client, topic string, message interface{}) {
	log := logging.WithContext(context.Background())
	err := mqttclient.ConnectToMqtt(client)
	if err != nil {
		log.Error("Failed to connect to MQTT", err)
	}
	defer client.Disconnect(250)

	token := client.Publish(topic, 0, false, message)
	token.Wait()
	log.Infof("Published message (%s) to MQTT topic %s", message, topic)
}

func waitForFile(filePath string, verifyFrequencyMs int) error {
	for {
		_, err := os.Stat(filePath)
		if err != nil {
			time.Sleep(time.Millisecond * time.Duration(verifyFrequencyMs))
			continue
		}
		break
	}

	return nil
}

func removeFile(filename string) error {
	ctx := context.Background()
	log := logging.WithContext(ctx)
	err := os.Remove(filename)
	if err != nil {
		log.Error(err)
		return err
	}
	log.Infof("Removed file %s", filename)
	return nil
}

func verifyLabelConfidenceNotLessThan(label types.Label, labelName string, confidence string) (bool, error) {
	var confidenceFloat64 float64

	confidenceFloat64, err := strconv.ParseFloat(confidence, 32)
	if err != nil {
		return false, err
	}

	if *label.Name == labelName && *label.Confidence < float32(confidenceFloat64) {
		return false, fmt.Errorf("Label %s has confidence less than %s (%f)", labelName, confidence, *label.Confidence)
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
		return false, fmt.Errorf("Label %s has confidence more than %s (%f)", labelName, confidence, *label.Confidence)
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

func respondWithError(w http.ResponseWriter, statusCode int, message string) {
	response := Response{Message: message}
	respondWithJSON(w, statusCode, response)
}

func respondWithJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	ctx := context.Background()
	log := logging.WithContext(ctx)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Errorf("Failed to encode JSON response: %v", err)
	}
}
