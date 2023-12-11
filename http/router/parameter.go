package router

// Parameter 参数
type Parameter struct {
	Key   string
	Value string
}

// Parameters 参数（复数）
type Parameters []Parameter

// Get 查询
func (p Parameters) Get(name string) (string, bool) {
	for _, entry := range p {
		if entry.Key == name {
			return entry.Value, true
		}
	}
	return "", false
}

// ByName 通过名字查询
func (p Parameters) ByName(name string) (value string) {
	value, _ = p.Get(name)
	return
}
