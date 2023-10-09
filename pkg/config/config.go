package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-playground/validator"
	"github.com/spf13/viper"

	"github.com/adutchak/recognizer/pkg/logging"
)

type Config struct {
	DiscoveryMode            bool   `json:"discoveryMode"`
	MqttTopic                string `json:"mqttTopic" validate:"required"`
	MqttBroker               string `json:"mqttBroker" validate:"required"`
	MqttPort                 int    `json:"mqttPort"`
	MqttClientId             string `json:"mqttClientId" validate:"required"`
	MqttUsername             string `json:"mqttUsername" validate:"required"`
	MqttPassword             string `json:"mqttPassword" validate:"required"`
	MqttRecognizedMessage    string `json:"mqttRecognizedMessage"`
	MqttNotRecognizedMessage string `json:"mqttNotRecognizedMessage"`

	TargetImagePath     string   `json:"targetImagePath" validate:"required"`
	SampleImagePaths    []string `json:"sampleImagePaths" validate:"required"`
	SimilarityThreshold float32  `json:"similarityThreshold"`

	ConfidencesNotLessThan string `json:"confidencesNotLessThan"`
	ConfidencesNotMoreThan string `json:"confidencesNotMoreThan"`

	ConfidencesNotLessThanNormalized map[string]string `json:"confidencesNotLessThanNormalized"`
	ConfidencesNotMoreThanNormalized map[string]string `json:"confidencesNotMoreThanNormalized"`
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

	conf := &Config{
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
		ConfidencesNotLessThan: v.GetString(ConfidencesNotLessThanKey),
		ConfidencesNotMoreThan: v.GetString(ConfidencesNotMoreThanKey),
		DiscoveryMode:          v.GetBool(DiscoveryModeKey),
	}

	validate := validator.New()
	if err := validate.Struct(conf); err != nil {
		l.Fatalf("Missing required attributes %v\n", err)
	}
	conf.ConfidencesNotLessThanNormalized = make(map[string]string)
	conf.ConfidencesNotMoreThanNormalized = make(map[string]string)

	confidencesNotLessThan := strings.Split(conf.ConfidencesNotLessThan, ",")
	confidencesNotMoreThan := strings.Split(conf.ConfidencesNotMoreThan, ",")

	for _, label := range confidencesNotLessThan {
		s := strings.Split(label, ":")
		if len(s) == 2 {
			conf.ConfidencesNotLessThanNormalized[s[0]] = s[1]
		}
	}
	for _, label := range confidencesNotMoreThan {
		s := strings.Split(label, ":")
		if len(s) == 2 {
			conf.ConfidencesNotMoreThanNormalized[s[0]] = s[1]
		}
	}
	return conf, nil
}
