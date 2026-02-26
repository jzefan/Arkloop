package geoip

// Noop 是未配置 GeoIP 数据库时的空实现，始终返回空 Result。
type Noop struct{}

func (Noop) LookupIP(_ string) Result {
	return Result{Type: IPTypeUnknown}
}
