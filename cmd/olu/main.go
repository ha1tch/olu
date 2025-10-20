package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/ha1tch/olu/pkg/cache"
	"github.com/ha1tch/olu/pkg/config"
	"github.com/ha1tch/olu/pkg/graph"
	"github.com/ha1tch/olu/pkg/server"
	"github.com/ha1tch/olu/pkg/storage"
	"github.com/ha1tch/olu/pkg/validation"
)

func main() {
	// Setup logger
	logger := zerolog.New(os.Stdout).With().
		Timestamp().
		Logger().
		Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	
	// Load configuration
	cfg := config.Default()
	config.LoadFromEnv(cfg)
	
	// Print banner
	printBanner(cfg, logger)
	
	// Create directories
	if err := os.MkdirAll(cfg.BaseDir, 0755); err != nil {
		logger.Fatal().Err(err).Msg("Failed to create base directory")
	}
	if err := os.MkdirAll(cfg.SchemaDir, 0755); err != nil {
		logger.Fatal().Err(err).Msg("Failed to create schema directory")
	}
	
	// Initialize storage
	var storeConfig map[string]interface{}
	
	if cfg.StorageType == "sqlite" {
		storeConfig = map[string]interface{}{
			"db_path": cfg.DBPath,
		}
	} else {
		storeConfig = map[string]interface{}{
			"base_dir": cfg.BaseDir,
			"schema":   cfg.Schema,
		}
	}
	
	store, err := storage.NewStore(cfg.StorageType, storeConfig)
	if err != nil {
		logger.Fatal().Err(err).Msg("Failed to initialize storage")
	}
	defer store.Close()
	
	// Log store info
	if infoProvider, ok := store.(storage.InfoProvider); ok {
		info := infoProvider.Info()
		logger.Info().
			Str("type", info.Type).
			Str("version", info.Version).
			Bool("supports_search", info.SupportsSearch).
			Bool("supports_batch", info.SupportsBatch).
			Bool("supports_transaction", info.SupportsTransaction).
			Msg("Storage initialized")
	}
	
	// Initialize cache
	var cacheInstance cache.Cache
	if cfg.CacheType == "redis" {
		redisCache, err := cache.NewRedisCache(
			cfg.RedisHost,
			cfg.RedisPort,
			time.Duration(cfg.CacheTTL)*time.Second,
		)
		if err != nil {
			logger.Warn().Err(err).Msg("Failed to connect to Redis, falling back to memory cache")
			cacheInstance = cache.NewMemoryCache(cfg.CacheSize, time.Duration(cfg.CacheTTL)*time.Second)
		} else {
			cacheInstance = redisCache
			logger.Info().Msg("Using Redis cache")
		}
	} else {
		cacheInstance = cache.NewMemoryCache(cfg.CacheSize, time.Duration(cfg.CacheTTL)*time.Second)
		logger.Info().Msg("Using in-memory cache")
	}
	defer cacheInstance.Close()
	
	// Initialize graph
	var graphInstance graph.Graph
	if cfg.GraphEnabled && cfg.GraphMode == "indexed" {
		graphInstance = graph.NewIndexedGraph()
		
		// Load graph from file if exists
		graphFile := filepath.Join(cfg.BaseDir, cfg.GraphDataFile)
		if err := graphInstance.Load(graphFile); err != nil {
			logger.Warn().Err(err).Msg("Failed to load graph, starting with empty graph")
		} else {
			logger.Info().Msg("Loaded existing graph")
		}
		
		// Load entities into graph
		if err := loadEntitiesIntoGraph(cfg, store, graphInstance, logger); err != nil {
			logger.Error().Err(err).Msg("Failed to load entities into graph")
		}
		
		logger.Info().Msg("Graph initialized")
	} else {
		logger.Info().Msg("Graph disabled")
	}
	
	// Initialize validator
	validator := validation.NewJSONSchemaValidator(cfg.SchemaDir)
	if err := validator.LoadAllSchemas(); err != nil {
		logger.Warn().Err(err).Msg("Failed to load schemas")
	}
	
	// Create server
	srv := server.New(cfg, store, cacheInstance, graphInstance, validator, logger)
	
	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	
	go func() {
		<-sigChan
		logger.Info().Msg("Shutting down gracefully...")
		
		// Save graph
		if graphInstance != nil && cfg.GraphEnabled {
			graphFile := filepath.Join(cfg.BaseDir, cfg.GraphDataFile)
			if err := graphInstance.Save(graphFile); err != nil {
				logger.Error().Err(err).Msg("Failed to save graph")
			} else {
				logger.Info().Msg("Graph saved")
			}
		}
		
		os.Exit(0)
	}()
	
	// Start server
	logger.Info().Msg("Server ready to accept requests")
	if err := srv.Start(); err != nil {
		logger.Fatal().Err(err).Msg("Server failed")
	}
}

