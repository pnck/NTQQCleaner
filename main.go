// qq-cleaner: a safer QQ cache cleaner (docs/README.md).
//
// Subcommands (docs/04 §8):
//
//	qq-cleaner scan  [--root R] [--account HASH] [--json] ...   dry-run scan
//	qq-cleaner clean --file manifest.json [--force] ...         execute a manifest
//	qq-cleaner gui                                             embedded web UI (default)
//
// The default build is CLI-only; the GUI is included with `-tags wails`
// (see Makefile). Both share the same internal/ logic and redlines.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"qqcleaner/internal/app"
	"qqcleaner/internal/discovery"
	"qqcleaner/internal/qq"
	"qqcleaner/internal/report"
	"qqcleaner/internal/rules"
)

// version is stamped at build time:
//
//	go build -ldflags "-X main.version=v1.0.0"
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := "gui" // default command
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "scan":
		return scanCmd(args)
	case "clean":
		return cleanCmd(args)
	case "gui":
		return runGUI()
	case "version", "-v", "--version":
		fmt.Println("qq-cleaner", version)
		return nil
	case "help", "-h", "--help":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", cmd, usageText)
	}
}

func usage() { fmt.Print(usageText) }

const usageText = `qq-cleaner — safer QQ cache cleaner (default: dry-run)

Usage:
  qq-cleaner [gui]                              start the GUI (default)
  qq-cleaner scan [flags]                       dry-run scan and report
  qq-cleaner clean --file manifest.json [flags] execute a scan manifest
  qq-cleaner version

scan flags:
  --root PATH          QQ data root (default: auto-detect)
  --account HASH       only this account instance (nt_qq_<hash>)
  --only-biz NAME      only this biz dir (repeatable: Pic Video Ptt File dataline Emoji)
  --min-age-days N     skip files newer than N days (default 3)
  --min-size BYTES     skip files smaller than this (default 0)
  --config PATH        config file (default ~/.qq-cleaner/config.yaml)
  --json               emit a JSON manifest (feed it to the clean command)

clean flags:
  --file PATH          manifest from scan --json (required)
  --force              required for any deletion (redline)
  --backup-dir PATH    move files there instead of deleting (recommended)
  --audit-log PATH     JSONL audit log (default ~/.qq-cleaner/audit.log)
  --config PATH        config file

Redlines (always enforced, see docs/06): dry-run by default, path
whitelist/blacklist re-verified per file, QQ-running guard (--ignore-running
覆盖需显式确认), audit log for every deletion, explicit --force + confirmation.
`

// configPath returns the default user config path (~/.qq-cleaner/config.yaml).
func configPath() string { return filepath.Join(app.ConfigDir(), "config.yaml") }

// loadConfig reads --config or the default user config, falling back to
// built-in defaults when absent.
func loadConfig(path string) (rules.Config, error) {
	if path == "" {
		path = configPath()
		if _, err := os.Stat(path); err != nil {
			return rules.Default(), nil
		}
	}
	return rules.Load(path)
}

func scanCmd(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	var (
		root       = fs.String("root", "", "")
		account    = fs.String("account", "", "")
		configPath = fs.String("config", "", "")
		minAgeDays = fs.Int("min-age-days", rules.DefaultMinAgeDays, "")
		minSize    = fs.Int64("min-size", 0, "")
		asJSON     = fs.Bool("json", false, "")
		onlyBizs   = multiFlag{}
	)
	fs.Var(&onlyBizs, "only-biz", "")
	fs.Parse(args)

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	if *root == "" {
		if *root, err = autoDetectRoot(); err != nil {
			return err
		}
	}

	var accounts []string
	if *account != "" {
		accounts = []string{*account}
	}
	out, err := (&app.Engine{Cfg: cfg}).ScanAll(context.Background(), *root, accounts, onlyBizs.list, *minAgeDays, *minSize, qq.AllGates())
	if err != nil {
		return err
	}
	if *asJSON {
		return printManifest(out)
	}
	printScanReport(out)
	return nil
}

func autoDetectRoot() (string, error) {
	cands := qq.RootCandidates()
	for _, c := range cands {
		if discovery.IsInstanceRoot(c) {
			return c, nil
		}
	}
	return "", fmt.Errorf("no QQ data root found (tried: %s); pass --root", strings.Join(cands, ", "))
}

