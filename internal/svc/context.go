package svc

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/boxify/api-go/internal/config"
	corellm "github.com/boxify/api-go/internal/core/llm"
	coremcp "github.com/boxify/api-go/internal/core/mcp"
	"github.com/boxify/api-go/internal/core/prompt"
	ragchunker "github.com/boxify/api-go/internal/core/rag/chunker"
	ragclassifier "github.com/boxify/api-go/internal/core/rag/classifier"
	ragparser "github.com/boxify/api-go/internal/core/rag/documentparse"
	ragsearch "github.com/boxify/api-go/internal/core/rag/search"
	"github.com/boxify/api-go/internal/core/rag/webcrawl"
	domainskills "github.com/boxify/api-go/internal/domain/skills"
	infraes "github.com/boxify/api-go/internal/infrastructure/db/es"
	dbneo4j "github.com/boxify/api-go/internal/infrastructure/db/neo4j"
	dbpostgres "github.com/boxify/api-go/internal/infrastructure/db/postgres"
	infraredis "github.com/boxify/api-go/internal/infrastructure/db/redis"
	infrallm "github.com/boxify/api-go/internal/infrastructure/llm"
	"github.com/boxify/api-go/internal/infrastructure/queue"
	queueredis "github.com/boxify/api-go/internal/infrastructure/queue/redis"
	"github.com/boxify/api-go/internal/infrastructure/realtime"
	realtimeredis "github.com/boxify/api-go/internal/infrastructure/realtime/redis"
	"github.com/boxify/api-go/internal/infrastructure/security"
	"github.com/boxify/api-go/internal/infrastructure/storage"
	"github.com/boxify/api-go/internal/models"
	appprompts "github.com/boxify/api-go/internal/prompts"
	"github.com/boxify/api-go/internal/prompts/promptsgen"
	"github.com/boxify/api-go/internal/repository"
	repositoryes "github.com/boxify/api-go/internal/repository/es"
	"github.com/boxify/api-go/internal/repository/graph"
	repositorypostgres "github.com/boxify/api-go/internal/repository/postgres"
	"github.com/boxify/api-go/internal/xerr"
	"gorm.io/gorm"
)

type ServiceContext struct {
	Config config.Config

	GormDB        *gorm.DB
	Neo4j         *dbneo4j.Client
	Redis         *infraredis.Client
	Realtime      realtime.Broker
	TaskProducer  queue.Producer
	Elasticsearch *infraes.Client
	Storage       storage.Store
	URLSigner     storage.URLSigner

	UserRepo            repository.UserRepository
	RefreshTokenRepo    repository.RefreshTokenRepository
	MemoryGraphRepo     repository.MemoryGraphRepository
	ModelConfigRepo     repository.ModelConfigRepository
	ConversationRepo    repository.ConversationRepository
	MessageRepo         repository.MessageRepository
	MessageFeedbackRepo repository.MessageFeedbackRepository
	AgentConfigRepo     repository.AgentConfigRepository
	AgentPersonaRepo    repository.AgentPersonaRepository
	AgentTaskRepo       repository.AgentTaskRepository
	MCPServerRepo       repository.MCPServerRepository
	KnowledgeBaseRepo   repository.KnowledgeBaseRepository
	SkillRepo           repository.SkillRepository
	DocumentRepo        repository.DocumentRepository
	ImageRepo           repository.ImageRepository
	TagRepo             repository.TagRepository
	RAGChunkRepo        repository.RAGChunkRepository
	RAGSearcher         *ragsearch.Searcher[models.RAGChunkSource]
	RAGClassifier       *ragclassifier.Classifier
	RAGDocumentParser   *ragparser.Parser
	RAGChunker          *ragchunker.Chunker
	RAGWebCrawler       *webcrawl.Crawler
	SkillRegistry       *domainskills.Registry

	SecretCipher *security.SecretCipher
	TokenIssuer  *security.TokenIssuer

	PromptManager  *prompt.Manager
	PromptClient   *promptsgen.Client
	LLMManager     *corellm.Manager
	MCPToolService *coremcp.Service

	closeOnce sync.Once
	closeErr  error
}

