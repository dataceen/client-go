// Codegen CLI for the Dataceen Go client.
//
// Usage:
//
//	go run ./tools/codegen/cmd schema           # fetch + cache full introspection
//	go run ./tools/codegen/cmd schema --print   # print stats from cached schema
//	go run ./tools/codegen/cmd emit Customer    # emit Customer + transitive types
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/joho/godotenv"

	"github.com/dataceen/client-go/pkg/dataceen"
	"github.com/dataceen/client-go/tools/codegen/internal/emit"
	"github.com/dataceen/client-go/tools/codegen/internal/schema"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	switch cmd {
	case "schema":
		if err := runSchema(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "emit":
		if err := runEmit(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `dataceen codegen

Subcommands:
  schema         Fetch full GraphQL introspection from the live backend and
                 write tools/codegen/cache/schema.json.
  schema --print Re-print stats from the cached file without re-fetching.
  emit ENTITY    Emit Go source for ENTITY (and the entities it transitively
                 references) into pkg/generated/. Pass multiple ENTITY names
                 to emit several roots in one run.

Flags for emit:
  -out PATH      Output dir (default: pkg/generated)
  -pkg NAME      Package name in the emitted files (default: generated)
  -cache PATH    Cached schema to read (default: tools/codegen/cache/schema.json)`)
}

func runSchema(args []string) error {
	fs := flag.NewFlagSet("schema", flag.ExitOnError)
	cachePath := fs.String("out", schema.CachePath, "where to write the cached schema")
	printOnly := fs.Bool("print", false, "skip fetch; print stats from existing cache")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *printOnly {
		cached, err := schema.ReadCache(*cachePath)
		if err != nil {
			return err
		}
		if cached == nil {
			return fmt.Errorf("no cache at %s — run without --print to fetch", *cachePath)
		}
		printStats(cached)
		return nil
	}

	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Printf("[codegen] fetching introspection from %s/%s/%s/%s ...\n",
		cfg.APIURL+cfg.APIPath, cfg.Domain, cfg.Model, cfg.Scope)
	cached, err := schema.Fetch(ctx, cfg)
	if err != nil {
		return err
	}
	if err := schema.WriteCache(*cachePath, cached); err != nil {
		return err
	}
	fmt.Printf("[codegen] wrote %s\n", *cachePath)
	printStats(cached)
	return nil
}

func runEmit(args []string) error {
	fs := flag.NewFlagSet("emit", flag.ExitOnError)
	outDir := fs.String("out", "pkg/generated", "output dir for generated Go files")
	pkgName := fs.String("pkg", "generated", "package name in emitted files")
	cachePath := fs.String("cache", schema.CachePath, "cached schema to parse")
	if err := fs.Parse(args); err != nil {
		return err
	}
	roots := fs.Args()
	if len(roots) == 0 {
		return fmt.Errorf("emit: pass at least one entity name (e.g. `emit Customer`)")
	}

	cached, err := schema.ReadCache(*cachePath)
	if err != nil {
		return err
	}
	if cached == nil {
		return fmt.Errorf("no cache at %s — run `schema` first", *cachePath)
	}
	model := schema.Parse(cached)

	res, err := emit.Run(model, roots, *outDir, *pkgName)
	if res != nil {
		fmt.Printf("[codegen] roots: %v\n", roots)
		fmt.Printf("[codegen] entities discovered (BFS): %d\n", len(res.Entities))
		fmt.Printf("[codegen] files written: %d into %s\n", len(res.FilesWritten), res.OutDir)
		if len(res.FilesSkipped) > 0 {
			fmt.Printf("[codegen] files skipped (not applicable): %d\n", len(res.FilesSkipped))
		}
	}
	return err
}

func loadConfig() (dataceen.Config, error) {
	// Look for .env at repo root (CLI runs from any subdir).
	for _, p := range []string{".env", "../.env", "../../.env", "../../../.env"} {
		if _, err := os.Stat(p); err == nil {
			_ = godotenv.Load(p)
			break
		}
	}
	cfg := dataceen.Config{
		TenantID:          os.Getenv("DATACEEN_TENANT_ID"),
		ClientID:          os.Getenv("DATACEEN_CLIENT_ID"),
		ClientSecret:      os.Getenv("DATACEEN_CLIENT_SECRET"),
		Domain:            os.Getenv("DATACEEN_DOMAIN"),
		Model:             os.Getenv("DATACEEN_MODEL"),
		Scope:             os.Getenv("DATACEEN_SCOPE"),
		APIURL:            os.Getenv("DATACEEN_API_URL"),
		APIPath:           os.Getenv("DATACEEN_API_PATH"),
		APIScope:          os.Getenv("DATACEEN_API_SCOPE"),
		SubscriptionURL:   os.Getenv("DATACEEN_SUBSCRIPTION_URL"),
		SubscriptionScope: os.Getenv("DATACEEN_SUBSCRIPTION_SCOPE"),
	}
	if cfg.ClientSecret == "" {
		return cfg, fmt.Errorf("DATACEEN_CLIENT_SECRET is empty (looked for .env in cwd, ../, ../../, ../../../)")
	}
	return cfg, nil
}

func printStats(c *schema.CachedSchema) {
	fmt.Printf("[codegen] fetched at: %s\n", c.Metadata.FetchedAt)
	fmt.Printf("[codegen] endpoint:   %s\n", c.Metadata.Endpoint)
	fmt.Printf("[codegen] total types: %d\n", c.Metadata.Stats.TotalTypes)
	kinds := make([]string, 0, len(c.Metadata.Stats.ByKind))
	for k := range c.Metadata.Stats.ByKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Printf("[codegen]   %-14s %d\n", k, c.Metadata.Stats.ByKind[k])
	}
}
