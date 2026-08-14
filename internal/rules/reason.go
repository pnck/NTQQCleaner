package rules

import (
	"strings"

	"qqcleaner/internal/classify"
	"qqcleaner/internal/qq"
)

// Knowledge 是 rules 层对 qq 知识实现的窄接口（消费方定义）：
// reason 标签与白名单/黑名单判定只需要结构事实。
type Knowledge interface {
	Whitelisted(rel string, gates qq.Gates) bool
	StateDirs() []string
	DBSuffixes() []string
}

// Reason 返回短标签（如「缩略图」「重复出现」）；UI 悬浮 tooltip 用
// 标签词典解释含义，CLI manifest 直接展示标签。只做展示说明，不影响
// 可清性（docs/03 §4）。
//
// 关联情况分三种，互不混淆（用户反馈：Ori 曾因同名缩略图被误标
// 「重复出现」）：
//   - 重复出现：字节级内容相同（SHA-256）的其它副本存在（contentCount ≥ 2）
//   - 原图仍在：缩略图行，其同 md5 的原文件存在（可重建）
//   - 有缩略图：原文件行，其同 md5 的缩略图存在
func Reason(e classify.FileEntry, hasOri, hasThumb bool, contentCount int) string {
	var parts []string
	switch {
	case e.IsTemp:
		parts = append(parts, "下载中断残留")
	case strings.Contains(strings.ToLower(e.Category), "dataline/tmp"):
		parts = append(parts, "传输残留")
	case e.IsThumb:
		parts = append(parts, "缩略图")
	case strings.Contains(strings.ToLower(e.Category), "marketface"):
		parts = append(parts, "表情包")
	case strings.Contains(strings.ToLower(e.Category), "personal-emoji"):
		parts = append(parts, "个人表情")
	case e.Biz == "log" || e.Biz == "log-cache":
		parts = append(parts, "运行日志")
	case e.Biz == "avatar":
		parts = append(parts, "头像缓存")
	case strings.HasSuffix(strings.ToLower(e.Category), "/ori"):
		parts = append(parts, "原图/原文件")
	}
	if e.IsThumb && hasOri {
		parts = append(parts, "原图仍在")
	} else if strings.EqualFold(e.Sub, "Ori") && hasThumb {
		parts = append(parts, "有缩略图")
	}
	if contentCount >= 2 {
		parts = append(parts, "重复出现")
	}
	if len(parts) == 0 {
		return "缓存文件"
	}
	return strings.Join(parts, "；")
}
