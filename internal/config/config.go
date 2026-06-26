package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

const defaultConfigPath = "configs/config.yml"

type Config struct {
	App           AppConfig           `yaml:"app"`
	HTTP          HTTPConfig          `yaml:"http"`
	Database      DatabaseConfig      `yaml:"database"`
	Redis         RedisConfig         `yaml:"redis"`
	Elasticsearch ElasticsearchConfig `yaml:"elasticsearch"`
	Neo4j         Neo4jConfig         `yaml:"neo4j"`
	JWT           JWTConfig           `yaml:"jwt"`
	SecretKey     string              `yaml:"secret_key"`
	Storage       StorageConfig       `yaml:"storage"`
	LLM           LLMConfig           `yaml:"llm"`
	Rag           RagConfig           `yaml:"rag"`
	Memory        MemoryConfig        `yaml:"memory"`
}

type AppConfig struct {
	Env string `yaml:"env"`
}

type HTTPConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type DatabaseConfig struct {
	URL string `yaml:"url"`
}

type RedisConfig struct {
	Addr string `yaml:"addr"`
}

type ElasticsearchConfig struct {
	URL string `yaml:"url"`
}

type Neo4jConfig struct {
	URI      string `yaml:"uri"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}

type JWTConfig struct {
	Secret         string `yaml:"secret"`
	AccessTokenTTL string `yaml:"access_token_ttl"`
}

type StorageConfig struct {
	Backend string `yaml:"backend"`
	Dir     string `yaml:"dir"`
}

type LLMConfig struct {
	Provider       string `yaml:"provider"`
	Model          string `yaml:"model"`
	EmbeddingModel string `yaml:"embedding_model"`
	BaseURL        string `yaml:"base_url"`
	APIKey         string `yaml:"api_key"`
}

type RagConfig struct {
	EmbeddingDim int `yaml:"embedding_dim"`
}

type MemoryConfig struct {
	NameSimGate                      float64 `yaml:"name_sim_gate"`
	LLMMergeConfidence               float64 `yaml:"llm_merge_confidence"`
	CommunityClusteringMaxIterations int     `yaml:"community_clustering_max_iterations"`
	CommunityVoteSemWeight           float64 `yaml:"community_vote_sem_weight"`
	CommunityVoteRelWeight           float64 `yaml:"community_vote_rel_weight"`
	CommunityMergeThreshold          float64 `yaml:"community_merge_threshold"`
}

func Load() Config {
	path := os.Getenv("CONFIG_PATH")
	if path == "" {
		path = defaultConfigPath
	}
	cfg, err := LoadFile(path)
	if err != nil {
		panic(fmt.Sprintf("load config %s: %v", path, err))
	}
	return cfg
}

func LoadFile(path string) (Config, error) {
	cfg := defaultConfig()
	if path != "" {
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			if err := yaml.Unmarshal(data, &cfg); err != nil {
				return Config{}, fmt.Errorf("parse yaml: %w", err)
			}
		case os.IsNotExist(err):
		default:
			return Config{}, fmt.Errorf("read yaml: %w", err)
		}
	}
	applyEnv(&cfg)
	return cfg, nil
}

func (c Config) HTTPAddr() string {
	return fmt.Sprintf("%s:%d", c.HTTP.Host, c.HTTP.Port)
}

func defaultConfig() Config {
	return Config{
		App:           AppConfig{Env: "development"},
		HTTP:          HTTPConfig{Host: "0.0.0.0", Port: 8000},
		Database:      DatabaseConfig{URL: "postgres://comet:comet@localhost:5432/comet?sslmode=disable"},
		Redis:         RedisConfig{Addr: "localhost:6379"},
		Elasticsearch: ElasticsearchConfig{URL: "http://localhost:9200"},
		Neo4j:         Neo4jConfig{URI: "bolt://localhost:7687"},
		JWT:           JWTConfig{Secret: "change-me", AccessTokenTTL: "168h"},
		SecretKey:     "0123456789abcdef0123456789abcdef",
		Storage:       StorageConfig{Backend: "local", Dir: "./storage"},
		LLM:           LLMConfig{Provider: "openai", BaseURL: "https://api.openai.com/v1"},
		Rag:           RagConfig{EmbeddingDim: 1024},
		Memory: MemoryConfig{
			NameSimGate:                      0.8,
			LLMMergeConfidence:               0.8,
			CommunityClusteringMaxIterations: 10,
			CommunityVoteSemWeight:           0.6,
			CommunityVoteRelWeight:           0.4,
			CommunityMergeThreshold:          0.85,
		},
	}
}

func applyEnv(cfg *Config) {
	cfg.App.Env = env("APP_ENV", cfg.App.Env)
	cfg.HTTP.Host = env("APP_HOST", cfg.HTTP.Host)
	cfg.HTTP.Port = envInt("APP_PORT", cfg.HTTP.Port)
	cfg.Database.URL = env("DATABASE_URL", cfg.Database.URL)
	cfg.Redis.Addr = env("REDIS_ADDR", cfg.Redis.Addr)
	cfg.Elasticsearch.URL = env("ES_HOST", cfg.Elasticsearch.URL)
	cfg.Neo4j.URI = env("NEO4J_URI", cfg.Neo4j.URI)
	cfg.Neo4j.Username = env("NEO4J_USERNAME", cfg.Neo4j.Username)
	cfg.Neo4j.Password = env("NEO4J_PASSWORD", cfg.Neo4j.Password)
	cfg.Neo4j.Database = env("NEO4J_DATABASE", cfg.Neo4j.Database)
	cfg.JWT.Secret = env("JWT_SECRET", cfg.JWT.Secret)
	cfg.JWT.AccessTokenTTL = env("JWT_ACCESS_TOKEN_TTL", cfg.JWT.AccessTokenTTL)
	cfg.SecretKey = env("SECRET_KEY", cfg.SecretKey)
	cfg.Storage.Backend = env("STORAGE_BACKEND", cfg.Storage.Backend)
	cfg.Storage.Dir = env("STORAGE_DIR", cfg.Storage.Dir)
	cfg.LLM.Provider = env("LLM_PROVIDER", cfg.LLM.Provider)
	cfg.LLM.Model = env("LLM_MODEL", cfg.LLM.Model)
	cfg.LLM.EmbeddingModel = env("LLM_EMBEDDING_MODEL", cfg.LLM.EmbeddingModel)
	cfg.LLM.BaseURL = env("LLM_BASE_URL", cfg.LLM.BaseURL)
	cfg.LLM.APIKey = env("LLM_API_KEY", cfg.LLM.APIKey)
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