func printBanner(cfg *config.Config, logger zerolog.Logger) {
	// Light blue color code
	lightBlue := "\033[1;36m"
	reset := "\033[0m"
	
	// Print ASCII art in light blue
	fmt.Print(lightBlue)
	fmt.Println("//////////////////////////////////////////////")
	fmt.Println("//..........................................//")
	fmt.Println("//..........................................//")
	fmt.Println("//....._,gggggg,_...........................//")
	fmt.Println("//...,d8P\"\"d8P\"Y8b,....,dPYb,...............//")
	fmt.Println("//..,d8'...Y8...\"8b,dP.IP'`Yb...............//")
	fmt.Println("//..d8'....`Ybaaad88P'.I8..8I...............//")
	fmt.Println("//..8P.......`\"\"\"\"Y8...I8..8'...............//")
	fmt.Println("//..8b............d8...I8.dP..gg......gg....//")
	fmt.Println("//..Y8,..........,8P...I8dP...I8......8I....//")
	fmt.Println("//..`Y8,........,8P'...I8P....I8,....,8I....//")
	fmt.Println("//...`Y8b,,__,,d8P'...,d8b,_.,d8b,..,d8b,...//")
	fmt.Println("//.....`\"Y8888P\"'.....8P'\"Y888P'\"Y88P\"`Y8...//")
	fmt.Println("//..........................................//")
	fmt.Println("//..........................................//")
	fmt.Println("//..........................................//")
	fmt.Println("//..........................................//")
	fmt.Println("//..........................................//")
	fmt.Println("//////////////////////////////////////////////")
	fmt.Print(reset)
	
	fmt.Println()
	fmt.Println("//////////////////////////// olu " + config.Version + " /////////////////////////////")
	fmt.Println("----------------------------------------------------------------------")
	fmt.Println("Server Configuration:")
	fmt.Printf("  Host: %s\n", cfg.Host)
	fmt.Printf("  Port: %d\n", cfg.Port)
	fmt.Printf("  Schema: %s\n", cfg.Schema)
	fmt.Println()
	fmt.Println("Graph Configuration:")
	if cfg.GraphEnabled {
		fmt.Printf("  Mode: Enabled (%s)\n", cfg.GraphMode)
		fmt.Printf("  Query TTL: %d seconds\n", cfg.GraphQueryTTL)
		fmt.Printf("  Cycle Detection: %s\n", cfg.GraphCycleDetection)
	} else {
		fmt.Println("  Mode: Disabled")
	}
	fmt.Println()
	fmt.Println("Cache Configuration:")
	fmt.Printf("  Type: %s\n", cfg.CacheType)
	fmt.Printf("  TTL: %d seconds\n", cfg.CacheTTL)
	if cfg.CacheType == "redis" {
		fmt.Printf("  Redis: %s:%d\n", cfg.RedisHost, cfg.RedisPort)
	}
	fmt.Println()
	fmt.Println("Other Configuration:")
	fmt.Printf("  Full-text search: %v\n", cfg.FullTextEnabled)
	fmt.Printf("  Cascading delete: %v\n", cfg.CascadingDelete)
	fmt.Printf("  REF embed depth: %d\n", cfg.RefEmbedDepth)
	fmt.Printf("  Patch null handling: %s\n", cfg.PatchNullBehavior)
	fmt.Printf("  Max query depth: %d\n", cfg.MaxQueryDepth)
	fmt.Println("----------------------------------------------------------------------")
	fmt.Println()
}

func loadEntitiesIntoGraph(
	cfg *config.Config,
	store storage.Store,
	g graph.Graph,
	logger zerolog.Logger,
) error {
	ctx := context.Background()
	schemaPath := filepath.Join(cfg.BaseDir, cfg.Schema)
	
	entries, err := os.ReadDir(schemaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	
	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		
		entityName := entry.Name()
		entities, err := store.List(ctx, entityName)
		if err != nil {
			logger.Warn().Err(err).Str("entity", entityName).Msg("Failed to list entities")
			continue
		}
		
		for _, data := range entities {
			id, ok := data["id"].(float64)
			if !ok {
				if idInt, ok := data["id"].(int); ok {
					id = float64(idInt)
				} else {
					continue
				}
			}
			
			if err := g.UpdateFromEntity(entityName, int(id), data); err != nil {
				logger.Warn().Err(err).
					Str("entity", entityName).
					Int("id", int(id)).
					Msg("Failed to add entity to graph")
			} else {
				count++
			}
		}
	}
	
	logger.Info().Int("count", count).Msg("Loaded entities into graph")
	return nil
}
