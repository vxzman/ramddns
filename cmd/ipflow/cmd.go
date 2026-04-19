package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"ipflow/internal/config"
	"ipflow/internal/log"
	"ipflow/internal/platform/ifaddr"
	"ipflow/internal/provider/cloudflare"
	"ipflow/internal/provider/factory"
	"github.com/spf13/cobra"
)

var version = "dev"
var commit = ""
var buildDate = ""

func printVersion() {
	fmt.Printf("ipflow %s\n", version)
	if commit != "" {
		fmt.Printf("commit: %s\n", commit)
	}
	if buildDate != "" {
		fmt.Printf("built: %s\n", buildDate)
	}
}

var rootCmd = &cobra.Command{
	Use:   "ipflow",
	Short: "强大的动态 DNS 客户端 - 支持多域名多服务商",
}

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "运行 DDNS 更新",
	Run: func(cmd *cobra.Command, args []string) {
		// 解析参数
		configPath, _ := cmd.Flags().GetString("config")
		ignoreCache, _ := cmd.Flags().GetBool("ignore-cache")
		timeout, _ := cmd.Flags().GetInt("timeout")
		if configPath == "" {
			fmt.Fprintln(os.Stderr, "缺少配置文件参数：--config")
			os.Exit(1)
		}

		// 创建带超时的 context
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		// 监听信号，支持优雅退出
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigChan
			log.Info("Received shutdown signal, exiting...")
			cancel()
		}()

		// 读取配置
		cfg, absConfigFile := config.ReadConfig(configPath, false)
		if cfg == nil {
			fmt.Fprintln(os.Stderr, "Failed to load configuration")
			os.Exit(1)
		}

		// 初始化日志系统
		if err := log.Init(cfg.General.LogOutput); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
			os.Exit(1)
		}

		log.Info("ipflow starting with %d record(s)", len(cfg.Records))

		// 获取当前 IP 地址（get_ip 始终直连，不使用代理）
		var infos []ifaddr.IPv6Info
		var err error
		if cfg.General.GetIP.Interface != "" {
			infos, err = ifaddr.GetAvailableIPv6(cfg.General.GetIP.Interface)
			if err != nil {
				log.Info("Interface %s failed: %v", cfg.General.GetIP.Interface, err)
				log.Info("Trying fallback API...")
				infos, err = ifaddr.GetIPv6FromAPIs(cfg.General.GetIP.URLs, false)
				if err != nil {
					log.Error("Fallback also failed: %v", err)
					os.Exit(1)
				}
			}
		} else {
			infos, err = ifaddr.GetIPv6FromAPIs(cfg.General.GetIP.URLs, false)
			if err != nil {
				log.Error("Failed to get IP from APIs: %v", err)
				os.Exit(1)
			}
		}

		currentIP, err := ifaddr.SelectBestIPv6(infos)
		if err != nil {
			log.Error("Failed to select best IPv6 address: %v", err)
			os.Exit(1)
		}

		log.Info("Current IPv6 address: %s", currentIP)

		// 检查缓存
		cacheFilePath := config.GetCacheFilePath(absConfigFile, cfg.General.WorkDir)
		lastIP := config.ReadLastIP(cacheFilePath)

		if !ignoreCache {
			if lastIP != "" && lastIP == currentIP {
				log.Info("IP has not changed since last run: %s", currentIP)
			} else if lastIP != "" {
				log.Info("IP changed from %s to %s", lastIP, currentIP)
			}
		}

		// 批量更新所有记录
		updateRecords(ctx, cfg, currentIP, cacheFilePath, ignoreCache, lastIP)
	},
}

