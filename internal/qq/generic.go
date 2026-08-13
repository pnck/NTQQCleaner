package qq

// genericKnowledge 是兜底实现：对未知布局无任何知识，fail-closed。
// 唯一保留的是与版本无关的安全底线（db 文件、nt_db 目录永不删除）。
type genericKnowledge struct{}

func (genericKnowledge) Name() string      { return "generic" }
func (genericKnowledge) ScanCapable() bool { return false }
func (genericKnowledge) InstanceDirs(root string) ([]Instance, error) {
	return nil, nil
}
func (genericKnowledge) Identify(root string, inst Instance) string { return "" }
func (genericKnowledge) BizDirs() []string                          { return nil }
func (genericKnowledge) SkipDirs() map[string]bool                  { return nil }
func (genericKnowledge) Classify(segments []string) (biz, category, sub, month string) {
	return "", "", "", ""
}
func (genericKnowledge) ParseFilename(base string) (md5, sizeTag, ext string, ok bool) {
	return "", "", "", false
}
func (genericKnowledge) IsMonthDir(name string) bool          { return false }
func (genericKnowledge) Whitelisted(rel string, g Gates) bool { return false }
func (genericKnowledge) StateDirs() []string                  { return []string{"nt_db"} }
func (genericKnowledge) DBSuffixes() []string {
	return []string{".db", ".db-wal", ".db-shm", ".db-first.material", ".db-last.material"}
}
