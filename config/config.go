package config

import (
	"log"
	"os"
	"strings"

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
	Path                         string `toml:"path"`
	MaxSizeMB                    int    `toml:"maxSizeMB"`
	SessionConversationDir       string `toml:"sessionConversationDir"`
	SessionConversationRetention int    `toml:"sessionConversationRetention"`
}

type MCPConfig struct {
	Enabled        bool              `toml:"enabled"`
	AutoStart      bool              `toml:"autoStart"`
	AutoStartLocal bool              `toml:"autoStartLocal"`
	BaseURL        string            `toml:"baseUrl"`
	HTTPAddr       string            `toml:"httpAddr"`
	DefaultServer  string            `toml:"defaultServer"`
	Servers        []MCPServerConfig `toml:"servers"`
}

// MCPServerConfig 描述一个可被聚合层接入的 MCP Server。
// 第一阶段先支持 HTTP / Streamable HTTP 与 stdio 两类传输。
type MCPServerConfig struct {
	Name           string            `toml:"name"`
	Enabled        bool              `toml:"enabled"`
	Transport      string            `toml:"transport"`
	BaseURL        string            `toml:"baseUrl"`
	Command        string            `toml:"command"`
	Args           []string          `toml:"args"`
	Headers        map[string]string `toml:"headers"`
	TimeoutSeconds int               `toml:"timeoutSeconds"`
	MaxResultChars int               `toml:"maxResultChars"`
	ToolAllowlist  []string          `toml:"toolAllowlist"`
	ToolBlocklist  []string          `toml:"toolBlocklist"`
	Origin         string            `toml:"origin"`
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

// EffectiveServers 返回启用且可用的 MCP Server 配置列表。
// 为兼容旧配置，当未声明 servers 时，会根据旧字段兜底构造一个 local server。
func (c *MCPConfig) EffectiveServers() []MCPServerConfig {
	if c == nil || !c.Enabled {
		return nil
	}

	servers := make([]MCPServerConfig, 0, len(c.Servers)+1)
	for _, server := range c.Servers {
		normalized := server.normalized()
		if !normalized.Enabled {
			continue
		}
		if normalized.Name == "" {
			continue
		}
		if normalized.Transport == "stdio" && strings.TrimSpace(normalized.Command) == "" {
			continue
		}
		if normalized.Transport != "stdio" && strings.TrimSpace(normalized.BaseURL) == "" {
			continue
		}
		servers = append(servers, normalized)
	}

	if len(servers) > 0 {
		return servers
	}

	legacyBaseURL := strings.TrimSpace(c.BaseURL)
	if legacyBaseURL == "" {
		legacyBaseURL = "http://localhost:29871/mcp"
	}
	return []MCPServerConfig{
		{
			Name:           "local",
			Enabled:        true,
			Transport:      "streamable_http",
			BaseURL:        legacyBaseURL,
			TimeoutSeconds: 15,
			MaxResultChars: 8000,
			Origin:         "local",
		},
	}
}

// ShouldAutoStartLocal 判断当前进程是否需要自动拉起本地 MCP Server。
// 这里同时兼容旧字段 autoStart 与新字段 autoStartLocal。
func (c *MCPConfig) ShouldAutoStartLocal() bool {
	if c == nil || !c.Enabled {
		return false
	}
	if c.AutoStartLocal {
		return true
	}
	return c.AutoStart
}

func (c MCPServerConfig) normalized() MCPServerConfig {
	c.Name = strings.TrimSpace(c.Name)
	c.Transport = strings.ToLower(strings.TrimSpace(c.Transport))
	c.BaseURL = strings.TrimSpace(c.BaseURL)
	c.Command = strings.TrimSpace(c.Command)
	c.Origin = strings.TrimSpace(c.Origin)
	if c.Transport == "" {
		c.Transport = "streamable_http"
	}
	if c.TimeoutSeconds <= 0 {
		c.TimeoutSeconds = 15
	}
	if c.MaxResultChars <= 0 {
		c.MaxResultChars = 8000
	}
	if c.Origin == "" {
		c.Origin = "third_party"
	}
	return c
}