// updateRecords updates all DNS records in parallel
func updateRecords(ctx context.Context, cfg *config.Config, currentIP string, cacheFilePath string, ignoreCache bool, lastIP string) {
	var wg sync.WaitGroup
	results := make([]updateResult, len(cfg.Records))
	var mu sync.Mutex // 保护 results 的并发写入

	for i, record := range cfg.Records {
		wg.Add(1)
		go func(idx int, rec config.RecordConfig) {
			defer wg.Done()
			result := updateSingleRecord(ctx, cfg, &rec, currentIP, cacheFilePath, ignoreCache)
			mu.Lock()
			results[idx] = result
			mu.Unlock()
		}(i, record)
	}

	wg.Wait()

	// 汇总结果
	successCount := 0
	failCount := 0
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

	// 仅在 IP 发生变化且至少有一次成功时写入缓存，避免重复写入
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
func updateSingleRecord(ctx context.Context, cfg *config.Config, record *config.RecordConfig, currentIP string, cacheFilePath string, ignoreCache bool) updateResult {
	result := updateResult{record: fmt.Sprintf("%s.%s", record.Record, record.Zone)}

	// 检查 context 是否已取消
	select {
	case <-ctx.Done():
		result.err = ctx.Err()
		result.success = false
		return result
	default:
	}

	log.Info("Processing record: %s (%s)", result.record, record.Provider)

	// 使用工厂创建 Provider
	provider, err := factory.GetProvider(cfg, record)
	if err != nil {
		log.Error("Failed to create provider for %s: %v", result.record, err)
		result.err = err
		result.success = false
		return result
	}

	// 检查代理配置（仅用于日志记录）
	proxyURL := config.GetRecordProxy(cfg, record)
	if proxyURL != "" {
		// 检查 provider 是否支持代理
		if _, ok := provider.(interface{ GetProxy() string }); !ok {
			log.Warning("Provider %s does not support proxy, ignoring use_proxy setting for %s", record.Provider, result.record)
		}
	}

	// 获取记录特定配置
	ttl := config.GetRecordTTL(record)
	extra := make(map[string]interface{})

	// 处理 Cloudflare 特定配置
	if record.Provider == "cloudflare" {
		zoneID := record.Cloudflare.ZoneID
		cacheZoneIDPath := cacheFilePath + ".zoneid.json"

		// 如果没有配置 ZoneID，尝试从本地缓存读取
		if zoneID == "" {
			if cached := config.ReadZoneIDCache(cacheZoneIDPath); cached != nil {
				if v, ok := cached[record.Zone]; ok && v != "" {
					zoneID = v
					record.Cloudflare.ZoneID = v
				}
			}
		}

		// 如果仍然没有 ZoneID，则通过 API 获取
		if zoneID == "" {
			log.Info("Zone ID not configured, fetching for zone: %s", record.Zone)
			cfProvider, ok := provider.(*cloudflare.CloudflareProvider)
			if !ok {
				result.err = fmt.Errorf("failed to cast provider to CloudflareProvider")
				result.success = false
				return result
			}
			fetchedZoneID, err := cfProvider.GetZoneID(ctx, record.Zone)
			if err != nil {
				log.Error("Failed to fetch Zone ID: %v", err)
				result.err = fmt.Errorf("failed to get Zone ID: %w", err)
				result.success = false
				return result
			}
			zoneID = fetchedZoneID
			log.Info("Zone ID fetched: %s", zoneID)

			// 保存 ZoneID 到本地缓存文件（不写入包含敏感信息的完整配置）
			record.Cloudflare.ZoneID = zoneID
			if writeErr := config.UpdateZoneIDCache(cacheZoneIDPath, record.Zone, zoneID); writeErr != nil {
				log.Warning("Warning: Failed to save Zone ID cache: %v", writeErr)
			}
		}

		// 获取 TTL 和 Proxied 设置
		if record.Cloudflare.TTL > 0 {
			ttl = record.Cloudflare.TTL
		}
		extra["proxied"] = record.Proxied
		if record.Cloudflare.Proxied {
			extra["proxied"] = true
		}
		extra["zoneID"] = zoneID
	}

	// 更新 DNS 记录（通用逻辑）
	success, err := provider.UpsertRecord(ctx, record.Zone, record.Record, currentIP, ttl, extra)
	if err != nil {
		log.Error("Failed to update %s: %v", result.record, err)
		result.err = err
		result.success = false
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

// updateCache updates the cache file once per run
func updateCache(cacheFilePath string, currentIP string) {
	if writeErr := config.WriteLastIP(cacheFilePath, currentIP); writeErr != nil {
		log.Warning("Warning: Failed to write IP to cache: %v", writeErr)
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
	runCmd.Flags().StringP("config", "f", "", "配置文件路径 (JSON 格式)")
	runCmd.Flags().BoolP("ignore-cache", "i", false, "忽略缓存 IP，强制更新")
	runCmd.Flags().IntP("timeout", "t", 300, "超时时间（秒），默认 300 秒")
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(versionCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "命令执行失败：%v\n", err)
		os.Exit(1)
	}
}
