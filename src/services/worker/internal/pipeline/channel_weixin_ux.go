package pipeline

// WeixinChannelUX weixin 渠道 UX 配置（预留扩展）。
type WeixinChannelUX struct {
}

// ParseWeixinChannelUX 从 channel.config_json 解析 weixin UX 配置。
func ParseWeixinChannelUX(configJSON []byte) WeixinChannelUX {
	return WeixinChannelUX{}
}
