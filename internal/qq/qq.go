// Package qq 是「逆向结论知识层」的抽象与调度（版本 dispatcher）。
//
// 变异轴：不同平台上的 QQ binary 不一样、不同版本的 QQ 逻辑不一样。
// 因此本包只定义接口与注册表，具体知识在 internal/qq/impl/* 实现包里：
//   - impl/nt       NT 架构（mac: nt_qq_<32hex>；Windows: <QQ号>/nt_qq/；
//     缓存布局跨平台共享，差异点见 docs/08）
//   - impl/legacy   旧版（数字目录 + msg3.0.db）：识别 + 只统计报告，
//     fail-closed 不扫描不清理（docs/08）
//   - generic       兜底（无任何知识，fail-closed：拒绝扫描/清理）
//
// 上层（discovery/classify/rules/clean/app）只依赖 Knowledge 接口；
// 新增一个 QQ 平台/版本 = 新增一个 impl 包 + 注册 probe，上层零改动。
// 注册通过 internal/qqimpl 的副作用导入完成。
package qq

import (
	"os"
	"path/filepath"
)

// Instance 是一个账号实例目录（NT mac: nt_qq_<32hex>；NT Windows:
// <QQ号> 明文目录；旧版: 数字目录）。
type Instance struct {
	DirName string
	Hash    string
	// NtRel 从实例目录到 nt_* 子目录组的相对路径（docs/08 §3.2）：
	//   - NT macOS: ""（nt_data/nt_db/nt_temp 直接位于实例目录下）
	//   - NT Windows: "nt_qq"（<QQ号>/nt_qq/{nt_data,nt_db,nt_temp}；
	//     实例目录顶层还残留旧版库，经 ResidueReporter 只统计报告）
	//   - 旧版: ""（无 nt_* 组，值无意义）
	NtRel string
}

// LegacyResidue 是账号实例里的旧版 QQ 数据残留（只统计，永不清理）。
// 典型：Windows NTQQ 账号目录顶层的 Msg3.0.db 等 38 个旧库——迁移到
// NT 后停更，QQ 官方清理器同样只统计不清理（逆向实测结论，docs/08 §2.2）。
type LegacyResidue struct {
	Path  string // 绝对路径（文件或目录）
	Size  int64  // 总大小（只 stat 汇总，绝不读文件内容）
	Count int    // 文件数
}

// ResidueReporter 是 Knowledge 的可选扩展：能报告旧版数据残留的实现
// （NT Windows、legacy）实现它；无残留的实现（NT macOS）返回 nil。
// 上层只用来展示统计；Residues 的输出绝不进入 clean.Request，
// 任何模式下都不可能据此删除。
type ResidueReporter interface {
	Residues(root string, inst Instance) ([]LegacyResidue, error)
}

// StatTree 只统计目录树的大小与文件数：只 stat 不读内容、不跟随
// 符号链接、不可读条目跳过。供 ResidueReporter 实现复用（docs/08 §3.5）。
func StatTree(p string) (size int64, count int, err error) {
	err = filepath.WalkDir(p, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		if info, err := d.Info(); err == nil {
			size += info.Size()
			count++
		}
		return nil
	})
	return size, count, err
}

// Gates 是白名单结构校验需要的分类门控（由 rules.Config 映射而来）。
// 门控是删除政策的一部分：CLI 按 config 保守默认，GUI 全部放开
// （选择权在用户筛选器）。
type Gates struct {
	CleanTemp          bool
	CleanThumb         bool
	CleanOri           bool
	CleanFile          bool
	CleanBaseEmoji     bool
	CleanMarketface    bool
	CleanPersonalEmoji bool
	CleanLog           bool
	CleanDatalineTmp   bool
	CleanAvatar        bool
}

// AllGates 全部门控打开：只问「结构是否在白名单内」，不问删除政策。
// CLI 扫描入库用它过滤（报告全部结构合法文件；分类门控留在清理时
// 按 config 判定）。
func AllGates() Gates {
	return Gates{
		CleanTemp: true, CleanThumb: true, CleanOri: true,
		CleanFile: true, CleanBaseEmoji: true,
		CleanMarketface: true, CleanPersonalEmoji: true,
		CleanLog: true, CleanDatalineTmp: true, CleanAvatar: true,
	}
}

// Knowledge 是一个「QQ 平台×版本族」的逆向结论实现。
type Knowledge interface {
	// Name 返回实现名（如 "nt" / "legacy" / "generic"）。
	Name() string
	// ScanCapable 是否具备扫描所需的全部知识；false 时上层拒绝扫描
	// 与清理（fail-closed）。
	ScanCapable() bool

	// 账号实例与识别
	InstanceDirs(root string) ([]Instance, error)
	Identify(root string, inst Instance) string

	// 布局与命名
	BizDirs() []string
	SkipDirs() map[string]bool
	Classify(segments []string) (biz, category, sub, month string)
	ParseFilename(base string) (md5, sizeTag, ext string, ok bool)
	IsMonthDir(name string) bool

	// 白名单/黑名单结构
	Whitelisted(rel string, g Gates) bool
	StateDirs() []string
	DBSuffixes() []string
}

// ---- 注册表：probe 链（版本 dispatcher）----

var probes []func(root string) Knowledge

// RegisterProbe 注册一个布局探测（注册顺序 = 优先级）。
func RegisterProbe(p func(root string) Knowledge) { probes = append(probes, p) }

// Detect 依据磁盘布局识别 QQ 平台×版本族。
// 两遍扫描：第一遍取「具备扫描能力」的匹配实现（数据根中可能同时残留
// 新旧布局，如数字目录+msg3.0.db 与 nt_qq_* 并存——旧布局会被识别但
// 不具扫描能力）；第二遍取任一已识别实现（如 legacy，供上层报告
// 具体的布局类型）；全部未命中返回 generic 兜底（fail-closed）。
// 优先级与注册顺序解耦。
func Detect(root string) Knowledge {
	var recognized Knowledge
	for _, p := range probes {
		if k := p(root); k != nil {
			if k.ScanCapable() {
				return k
			}
			if recognized == nil {
				recognized = k
			}
		}
	}
	if recognized != nil {
		return recognized
	}
	return genericKnowledge{}
}

// ---- 根路径候选注册（各 impl 的 per-OS 文件注册）----

var rootProviders []func() []string

// RegisterRoots 注册一组本 OS 的默认数据根候选。
func RegisterRoots(f func() []string) { rootProviders = append(rootProviders, f) }

// RootCandidates 汇总所有实现的当前 OS 根路径候选。
func RootCandidates() []string {
	var out []string
	for _, f := range rootProviders {
		out = append(out, f()...)
	}
	return out
}
