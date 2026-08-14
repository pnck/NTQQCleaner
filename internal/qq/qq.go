// Package qq 是「逆向结论知识层」的抽象与调度（版本 dispatcher）。
//
// 变异轴：不同平台上的 QQ binary 不一样、不同版本的 QQ 逻辑不一样。
// 因此本包只定义接口与注册表，具体知识在 internal/qq/impl/* 实现包里：
//   - impl/nt       NT 架构（nt_qq_<32hex> 布局，macOS 已逆向；布局跨平台共享）
//   - impl/legacy   旧版（数字目录 + msg3.0.db，占位：保守不扫描）
//   - generic       兜底（无任何知识，fail-closed：拒绝扫描/清理）
//
// 上层（discovery/classify/rules/clean/app）只依赖 Knowledge 接口；
// 新增一个 QQ 平台/版本 = 新增一个 impl 包 + 注册 probe，上层零改动。
// 注册通过 internal/qqimpl 的副作用导入完成。
package qq

// Instance 是一个账号实例目录（NT: nt_qq_<32hex>；旧版: 数字目录）。
type Instance struct {
	DirName string
	Hash    string
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
