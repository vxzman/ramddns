package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"aiolos/internal/config"
	"aiolos/internal/log"
	"aiolos/internal/platform/ifaddr"
	"aiolos/internal/provider/cloudflare"
	"aiolos/internal/provider/factory"
	"github.com/spf13/cobra"
)

var (
	version   = "dev"
	commit    = ""
	buildDate = ""
)

func printVersion() {
	fmt.Printf("aiolos %s\n", version)
	if commit != "" {
		fmt.Printf("commit: %s\n", commit)
	}
	if buildDate != "" {
		fmt.Printf("built: %s\n", buildDate)
	}
}

var rootCmd = &cobra.Command{
	Use:   "aiolos",
	Short: "强大的动态 DNS 客户端 - 支持多域名多服务商",
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "运行 DDNS 更新",
	Run: func(cmd *cobra.Command, args []string) {
		configPath, _ := cmd.Flags().GetString("config")
		dirPath, _ := cmd.Flags().GetString("dir")
		ignoreCache, _ := cmd.Flags().GetBool("ignore-cache")
		timeout, _ := cmd.Flags().GetInt("timeout")

		if configPath == "" {
			if dirPath == "" {
				fmt.Fprintln(os.Stderr, "缺少配置文件参数：--config，或请通过--dir 指定工作目录以在其中查找 config.json")
				os.Exit(1)
			}
			configPath = filepath.Join(dirPath, "config.json")
			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "配置文件未找到：%s\n", configPath)
				os.Exit(1)
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigChan
			log.Info("Received shutdown signal, exiting...")
			cancel()
		}()

		cfg, absConfigFile := config.ReadConfig(configPath, false)
		if cfg == nil {
			fmt.Fprintln(os.Stderr, "Failed to load configuration")
			os.Exit(1)
		}

		baseDir := dirPath
		if baseDir == "" {
			baseDir = filepath.Dir(absConfigFile)
		}

		if err := config.ResolveSecrets(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to resolve secrets: %v\n", err)
			os.Exit(1)
		}

		logOutput := cfg.General.LogOutput
		if logOutput != "" && !filepath.IsAbs(logOutput) {
			logOutput = filepath.Join(baseDir, logOutput)
			if dir := filepath.Dir(logOutput); dir != "" {
				if err := os.MkdirAll(dir, 0755); err != nil {
					fmt.Fprintf(os.Stderr, "Failed to create log directory: %v\n", err)
					os.Exit(1)
				}
			}
		}
		if err := log.Init(logOutput); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
			os.Exit(1)
		}

		log.Info("aiolos starting with %d record(s)", len(cfg.Records))

		currentIP, err := getCurrentIP(cfg)
		if err != nil {
			log.Error("Failed to get current IP: %v", err)
			os.Exit(1)
		}

		log.Info("Current IPv6 address: %s", currentIP)

		cacheFilePath := config.GetCacheFilePath(absConfigFile, dirPath)
		lastIP := config.ReadLastIP(cacheFilePath)

		if !ignoreCache {
			if lastIP != "" && lastIP == currentIP {
				log.Info("IP has not changed since last run: %s", currentIP)
			} else if lastIP != "" {
				log.Info("IP changed from %s to %s", lastIP, currentIP)
			}
		}

		updateRecords(ctx, cfg, currentIP, cacheFilePath, lastIP)
	},
}

// getCurrentIP gets the current IPv6 address
func getCurrentIP(cfg *config.Config) (string, error) {
	var infos []ifaddr.IPv6Info
	var err error

	if cfg.General.GetIP.Interface != "" {
		infos, err = ifaddr.GetAvailableIPv6(cfg.General.GetIP.Interface)
		if err != nil {
			log.Info("Interface %s failed: %v", cfg.General.GetIP.Interface, err)
			log.Info("Trying fallback API...")
			infos, err = ifaddr.GetIPv6FromAPIs(cfg.General.GetIP.URLs, false)
			if err != nil {
				return "", err
			}
		}
	} else {
		infos, err = ifaddr.GetIPv6FromAPIs(cfg.General.GetIP.URLs, false)
		if err != nil {
			return "", err
		}
	}

	return ifaddr.SelectBestIPv6(infos)
}

