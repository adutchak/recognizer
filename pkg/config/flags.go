package config

import (
	"github.com/spf13/pflag"
)

const (
	DefaultConfigFile = "config.yaml"
)

var DefaultConfig = Config{
	MqttTopic:                "enterance/recognizer",
	SimilarityThreshold:      95,
	MqttPort:                 1883,
	MqttRecognizedMessage:    `{"message": "recognized"}`,
	MqttNotRecognizedMessage: `{"message": "not_recognized"}`,
	DiscoveryMode:            false,
}

func BuildFlagSet() *pflag.FlagSet {
	fs := pflag.NewFlagSet("recognizer", pflag.ExitOnError)
	fs.String(MqttTopicKey, DefaultConfig.MqttTopic, "specifies the mqtt topic to work with")
	fs.String(MqttBrokerKey, "", "specifies the mqtt broker to work with")
	fs.String(MqttClientIdKey, "", "specifies the mqtt client ID")
	fs.String(MqttUsernameKey, "", "specifies the mqtt username")
	fs.String(MqttPasswordKey, "", "specifies the mqtt password")
	fs.Int(MqttPortKey, DefaultConfig.MqttPort, "specifies the mqtt port")
	fs.String(MqttRecognizedMessageKey, DefaultConfig.MqttRecognizedMessage, "mqtt message for recognized event")
	fs.String(MqttNotRecognizedMessageKey, DefaultConfig.MqttNotRecognizedMessage, "mqtt message for not recognized event")

	fs.String(TargetImagePathKey, "", "specifies the target image path to work with")
	fs.StringSlice(SampleImagePathsKey, []string{}, "specifies the target image path to work with")
	fs.Float32(SimilarityThresholdKey, DefaultConfig.SimilarityThreshold, "specifies the minimal similarity threshold")
	fs.String(ConfidencesNotLessThanKey, "", "specifies labels whose recognized confidence should not be less than threshold, example: \"Photography:98.0,Fisheye:60.0,Computer Hardware:40.0\"")
	fs.String(ConfidencesNotMoreThanKey, "", "specifies labels whose recognized confidence should be more than threshold, example: \"Electronics:90.0,Phone:40.0,Computer Hardware:40.0\"")

	fs.Bool(DiscoveryModeKey, DefaultConfig.DiscoveryMode, "mode which simply prints recognized information")
	return fs
}