func printScanReport(out *app.Outcome) {
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "QQ data root:", out.Root)
	for _, a := range out.Accounts {
		fmt.Fprintf(w, "\n账号 %s\t→ QQ %s\t(最新月份 %s)\n", shortHash(a.Hash), orUnknown(a.QQNum), a.LatestMonth)
		fmt.Fprintf(w, "  文件数\t%d\n", a.TotalFiles)
		fmt.Fprintf(w, "  总大小\t%s\n", humanSize(a.TotalSize))
		fmt.Fprintf(w, "  内容哈希\t%d\n", a.HashedFiles)
	}
	w.Flush()
}

func printManifest(out *app.Outcome) error {
	m := report.Manifest{
		Version: 1,
		Root:    out.Root,
	}
	for _, a := range out.Accounts {
		ma := report.ManifestAccount{Hash: a.Hash, QQNum: a.QQNum, NtData: a.NtData}
		for _, id := range out.ByAccount[a.Hash] {
			e := out.Entries[id]
			ma.Entries = append(ma.Entries, report.ManifestEntry{
				Path:        e.Path,
				Size:        e.Size,
				MTime:       e.MTime,
				MD5:         e.MD5,
				Biz:         e.Biz,
				Sub:         e.Sub,
				Month:       e.Month,
				Reason:      out.Reasons[id],
				ContentHash: e.ContentHash,
			})
		}
		m.Accounts = append(m.Accounts, ma)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(m)
}

func cleanCmd(args []string) error {
	fs := flag.NewFlagSet("clean", flag.ExitOnError)
	var (
		file       = fs.String("file", "", "")
		force      = fs.Bool("force", false, "")
		backupDir  = fs.String("backup-dir", "", "")
		auditLog   = fs.String("audit-log", "", "")
		configPath = fs.String("config", "", "")
		ignoreQQ   = fs.Bool("ignore-running", false, "")
	)
	fs.Parse(args)

	if *file == "" {
		return fmt.Errorf("--file is required (manifest from `scan --json`)")
	}
	if !*force {
		return fmt.Errorf("refusing: --force is required for any deletion (redline)")
	}
	data, err := os.ReadFile(*file)
	if err != nil {
		return err
	}
	var m report.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("parse manifest: %w", err)
	}
	cfg, err := loadConfig(*configPath)
	if err != nil {
		return err
	}
	if *auditLog == "" {
		*auditLog = filepath.Join(app.ConfigDir(), "audit.log")
	}

	// 清单中的全部条目都进入清理流程；每个文件在 clean.Run 内逐条
	// 重新校验白名单/黑名单（config 的类别门控 + 结构红线），未通过
	// 的跳过并计入 skipped。
	var files []classifyEntry
	var bytes int64
	for _, a := range m.Accounts {
		for _, e := range a.Entries {
			files = append(files, classifyEntry{path: e.Path, size: e.Size, mtime: e.MTime, md5: e.MD5, biz: e.Biz, sub: e.Sub, month: e.Month})
			bytes += e.Size
		}
	}
	if len(files) == 0 {
		fmt.Println("manifest contains no files")
		return nil
	}
	fmt.Printf("将处理 %d 个文件 (%s)；逐文件重新校验白名单/黑名单\n", len(files), humanSize(bytes))
	if *backupDir != "" {
		fmt.Printf("备份目录: %s\n", *backupDir)
	} else {
		fmt.Println("无备份目录：审计日志记录路径/大小/时间")
	}
	fmt.Print("输入 yes 确认: ")
	var confirm string
	fmt.Scanln(&confirm)
	if confirm != "yes" {
		return fmt.Errorf("aborted (confirmation required)")
	}

	res, err := cleanRun(m, files, *backupDir, *auditLog, cfg, *ignoreQQ)
	if err != nil {
		return err
	}
	fmt.Printf("完成: 处理 %d, 移动 %d, 删除 %d, 跳过 %d, 失败 %d, 释放 %s\n",
		res.Processed, res.Moved, res.Deleted, res.Skipped, res.Failed, humanSize(res.BytesFreed))
	for _, e := range res.Errors {
		fmt.Fprintln(os.Stderr, "  !", e)
	}
	return nil
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12] + "…"
	}
	return h
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}

func humanSize(n int64) string {
	const (
		KB = 1 << 10
		MB = 1 << 20
		GB = 1 << 30
	)
	switch {
	case n >= GB:
		return fmt.Sprintf("%.1fG", float64(n)/GB)
	case n >= MB:
		return fmt.Sprintf("%.1fM", float64(n)/MB)
	case n >= KB:
		return fmt.Sprintf("%.0fK", float64(n)/KB)
	default:
		return fmt.Sprintf("%dB", n)
	}
}
