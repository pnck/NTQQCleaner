// 全局「最后一次鼠标聚焦区域」：⌘/Ctrl+A 按它决定全选哪片区域，而不是
// 探测 document.activeElement——点击详情区的可选中文字会把焦点落在文字
// 上，closest 探测会把全选误判成「全选这段文字」。
export type FocusArea = "wall" | "tree-biz" | "tree-month" | null;

export const lastFocusArea: { cur: FocusArea } = { cur: null };

export function setFocusArea(a: FocusArea) {
  lastFocusArea.cur = a;
}
