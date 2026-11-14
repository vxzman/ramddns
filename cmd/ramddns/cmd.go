package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"ramddns/internal/config"
	"ramddns/internal/log"
	"ramddns/internal/platform/ifaddr"
	"ramddns/internal/provider/cloudflare"
	"ramddns/internal/provider/factory"
)

var (
	version   = "dev"
	commit    = ""
	buildDate = ""
)

func printVersion() {
	fmt.Printf("ramddns %s\n", version)
	if commit != "" {
		fmt.Printf("commit: %s\n", commit)
	}
	if buildDate != "" {
		fmt.Printf("built: %s\n", buildDate)
	}
}

var rootCmd = &cobra.Command{
	Use:   "ramddns",
	Short: "轻量级 DDNS 客户端 - 支持多域名多服务商",
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "运行 DDNS 更新",
	Run: func(cmd *cobra.Command, args []string) {
		configPath, _ := cmd.Flags().GetString("config")
		dirPath, _ := cmd.Flags().GetString("dir")
		ignoreCache, _ := cmd.Flags().GetBool("ignore")
		timeout, _ := cmd.Flags().GetInt("timeout")
		netnsFlag, _ := cmd.Flags().GetString("netns")

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

		if err := log.Init(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
			os.Exit(1)
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
		if err := config.ApplyNetNSOverride(cfg, netnsFlag); err != nil {
			log.Error("Invalid config", "err", err)
			os.Exit(1)
		}

		log.Info("ramddns starting", "records", len(cfg.Records), "netns", cfg.IPSource.NetNS)

		currentIP, err := getCurrentIP(ctx, cfg)
		if err != nil {
			log.Error("Failed to get current IP", "err", err)
			var probeErr *ifaddr.ProbeError
			if errors.As(err, &probeErr) {
				os.Exit(probeErr.Code)
			}
			os.Exit(1)
		}

		log.Info("Current IPv6 address", "ip", currentIP)

		cacheFilePath := config.GetCacheFilePath(absConfigFile, dirPath)
		cacheData := config.ParseCacheFile(cacheFilePath)
		lastIP := cacheData.LastIP

		if !ignoreCache {
			if lastIP != "" && lastIP == currentIP {
				log.Info("IP has not changed since last run", "ip", currentIP)
				return
			} else if lastIP != "" {
				log.Info("IP changed", "from", lastIP, "to", currentIP)
			}
		} else {
			log.Info("Ignoring cache (--ignore), forcing DDNS update")
		}

		updateRecords(ctx, cfg, currentIP, cacheFilePath, lastIP, ignoreCache)
	},
}

// getCurrentIP gets the current IPv6 address.
// A configured netns is probed only inside the re-exec child. Failure does
// not fall back to the host interfaces or the host HTTP APIs.
func getCurrentIP(ctx context.Context, cfg *config.Config) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if cfg.IPSource.NetNS != "" {
		result, err := ifaddr.ProbeNetNS(ctx, cfg.IPSource.NetNS, cfg.IPSource.Interface, cfg.IPSource.FallbackURLs)
		if err != nil {
			return "", err
		}
		return result.IP, nil
	}

	var infos []ifaddr.IPv6Info
	var err error

	if cfg.IPSource.Interface != "" {
		infos, err = ifaddr.GetAvailableIPv6(cfg.IPSource.Interface)
		if err != nil {
			log.Info("Interface failed", "interface", cfg.IPSource.Interface, "err", err)
			log.Info("Trying fallback API...")
			infos, err = ifaddr.GetIPv6FromAPIs(ctx, cfg.IPSource.FallbackURLs, false)
			if err != nil {
				return "", err
			}
		}
	} else {
		infos, err = ifaddr.GetIPv6FromAPIs(ctx, cfg.IPSource.FallbackURLs, false)
		if err != nil {
			return "", err
		}
	}

	return ifaddr.SelectBestIPv6(infos)
}