func New(ctx context.Context, cfg config.Config) (*ServiceContext, error) {
	cipher, err := security.NewSecretCipher(cfg.SecretKey)
	if err != nil {
		return nil, xerr.Wrapf(err, "创建密钥加密器失败")
	}
	accessTokenTTL, err := time.ParseDuration(cfg.JWT.AccessTokenTTL)
	if err != nil {
		return nil, xerr.BadRequest("JWT access token TTL 配置无效")
	}

	promptManager := prompt.NewManager()
	if err := appprompts.Register(promptManager); err != nil {
		return nil, xerr.Wrapf(err, "注册提示词失败")
	}
	skillRegistry, err := domainskills.NewRegistry()
	if err != nil {
		return nil, xerr.Wrapf(err, "注册内置技能失败")
	}

	db, err := dbpostgres.NewGormDB(ctx, dbpostgres.Config{URL: cfg.Database.URL})
	if err != nil {
		return nil, err
	}

	svcCtx := &ServiceContext{
		Config: cfg,

		SecretCipher: cipher,
		TokenIssuer:  security.NewTokenIssuer(cfg.JWT.Secret, accessTokenTTL),

		PromptManager:  promptManager,
		PromptClient:   promptsgen.NewClient(promptManager),
		LLMManager:     BuildLLMManager(),
		MCPToolService: coremcp.NewService(coremcp.Options{}),
		SkillRegistry:  skillRegistry,
	}
	bindPostgresRepositories(svcCtx, db)

	redisClient, err := infraredis.NewClient(ctx, infraredis.Config{
		Addr:     cfg.Redis.Addr,
		Username: cfg.Redis.Username,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err != nil {
		_ = svcCtx.Close(ctx)
		return nil, err
	}
	svcCtx.Redis = redisClient
	svcCtx.Realtime = BuildRealtime(redisClient)
	svcCtx.TaskProducer = BuildTaskProducer(cfg.Redis)

	esClient, err := infraes.NewClient(infraes.Config{
		URL:      cfg.Elasticsearch.URL,
		Username: cfg.Elasticsearch.Username,
		Password: cfg.Elasticsearch.Password,
		APIKey:   cfg.Elasticsearch.APIKey,
	})
	if err != nil {
		_ = svcCtx.Close(ctx)
		return nil, err
	}
	svcCtx.Elasticsearch = esClient
	ragChunkRepo := repositoryes.NewRAGChunkRepository(esClient, cfg.Rag.ChunkIndex)
	svcCtx.RAGChunkRepo = ragChunkRepo
	svcCtx.RAGSearcher = ragsearch.NewSearcher[models.RAGChunkSource](
		esClient,
		ragsearch.WithIndex(cfg.Rag.ChunkIndex),
		ragsearch.WithEmbeddingDim(cfg.Rag.EmbeddingDim),
		ragsearch.WithSourceDecoder[models.RAGChunkSource](ragChunkRepo.DecodeSource),
	)
	svcCtx.RAGDocumentParser = ragparser.NewParser()
	svcCtx.RAGChunker = ragchunker.NewChunker(ragchunker.WithParentChunkTokens(1200))
	svcCtx.RAGClassifier = ragclassifier.NewClassifier()
	svcCtx.RAGWebCrawler = webcrawl.NewCrawler()

	store, urlSigner, err := BuildStorage(cfg.Storage)
	if err != nil {
		_ = svcCtx.Close(ctx)
		return nil, err
	}
	svcCtx.Storage = store
	svcCtx.URLSigner = urlSigner

	if shouldInitNeo4j(cfg.Neo4j) {
		neo4jClient, err := dbneo4j.NewClient(ctx, dbneo4j.Config{
			URI:      cfg.Neo4j.URI,
			Username: cfg.Neo4j.Username,
			Password: cfg.Neo4j.Password,
			Database: cfg.Neo4j.Database,
		})
		if err != nil {
			_ = svcCtx.Close(ctx)
			return nil, err
		}
		svcCtx.Neo4j = neo4jClient
		svcCtx.MemoryGraphRepo = graph.NewMemoryGraphRepository(neo4jClient)
	}

	return svcCtx, nil
}

func bindPostgresRepositories(s *ServiceContext, db *gorm.DB) {
	s.GormDB = db
	s.UserRepo = repositorypostgres.NewUserRepository(db)
	s.RefreshTokenRepo = repositorypostgres.NewRefreshTokenRepository(db)
	s.ModelConfigRepo = repositorypostgres.NewModelConfigRepository(db)
	s.ConversationRepo = repositorypostgres.NewConversationRepository(db)
	s.MessageRepo = repositorypostgres.NewMessageRepository(db)
	s.MessageFeedbackRepo = repositorypostgres.NewMessageFeedbackRepository(db)
	s.AgentConfigRepo = repositorypostgres.NewAgentConfigRepository(db)
	s.AgentPersonaRepo = repositorypostgres.NewAgentPersonaRepository(db)
	s.AgentTaskRepo = repositorypostgres.NewAgentTaskRepository(db)
	s.MCPServerRepo = repositorypostgres.NewMCPServerRepository(db)
	s.KnowledgeBaseRepo = repositorypostgres.NewKnowledgeBaseRepository(db)
	s.SkillRepo = repositorypostgres.NewSkillRepository(db)
	s.DocumentRepo = repositorypostgres.NewDocumentRepository(db)
	s.ImageRepo = repositorypostgres.NewImageRepository(db)
	s.TagRepo = repositorypostgres.NewTagRepository(db)
}

func (s *ServiceContext) WithTx(ctx context.Context, fn func(txSvc *ServiceContext) error) error {
	if s == nil || s.GormDB == nil {
		return xerr.Internal("数据库未初始化", nil)
	}
	return s.GormDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 手动构造浅拷贝，避免复制 sync.Once（closeOnce）。
		txSvc := newTxContext(s)
		bindPostgresRepositories(&txSvc, tx)
		return fn(&txSvc)
	})
}

