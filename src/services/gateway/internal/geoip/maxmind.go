package geoip

import (
	"net"

	"github.com/oschwald/geoip2-golang"
)

// MaxMind 使用本地 GeoLite2-City.mmdb 数据库查询 IP 信息。
// 使用 geoip2.Open 打开数据库，线程安全。
type MaxMind struct {
	db *geoip2.Reader
}

// NewMaxMind 打开指定路径的 MaxMind 数据库文件。
// 调用方负责在程序退出时调用 Close()。
func NewMaxMind(path string) (*MaxMind, error) {
	db, err := geoip2.Open(path)
	if err != nil {
		return nil, err
	}
	return &MaxMind{db: db}, nil
}

func (m *MaxMind) Close() {
	_ = m.db.Close()
}

func (m *MaxMind) LookupIP(ipStr string) Result {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return Result{Type: IPTypeUnknown}
	}

	record, err := m.db.City(ip)
	if err != nil {
		return Result{Type: IPTypeUnknown}
	}

	result := Result{
		Country: record.Country.IsoCode,
		Type:    classifyType(record),
	}

	if name, ok := record.City.Names["en"]; ok {
		result.City = name
	}

	return result
}

// classifyType 根据 GeoLite2-City 的 traits 字段判断 IP 类型。
// GeoLite2-City 只有 IsAnonymousProxy / IsSatelliteProvider，
// 更精细的 hosting/VPN/Tor 检测需要付费 GeoIP2-Enterprise 或 GeoIP2-Anonymous-IP 数据库。
func classifyType(record *geoip2.City) IPType {
	t := record.Traits

	if t.IsAnonymousProxy {
		return IPTypeTor // 匿名代理，包含 Tor 出口节点
	}

	return IPTypeUnknown
}
