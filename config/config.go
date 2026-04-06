package config

import (
	"log"
	"os"

	"github.com/BurntSushi/toml"
)

type MainConfig struct {
	Port    int    `toml:"port"`
	AppName string `toml:"appName"`
	Host    string `toml:"host"`
}

type EmailConfig struct {
	Authcode string `toml:"authcode"`
	Email    string `toml:"email"`
}

type RedisConfig struct {
	RedisPort     int    `toml:"port"`
	RedisDb       int    `toml:"db"`
	RedisHost     string `toml:"host"`
	RedisPassword string `toml:"password"`
}

type MysqlConfig struct {
	MysqlPort         int    `toml:"port"`
	MysqlHost         string `toml:"host"`
	MysqlUser         string `toml:"user"`
	MysqlPassword     string `toml:"password"`
	MysqlDatabaseName string `toml:"databaseName"`
	MysqlCharset      string `toml:"charset"`
}

type JwtConfig struct {
	ExpireDuration        int    `toml:"expire_duration"`
	AccessExpireDuration  int    `toml:"access_expire_duration"`
	RefreshExpireDuration int    `toml:"refresh_expire_duration"`
	Issuer                string `toml:"issuer"`
	Subject               string `toml:"subject"`
	Key                   string `toml:"key"`
}

type OpenAIConfig struct {
	APIKey    string `toml:"apiKey"`
	ModelName string `toml:"modelName"`
	BaseURL   string `toml:"baseUrl"`
}

type Rabbitmq struct {
	RabbitmqPort     int    `toml:"port"`
	RabbitmqHost     string `toml:"host"`
	RabbitmqUsername string `toml:"username"`
	RabbitmqPassword string `toml:"password"`
	RabbitmqVhost    string `toml:"vhost"`
}

type RagModelConfig struct {
	StoreMode           string `toml:"storeMode"`
	RagEmbeddingModel   string `toml:"embeddingModel"`
	RagEmbeddingAPIKey  string `toml:"embeddingApiKey"`
	RagEmbeddingBaseURL string `toml:"embeddingBaseUrl"`
	RagChatModelName    string `toml:"chatModelName"`
	RagChatAPIKey       string `toml:"chatApiKey"`
	RagChatBaseURL      string `toml:"chatBaseUrl"`
	RagDocDir           string `toml:"docDir"`
	RagBaseUrl          string `toml:"baseUrl"`
	RagDimension        int    `toml:"dimension"`
	QueryCacheTTL       int    `toml:"queryCacheTTLSeconds"`
	IndexedCacheTTL     int    `toml:"indexedCacheTTLSeconds"`
}

type VoiceServiceConfig struct {
	VoiceServiceApiKey    string `toml:"voiceServiceApiKey"`
	VoiceServiceSecretKey string `toml:"voiceServiceSecretKey"`
}

type StorageConfig struct {
	Provider                   string `toml:"provider"`
	BasePath                   string `toml:"basePath"`
	Bucket                     string `toml:"bucket"`
	Endpoint                   string `toml:"endpoint"`
	AccessKey                  string `toml:"accessKey"`
	SecretKey                  string `toml:"secretKey"`
	UseSSL                     bool   `toml:"useSSL"`
	Region                     string `toml:"region"`
	AutoCreate                 bool   `toml:"autoCreateBucket"`
	ObjectPrefix               string `toml:"objectPrefix"`
	UploadPresignExpirySeconds int    `toml:"uploadPresignExpirySeconds"`
	PresignExpirySeconds       int    `toml:"presignExpirySeconds"`
}

type MilvusConfig struct {
	Host       string `toml:"host"`
	Port       int    `toml:"port"`
	Database   string `toml:"database"`
	Collection string `toml:"collection"`
	Username   string `toml:"username"`
	Password   string `toml:"password"`
	EnableAuth bool   `toml:"enableAuth"`
	Dimension  int    `toml:"dimension"`
}

type LogConfig struct {
	Path      string `toml:"path"`
	MaxSizeMB int    `toml:"maxSizeMB"`
}

type MCPConfig struct {
	Enabled   bool   `toml:"enabled"`
	AutoStart bool   `toml:"autoStart"`
	BaseURL   string `toml:"baseUrl"`
	HTTPAddr  string `toml:"httpAddr"`
}

type Config struct {
	EmailConfig        `toml:"emailConfig"`
	RedisConfig        `toml:"redisConfig"`
	MysqlConfig        `toml:"mysqlConfig"`
	JwtConfig          `toml:"jwtConfig"`
	MainConfig         `toml:"mainConfig"`
	OpenAIConfig       `toml:"openAIConfig"`
	Rabbitmq           `toml:"rabbitmqConfig"`
	RagModelConfig     `toml:"ragModelConfig"`
	VoiceServiceConfig `toml:"voiceServiceConfig"`
	StorageConfig      `toml:"storageConfig"`
	MilvusConfig       `toml:"milvusConfig"`
	LogConfig          `toml:"logConfig"`
	MCPConfig          `toml:"mcpConfig"`
}

type RedisKeyConfig struct {
	CaptchaPrefix   string
	IndexName       string
	IndexNamePrefix string
}

var DefaultRedisKeyConfig = RedisKeyConfig{
	CaptchaPrefix:   "captcha:%s",
	IndexName:       "rag_docs:%s:idx",
	IndexNamePrefix: "rag_docs:%s:",
}

var config *Config

func InitConfig() error {
	configPaths := []string{
		"config/config.local.toml",
		"config/config.toml",
		"config/config.example.toml",
	}
	for _, path := range configPaths {
		if _, err := os.Stat(path); err == nil {
			if _, err := toml.DecodeFile(path, config); err != nil {
				log.Fatal(err.Error())
				return err
			}
			return nil
		}
	}
	log.Fatal("no config file found: tried config/config.local.toml, config/config.toml, config/config.example.toml")
	return os.ErrNotExist
}

func GetConfig() *Config {
	if config == nil {
		config = new(Config)
		_ = InitConfig()
	}
	return config
}