// updateRecords updates all DNS records in parallel.
func updateRecords(ctx context.Context, cfg *config.Config, currentIP string, cacheFilePath string, lastIP string, ignoreCache bool) {
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

	log.Info("Update completed", "succeeded", successCount, "failed", failCount)

	if !ignoreCache && anySuccess && lastIP != currentIP {
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

func updateSingleRecord(ctx context.Context, cfg *config.Config, record *config.RecordConfig, currentIP string, cacheFilePath string) updateResult {
	result := updateResult{record: config.RecordFQDN(record)}

	select {
	case <-ctx.Done():
		result.err = ctx.Err()
		return result
	default:
	}

	log.Info("Processing record", "record", result.record, "provider", record.Provider)

	provider, err := factory.GetProvider(cfg, record)
	if err != nil {
		log.Error("Failed to create provider", "record", result.record, "err", err)
		result.err = err
		return result
	}

	if record.Provider == "cloudflare" {
		if err := setupCloudflareRecord(ctx, provider, record, cacheFilePath); err != nil {
			result.err = err
			return result
		}
	}

	ttl := config.GetRecordTTL(record)
	extra := buildExtraConfig(record)

	success, err := provider.UpsertRecord(ctx, record.Zone, record.Name, currentIP, ttl, extra)
	if err != nil {
		log.Error("Failed to update record", "record", result.record, "err", err)
		result.err = err
		return result
	}

	if success {
		log.Success("Record updated successfully", "record", result.record)
		result.success = true
	} else {
		log.Error("Record update returned false", "record", result.record)
		result.success = false
	}
	return result
}

func setupCloudflareRecord(ctx context.Context, provider any, record *config.RecordConfig, cacheFilePath string) error {
	cfProvider, ok := provider.(*cloudflare.CloudflareProvider)
	if !ok {
		return fmt.Errorf("failed to cast provider to CloudflareProvider")
	}

	cacheZoneIDPath := filepath.Join(filepath.Dir(cacheFilePath), "cache.zoneid.json")
	zoneID := record.ZoneID

	if zoneID == "" {
		if cached := config.ReadZoneIDCache(cacheZoneIDPath); cached != nil {
			zoneID = cached[record.Zone]
			record.ZoneID = zoneID
		}
	}

	if zoneID == "" {
		log.Info("Zone ID not configured, fetching", "zone", record.Zone)
		var err error
		zoneID, err = cfProvider.GetZoneID(ctx, record.Zone)
		if err != nil {
			log.Error("Failed to fetch Zone ID", "err", err)
			return fmt.Errorf("failed to get Zone ID: %w", err)
		}
		record.ZoneID = zoneID
		log.Info("Zone ID fetched", "zone_id", zoneID)

		if err := config.UpdateZoneIDCache(cacheZoneIDPath, record.Zone, zoneID); err != nil {
			log.Warning("Failed to save Zone ID cache", "err", err)
		}
	}

	return nil
}

func buildExtraConfig(record *config.RecordConfig) map[string]interface{} {
	extra := make(map[string]interface{})
	if record.Provider == "cloudflare" {
		extra["proxied"] = record.Proxied
		extra["zoneID"] = record.ZoneID
	}
	return extra
}

func updateCache(cacheFilePath string, currentIP string) {
	if err := config.WriteLastIP(cacheFilePath, currentIP); err != nil {
		log.Warning("Failed to write IP to cache", "err", err)
	}
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "显示版本信息",
	Run: func(cmd *cobra.Command, args []string) {
		printVersion()
	},
}

// probeCmd is the re-exec child. It must not call log.Init: stdout is JSON only.
var probeCmd = &cobra.Command{
	Use:    "__probe",
	Hidden: true,
	Run: func(cmd *cobra.Command, args []string) {
		ns, _ := cmd.Flags().GetString("netns")
		iface, _ := cmd.Flags().GetString("iface")
		rawURLs, _ := cmd.Flags().GetString("fallback-urls")
		os.Exit(ifaddr.RunProbe(ns, iface, ifaddr.SplitList(rawURLs)))
	},
}

func Execute() {
	runCmd.Flags().StringP("config", "c", "", "配置文件路径 (JSON 格式)")
	runCmd.Flags().StringP("dir", "d", "", "工作目录（用于存放缓存和相对日志路径）")
	runCmd.Flags().BoolP("ignore", "i", false, "忽略缓存，强制更新且不写入缓存")
	runCmd.Flags().IntP("timeout", "t", 300, "超时时间（秒），默认 300 秒")
	runCmd.Flags().String("netns", "", "Linux 网络命名空间，覆盖配置文件。地址探测在该 netns 内进行")

	probeCmd.Flags().String("netns", "", "network namespace name or /run/netns path")
	probeCmd.Flags().String("iface", "", "interface name")
	probeCmd.Flags().String("fallback-urls", "", "comma-separated fallback URLs")

	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(probeCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "命令执行失败：%v\n", err)
		os.Exit(1)
	}
}