// updateRecords updates all DNS records in parallel
func updateRecords(ctx context.Context, cfg *config.Config, currentIP string, cacheFilePath string, lastIP string) {
	var wg sync.WaitGroup
	results := make([]updateResult, len(cfg.Records))
	var mu sync.Mutex

	for i, record := range cfg.Records {
		wg.Add(1)
		go func(idx int, rec config.RecordConfig) {
			defer wg.Done()
			result := updateSingleRecord(ctx, cfg, &rec, currentIP, cacheFilePath)
			mu.Lock()
			results[idx] = result
			mu.Unlock()
		}(i, record)
	}

	wg.Wait()

	successCount, failCount := 0, 0
	anySuccess := false
	for _, result := range results {
		if result.success {
			successCount++
			anySuccess = true
		} else {
			failCount++
		}
	}

	log.Info("Update completed: %d succeeded, %d failed", successCount, failCount)

	if anySuccess && lastIP != currentIP {
		updateCache(cacheFilePath, currentIP)
	}

	if failCount > 0 {
		os.Exit(1)
	}
}

type updateResult struct {
	success bool
	err     error
	record  string
}

// updateSingleRecord updates a single DNS record
func updateSingleRecord(ctx context.Context, cfg *config.Config, record *config.RecordConfig, currentIP string, cacheFilePath string) updateResult {
	result := updateResult{record: fmt.Sprintf("%s.%s", record.Record, record.Zone)}

	select {
	case <-ctx.Done():
		result.err = ctx.Err()
		return result
	default:
	}

	log.Info("Processing record: %s (%s)", result.record, record.Provider)

	provider, err := factory.GetProvider(cfg, record)
	if err != nil {
		log.Error("Failed to create provider for %s: %v", result.record, err)
		result.err = err
		return result
	}

	// Setup Cloudflare specific configuration
	if record.Provider == "cloudflare" {
		if err := setupCloudflareRecord(ctx, provider, record, cacheFilePath); err != nil {
			result.err = err
			return result
		}
	}

	ttl := config.GetRecordTTL(record)
	extra := buildExtraConfig(record)

	success, err := provider.UpsertRecord(ctx, record.Zone, record.Record, currentIP, ttl, extra)
	if err != nil {
		log.Error("Failed to update %s: %v", result.record, err)
		result.err = err
		return result
	}

	if success {
		log.Success("Record %s updated successfully", result.record)
		result.success = true
	} else {
		log.Error("Record %s update returned false", result.record)
		result.success = false
	}
	return result
}

// setupCloudflareRecord sets up Cloudflare specific configuration
func setupCloudflareRecord(ctx context.Context, provider any, record *config.RecordConfig, cacheFilePath string) error {
	cfProvider, ok := provider.(*cloudflare.CloudflareProvider)
	if !ok {
		return fmt.Errorf("failed to cast provider to CloudflareProvider")
	}

	cacheZoneIDPath := cacheFilePath + ".zoneid.json"
	zoneID := record.Cloudflare.ZoneID

	// Try cache first
	if zoneID == "" {
		if cached := config.ReadZoneIDCache(cacheZoneIDPath); cached != nil {
			zoneID = cached[record.Zone]
			record.Cloudflare.ZoneID = zoneID
		}
	}

	// Fetch from API if not in cache
	if zoneID == "" {
		log.Info("Zone ID not configured, fetching for zone: %s", record.Zone)
		var err error
		zoneID, err = cfProvider.GetZoneID(ctx, record.Zone)
		if err != nil {
			log.Error("Failed to fetch Zone ID: %v", err)
			return fmt.Errorf("failed to get Zone ID: %w", err)
		}
		record.Cloudflare.ZoneID = zoneID
		log.Info("Zone ID fetched: %s", zoneID)

		if err := config.UpdateZoneIDCache(cacheZoneIDPath, record.Zone, zoneID); err != nil {
			log.Warning("Warning: Failed to save Zone ID cache: %v", err)
		}
	}

	return nil
}

// buildExtraConfig builds extra configuration map for provider
func buildExtraConfig(record *config.RecordConfig) map[string]interface{} {
	extra := make(map[string]interface{})
	if record.Provider == "cloudflare" {
		extra["proxied"] = record.Cloudflare.Proxied
		extra["zoneID"] = record.Cloudflare.ZoneID
	}
	return extra
}

// updateCache updates the cache file once per run
func updateCache(cacheFilePath string, currentIP string) {
	if err := config.WriteLastIP(cacheFilePath, currentIP); err != nil {
		log.Warning("Warning: Failed to write IP to cache: %v", err)
	}
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "显示版本信息",
	Run: func(cmd *cobra.Command, args []string) {
		printVersion()
	},
}

func Execute() {
	runCmd.Flags().StringP("config", "c", "", "配置文件路径 (JSON 格式)")
	runCmd.Flags().StringP("dir", "d", "", "工作目录（用于存放缓存和相对日志路径）")
	runCmd.Flags().BoolP("ignore-cache", "i", false, "忽略缓存 IP，强制更新")
	runCmd.Flags().IntP("timeout", "t", 300, "超时时间（秒），默认 300 秒")
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(versionCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "命令执行失败：%v\n", err)
		os.Exit(1)
	}
}
