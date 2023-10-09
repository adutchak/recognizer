package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"

	"github.com/adutchak/recognizer/pkg/logging"
)

type Config struct {
	MqttTopic                string `json:"mqttTopic"`
	MqttBroker               string `json:"mqttBroker"`
	MqttPort                 int    `json:"mqttPort"`
	MqttClientId             string `json:"mqttClientId"`
	MqttUsername             string `json:"mqttUsername"`
	MqttPassword             string `json:"mqttPassword"`
	MqttRecognizedMessage    string `json:"mqttRecognizedMessage"`
	MqttNotRecognizedMessage string `json:"mqttNotRecognizedMessage"`

	TargetImagePath     string   `json:"targetImagePath"`
	SampleImagePaths    []string `json:"sampleImagePaths"`
	SimilarityThreshold float32  `json:"similarityThreshold"`

	ConfidencesNotLessThan map[string]string `json:"confidencesNotLessThan"`
	ConfidencesNotMoreThan map[string]string `json:"confidencesNotMoreThan"`
}

func NewConfig() (*Config, error) {
	return Parse(os.Args[1:])
}

func Parse(args []string) (*Config, error) {
	ctx := context.Background()
	l := logging.WithContext(ctx)

	fs := BuildFlagSet()
	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	v := viper.New()
	v.AutomaticEnv()
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_", ".", "_"))

	if err := v.BindPFlags(fs); err != nil {
		return nil, err
	}

	configFile := v.GetString(ConfigFileKey)
	configFileExtension := filepath.Ext(configFile)
	configName := configFile[0 : len(configFile)-len(configFileExtension)]

	v.SetConfigName(configName)
	v.AddConfigPath(".")
	v.AddConfigPath("./configs")

	err := v.ReadInConfig()
	if err == nil {
		l.Infof("Found config %s, using values provided the config file.", configFile)
	}

	return &Config{
		MqttTopic:                v.GetString(MqttTopicKey),
		MqttBroker:               v.GetString(MqttBrokerKey),
		MqttPort:                 v.GetInt(MqttPortKey),
		MqttClientId:             v.GetString(MqttClientIdKey),
		MqttUsername:             v.GetString(MqttUsernameKey),
		MqttPassword:             v.GetString(MqttPasswordKey),
		MqttRecognizedMessage:    v.GetString(MqttRecognizedMessageKey),
		MqttNotRecognizedMessage: v.GetString(MqttNotRecognizedMessageKey),

		TargetImagePath:        v.GetString(TargetImagePathKey),
		SampleImagePaths:       v.GetStringSlice(SampleImagePathsKey),
		SimilarityThreshold:    float32(v.GetFloat64(SimilarityThresholdKey)),
		ConfidencesNotLessThan: v.GetStringMapString(ConfidencesNotLessThanKey),
		ConfidencesNotMoreThan: v.GetStringMapString(ConfidencesNotMoreThanKey),
	}, nil
}
