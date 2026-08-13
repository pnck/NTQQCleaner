package qq

// RootCandidates 返回各平台 QQ 数据根目录的默认候选（docs/README §3.1）。
// 每个平台一个 build-tagged 文件：QQ 安装路径布局属于逆向结论，
// 按平台拆分以便各自重新逆向时独立修改。

// rootCandidatesCommon 无平台时的兜底（空）。
