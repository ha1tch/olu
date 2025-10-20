package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/ha1tch/olu/pkg/storage"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: olu-migrate <source-dir> <target-db>")
		fmt.Println("Example: olu-migrate ./data/default ./olu.db")
		os.Exit(1)
	}

	sourceDir := os.Args[1]
	targetDB := os.Args[2]

	if err := migrate(sourceDir, targetDB); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Migration completed successfully!")
}

func migrate(sourceDir, targetDB string) error {
	ctx := context.Background()

	// Check if source exists
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("source directory does not exist: %s", sourceDir)
	}

	// Check if target exists
	if _, err := os.Stat(targetDB); err == nil {
		return fmt.Errorf("target database already exists: %s (delete it first)", targetDB)
	}

	// Create source store (JSONFile)
	fmt.Println("Opening source (JSONFile)...")
	baseDir := filepath.Dir(sourceDir)
	schema := filepath.Base(sourceDir)

	sourceStore, err := storage.NewStore("jsonfile", map[string]interface{}{
		"base_dir": baseDir,
		"schema":   schema,
	})
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer sourceStore.Close()

	// Create target store (SQLite)
	fmt.Println("Creating target (SQLite)...")
	targetStore, err := storage.NewStore("sqlite", map[string]interface{}{
		"db_path": targetDB,
	})
	if err != nil {
		return fmt.Errorf("failed to create target: %w", err)
	}
	defer targetStore.Close()

	// Get all entity types from source directory
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return fmt.Errorf("failed to read source directory: %w", err)
	}

	totalEntities := 0
	totalEdges := 0

	// Migrate each entity type
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		entityType := entry.Name()
		fmt.Printf("Migrating %s...\n", entityType)

		// List all entities of this type
		entities, err := sourceStore.List(ctx, entityType)
		if err != nil {
			fmt.Printf("  Warning: failed to list %s: %v\n", entityType, err)
			continue
		}

		// Migrate each entity
		for _, entity := range entities {
			// Get ID
			id, ok := entity["id"].(int)
			if !ok {
				if idFloat, ok := entity["id"].(float64); ok {
					id = int(idFloat)
				} else {
					fmt.Printf("  Warning: entity without valid ID: %v\n", entity)
					continue
				}
			}

			// Save to target
			if err := targetStore.Save(ctx, entityType, id, entity); err != nil {
				return fmt.Errorf("failed to migrate %s:%d: %w", entityType, id, err)
			}

			totalEntities++

			// Count REF fields (edges)
			for _, value := range entity {
				if valueMap, ok := value.(map[string]interface{}); ok {
					if valueMap["type"] == "REF" {
						totalEdges++
					}
				}
			}
		}

		fmt.Printf("  Migrated %d entities\n", len(entities))
	}

	fmt.Printf("\nMigration summary:\n")
	fmt.Printf("  Total entities: %d\n", totalEntities)
	fmt.Printf("  Total edges: %d\n", totalEdges)

	// Verify graph integrity
	fmt.Println("\nVerifying graph integrity...")
	type IntegrityChecker interface {
		VerifyGraphIntegrity(ctx context.Context) error
	}

	if checker, ok := targetStore.(IntegrityChecker); ok {
		if err := checker.VerifyGraphIntegrity(ctx); err != nil {
			return fmt.Errorf("graph integrity check failed: %w", err)
		}
		fmt.Println("  Graph integrity verified ✓")
	}

	return nil
}
