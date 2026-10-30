package core

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AutoImportResult 自动导入结果统计
type AutoImportResult struct {
	ScannedDirs []string `json:"scannedDirs"`
	FoundFiles  int      `json:"foundFiles"`
	Imported    int      `json:"imported"`
	Skipped     int      `json:"skipped"`
	Errors      []string `json:"errors,omitempty"`
}

// autoImportClientCredentials 启动时自动扫描官方客户端凭证目录并导入。
// 只导入从未导入过的新凭证（按 uid 去重），不覆盖已有凭证。
func autoImportClientCredentials(svc *Service) *AutoImportResult {
	cfg := svc.GetConfig()
	res := &AutoImportResult{}

	authDir := svc.AuthDir()
	existingUIDs := map[string]bool{}
	for _, a := range svc.Accounts() {
		existingUIDs[a.UID] = true
	}

	for _, dir := range cfg.ClientAuthDirs {
		dir = resolveTilde(dir)
		res.ScannedDirs = append(res.ScannedDirs, dir)

		matches, err := filepath.Glob(filepath.Join(dir, "workbuddy*.json"))
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("扫描 %s 失败: %v", dir, err))
			continue
		}

		for _, srcPath := range matches {
			res.FoundFiles++
			raw, err := os.ReadFile(srcPath)
			if err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("读取 %s 失败: %v", srcPath, err))
				continue
			}

			cred, err := ParseCredential(filepath.Base(srcPath), raw)
			if err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("解析 %s 失败: %v", srcPath, err))
				continue
			}
			if cred.UID == "" {
				res.Skipped++
				continue
			}
			if existingUIDs[cred.UID] {
				res.Skipped++
				continue
			}
			if cred.EncAuth {
				res.Errors = append(res.Errors, fmt.Sprintf("跳过 %s: token 为加密信封", srcPath))
				continue
			}

			// 拷贝到 auth_dir
			dstName := filepath.Base(srcPath)
			dstPath := filepath.Join(authDir, dstName)
			// 防重名：目标已存在则加序号
			for i := 1; ; i++ {
				if _, err := os.Stat(dstPath); os.IsNotExist(err) {
					break
				}
				ext := filepath.Ext(dstName)
				base := strings.TrimSuffix(dstName, ext)
				dstName = fmt.Sprintf("%s_%d%s", base, i, ext)
				dstPath = filepath.Join(authDir, dstName)
			}

			if err := os.MkdirAll(authDir, 0o700); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("创建目录失败: %v", err))
				continue
			}
			if err := os.WriteFile(dstPath, raw, 0o600); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("写入 %s 失败: %v", dstPath, err))
				continue
			}

			existingUIDs[cred.UID] = true
			res.Imported++
		}
	}

	return res
}

// resolveTilde 把 ~/ 前缀展开为用户主目录。
func resolveTilde(p string) string {
	p = strings.TrimSpace(p)
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(p, "~"), "/"))
	}
	return p
}

// needsSetup 判定当前是否需要引导用户完成初始设置：
//   - 凭证目录下无任何有效账号，且自动导入也未发现新凭证。
func needsSetup(svc *Service) bool {
	for _, a := range svc.Accounts() {
		if a.Status == "online" || a.Status == "expired" || a.Status == "unknown" {
			return false
		}
	}
	return true
}