// newTxContext 返回仅包含事务内执行所需字段的浅拷贝，跳过 closeOnce 等锁字段。
func newTxContext(s *ServiceContext) ServiceContext {
	return ServiceContext{
		Config:              s.Config,
		Neo4j:               s.Neo4j,
		Redis:               s.Redis,
		Realtime:            s.Realtime,
		TaskProducer:        s.TaskProducer,
		Elasticsearch:       s.Elasticsearch,
		Storage:             s.Storage,
		URLSigner:           s.URLSigner,
		UserRepo:            s.UserRepo,
		RefreshTokenRepo:    s.RefreshTokenRepo,
		MemoryGraphRepo:     s.MemoryGraphRepo,
		ModelConfigRepo:     s.ModelConfigRepo,
		ConversationRepo:    s.ConversationRepo,
		MessageRepo:         s.MessageRepo,
		MessageFeedbackRepo: s.MessageFeedbackRepo,
		AgentConfigRepo:     s.AgentConfigRepo,
		AgentPersonaRepo:    s.AgentPersonaRepo,
		AgentTaskRepo:       s.AgentTaskRepo,
		MCPServerRepo:       s.MCPServerRepo,
		KnowledgeBaseRepo:   s.KnowledgeBaseRepo,
		SkillRepo:           s.SkillRepo,
		DocumentRepo:        s.DocumentRepo,
		ImageRepo:           s.ImageRepo,
		TagRepo:             s.TagRepo,
		RAGChunkRepo:        s.RAGChunkRepo,
		RAGSearcher:         s.RAGSearcher,
		RAGClassifier:       s.RAGClassifier,
		RAGDocumentParser:   s.RAGDocumentParser,
		RAGChunker:          s.RAGChunker,
		RAGWebCrawler:       s.RAGWebCrawler,
		SkillRegistry:       s.SkillRegistry,
		SecretCipher:        s.SecretCipher,
		TokenIssuer:         s.TokenIssuer,
		PromptManager:       s.PromptManager,
		PromptClient:        s.PromptClient,
		LLMManager:          s.LLMManager,
		MCPToolService:      s.MCPToolService,
		closeErr:            s.closeErr,
	}
}

func (s *ServiceContext) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		var errs []error
		if s.Neo4j != nil {
			if err := s.Neo4j.Close(ctx); err != nil {
				errs = append(errs, err)
			}
		}
		if closer, ok := s.Storage.(interface{ Close() error }); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, xerr.Wrapf(err, "关闭存储客户端失败"))
			}
		}
		if s.Redis != nil {
			if err := s.Redis.Close(); err != nil {
				errs = append(errs, err)
			}
		}
		if s.TaskProducer != nil {
			if err := s.TaskProducer.Close(); err != nil {
				errs = append(errs, xerr.Wrapf(err, "关闭任务队列 producer 失败"))
			}
		}
		if s.GormDB != nil {
			sqlDB, err := s.GormDB.DB()
			if err != nil {
				errs = append(errs, xerr.Wrapf(err, "获取 Postgres 底层连接失败"))
			} else if err := sqlDB.Close(); err != nil {
				errs = append(errs, xerr.Wrapf(err, "关闭 Postgres 连接失败"))
			}
		}
		s.closeErr = errors.Join(errs...)
	})
	return s.closeErr
}

func BuildRealtime(redisClient *infraredis.Client) realtime.Broker {
	if redisClient == nil || redisClient.Raw() == nil {
		return nil
	}
	return realtimeredis.New(redisClient.Raw())
}

func BuildTaskProducer(cfg config.RedisConfig) queue.Producer {
	if cfg.Addr == "" {
		return nil
	}
	return queueredis.NewProducer(queueredis.Config{
		Addr:     cfg.Addr,
		Username: cfg.Username,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
}

func BuildLLMManager() *corellm.Manager {
	manager := corellm.NewManager()
	openAICompatible := infrallm.NewOpenAICompatibleFactory()
	for _, provider := range []string{"openai", "qwen", "doubao", "deepseek", "zhipu", "qianfan"} {
		manager.Register(provider, openAICompatible)
	}
	manager.Register("anthropic", infrallm.NewAnthropicFactory())
	return manager
}

func shouldInitNeo4j(cfg config.Neo4jConfig) bool {
	return cfg.URI != "" && cfg.Username != "" && cfg.Password != ""
}

func BuildStorage(cfg config.StorageConfig) (storage.Store, storage.URLSigner, error) {
	switch cfg.Backend {
	case "", "local":
		return storage.NewLocalStore(cfg.Dir), nil, nil
	case "cos":
		if cfg.COS.BucketURL == "" || cfg.COS.SecretID == "" || cfg.COS.SecretKey == "" {
			return nil, nil, xerr.BadRequest("COS 存储配置无效")
		}
		store, err := storage.NewCOSStore(storage.COSConfig{
			BucketURL: cfg.COS.BucketURL,
			SecretID:  cfg.COS.SecretID,
			SecretKey: cfg.COS.SecretKey,
			BaseURL:   cfg.COS.BaseURL,
		})
		if err != nil {
			return nil, nil, err
		}
		return store, store, nil
	default:
		return nil, nil, xerr.BadRequest("存储 backend 配置无效")
	}
}
