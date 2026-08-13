// reason 短标签 → 含义词典（悬浮 tooltip 用）。
// 标签由 Go 侧 rules.Reason 生成，务必与此表同步。

export const REASON_GLOSSARY: Record<string, string> = {
  "下载中断残留": "未完成的下载临时文件；QQ 自己也会优先清理这类文件",
  "缩略图": "小尺寸预览图；QQ 查看时可以从原图随时重新生成",
  "表情包": "从表情商店下载的整套表情资源",
  "个人表情": "自己制作或收藏的表情",
  "原图/原文件": "内容本身；删除后无法还原（除非设置了备份目录）",
  "原图仍在": "同 md5 的原图仍在缓存中，删掉这个缩略图零损失（可重建）",
  "有缩略图": "这个原文件有对应的缩略图缓存（同名 md5）；缩略图被清理不影响原文件",
  "重复出现":
    "内容完全相同的其它副本（按真实内容哈希识别，QQ 只按目录去重：同一文件在不同月份/目录会以不同名字各存一份）；删除其中一份不影响其它副本",
  "缓存文件": "常规缓存文件",
};

export interface ReasonPart {
  label: string;
  explain: string;
}

export function explainReason(reason: string): ReasonPart[] {
  return reason.split("；").map((s) => {
    const label = s.trim();
    return { label, explain: REASON_GLOSSARY[label] ?? "缓存文件的相关信息" };
  });
}
