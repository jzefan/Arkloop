package geoip

// IPType 表示 IP 地址的网络类型。
type IPType string

const (
	IPTypeUnknown    IPType = "unknown"
	IPTypeResidential IPType = "residential"
	IPTypeHosting    IPType = "hosting"    // 数据中心 / 云主机
	IPTypeTor        IPType = "tor"
)

// Result 包含一次 IP 查询的地理信息。
type Result struct {
	Country string // ISO 3166-1 alpha-2，如 "CN"
	City    string
	ASN     uint   // 自治系统号
	OrgName string // ASN 归属机构名称
	Type    IPType
}

// Lookup 按 IP 查询地理信息。
type Lookup interface {
	// LookupIP 返回 IP 的地理信息。IP 格式为 string（支持 IPv4/IPv6）。
	// 查询失败时返回空 Result，不返回 error（调用方不应因 GeoIP 失败而阻断请求）。
	LookupIP(ip string) Result
}
