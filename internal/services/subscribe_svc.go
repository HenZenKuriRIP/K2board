package services

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"K2board/internal/database"
	"K2board/internal/models"
	"K2board/internal/utils"
)

type SubscribeService struct{}

func NewSubscribeService() *SubscribeService {
	return &SubscribeService{}
}

type SubResult struct {
	Content      string
	UploadUsed   int64
	DownloadUsed int64
	TotalTraffic int64
	ExpireAt     int64
}

// GenerateSubscription generates the subscription content for a user in the requested format.
// Supported flags: clash (FlClash/Clash Meta YAML, recommended default), surge, shadowrocket;
// v2ray kept for legacy clients only.
func (s *SubscribeService) GenerateSubscription(token, flag string) (*SubResult, error) {
	var user models.User
	if err := database.DB.Where("token = ?", token).First(&user).Error; err != nil {
		return nil, fmt.Errorf("invalid token")
	}
	// enable = ban; expire_at = service window (see account_access.go)
	if IsAccountBanned(&user) {
		return nil, fmt.Errorf("user disabled")
	}
	if IsServiceExpired(&user, time.Now().Unix()) {
		return nil, fmt.Errorf("user expired")
	}
	if IsTrafficExceeded(&user) {
		return nil, fmt.Errorf("traffic limit exceeded")
	}

	// Strict visibility + fair first-node rotation (per plan/group, Redis INCR + user salt)
	nodes, err := NodesForSubscribe(&user)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no available nodes")
	}

	flag = strings.ToLower(strings.TrimSpace(flag))
	var content string
	switch flag {
	case "surge":
		content = s.buildSurge(&user, nodes)
	case "shadowrocket", "rocket", "sr":
		content = s.buildShadowrocket(&user, nodes)
	case "v2ray", "v2rayn", "base64":
		content = s.buildV2Ray(&user, nodes)
	default:
		// clash / meta / mihomo / flclash / empty
		content = s.buildClash(&user, nodes)
	}

	return &SubResult{
		Content:      content,
		UploadUsed:   0,
		DownloadUsed: user.TrafficUsed,
		TotalTraffic: user.TrafficLimit,
		ExpireAt:     user.ExpireAt,
	}, nil
}

// GetAllUserSubscriptions returns subscribe URL info for all users (admin use).
func (s *SubscribeService) GetAllUserSubscriptions(page, pageSize int, search string) ([]SubUserInfo, int64, error) {
	var users []models.User
	var total int64

	query := database.DB.Model(&models.User{})
	if search != "" {
		query = query.Where("email LIKE ?", "%"+search+"%")
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("id DESC").Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	result := make([]SubUserInfo, len(users))
	for i, u := range users {
		result[i] = SubUserInfo{
			ID:           u.ID,
			Email:        u.Email,
			Token:        u.Token,
			UUID:         u.UUID,
			PlanID:       u.PlanID,
			GroupID:      u.GroupID,
			TrafficUsed:  u.TrafficUsed,
			TrafficLimit: u.TrafficLimit,
			Enable:       u.Enable,
			ExpireAt:     u.ExpireAt,
		}
	}
	return result, total, nil
}

type SubUserInfo struct {
	ID           uint   `json:"id"`
	Email        string `json:"email"`
	Token        string `json:"token"`
	UUID         string `json:"uuid"`
	PlanID       uint   `json:"plan_id"`
	GroupID      uint   `json:"group_id"`
	TrafficUsed  int64  `json:"traffic_used"`
	TrafficLimit int64  `json:"traffic_limit"`
	Enable       bool   `json:"enable"`
	ExpireAt     int64  `json:"expire_at"`
}

// ======================== V2Ray format ========================

func (s *SubscribeService) buildV2Ray(user *models.User, nodes []models.Node) string {
	var links []string
	for _, node := range nodes {
		link := s.buildNodeLink(user, &node)
		if link != "" {
			links = append(links, link)
		}
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n")))
}

func (s *SubscribeService) buildNodeLink(user *models.User, node *models.Node) string {
	switch node.NodeType {
	case "v2ray":
		return s.vmessLink(user, node)
	case "vless":
		return s.vlessLink(user, node)
	case "anytls":
		return s.anytlsLink(user, node)
	case "trojan":
		return s.trojanLink(user, node)
	case "shadowsocks":
		return s.ssLink(user, node)
	}
	return ""
}

func (s *SubscribeService) vmessLink(user *models.User, node *models.Node) string {
	host := node.Host
	if host == "" {
		host = node.SNI
	}
	path := node.Path
	if path == "" {
		path = "/"
	}
	net := node.Network
	if net == "" {
		net = "ws"
	}
	tls := ""
	if node.TLS == 1 || node.TLS == 2 {
		tls = "tls"
	}
	cfg := fmt.Sprintf(`{"v":"2","ps":"%s","add":"%s","port":"%d","id":"%s","aid":"0","net":"%s","type":"none","host":"%s","path":"%s","tls":"%s"}`,
		node.Name, host, node.Port, user.UUID, net, host, path, tls)
	return "vmess://" + base64.StdEncoding.EncodeToString([]byte(cfg))
}

func (s *SubscribeService) vlessLink(user *models.User, node *models.Node) string {
	host := strings.TrimSpace(node.Host)
	if host == "" {
		host = strings.TrimSpace(node.SNI)
	}
	security := "none"
	if node.TLS == 1 {
		security = "tls"
	} else if node.TLS == 2 {
		security = "reality"
	}
	net := strings.ToLower(strings.TrimSpace(node.Network))
	if net == "" {
		if node.TLS == 2 {
			net = "tcp"
		} else {
			net = "ws"
		}
	}
	if net == "websocket" {
		net = "ws"
	}

	q := url.Values{}
	q.Set("type", net)
	q.Set("security", security)

	// path / host header for WS · XHTTP · SplitHTTP
	if needsSubTransportPath(net) {
		path := strings.TrimSpace(node.Path)
		if path == "" {
			path = "/"
		}
		q.Set("path", path)
		// host query = CDN/SNI host header when useful
		hdr := strings.TrimSpace(node.Host)
		if hdr == "" {
			hdr = strings.TrimSpace(node.SNI)
		}
		if hdr != "" {
			q.Set("host", hdr)
		}
	}
	if net == "grpc" && strings.TrimSpace(node.ServiceName) != "" {
		q.Set("serviceName", strings.TrimSpace(node.ServiceName))
	}

	// Vision flow: REALITY 常用；CDN XHTTP 一般不带
	if flow := strings.TrimSpace(node.Flow); flow != "" && !isXHTTPNet(net) {
		q.Set("flow", flow)
	}

	if node.TLS == 2 && len(node.RealitySettings) > 0 {
		var rc struct {
			PublicKey     string `json:"public_key"`
			ShortID       string `json:"short_id"`
			Fingerprint   string `json:"fingerprint"`
			Mldsa65Verify string `json:"mldsa65_verify"`
		}
		if err := json.Unmarshal(node.RealitySettings, &rc); err == nil {
			if rc.PublicKey != "" {
				q.Set("pbk", rc.PublicKey)
			}
			if rc.ShortID != "" {
				q.Set("sid", rc.ShortID)
			}
			fp := rc.Fingerprint
			if fp == "" {
				fp = "chrome"
			}
			q.Set("fp", fp)
			if v := strings.TrimSpace(rc.Mldsa65Verify); v != "" {
				q.Set("mldsa65Verify", v)
			}
		}
	} else if node.TLS == 1 {
		// TLS CDN / plain: fingerprint optional chrome for client friendliness
		q.Set("fp", "chrome")
	}

	if enc := utils.NormalizeVlessCrypto(node.VlessEncryption); enc != "" {
		q.Set("encryption", enc)
	}
	if sni := strings.TrimSpace(node.SNI); sni != "" {
		q.Set("sni", sni)
	}

	return fmt.Sprintf("vless://%s@%s:%d?%s#%s",
		user.UUID, host, node.Port, q.Encode(), url.QueryEscape(node.Name))
}

func isXHTTPNet(net string) bool {
	return net == "xhttp" || net == "splithttp"
}

func needsSubTransportPath(net string) bool {
	switch net {
	case "ws", "xhttp", "splithttp", "httpupgrade", "http", "h2":
		return true
	default:
		return false
	}
}

func (s *SubscribeService) anytlsLink(user *models.User, node *models.Node) string {
	host := node.Host
	if host == "" {
		host = node.SNI
	}
	path := node.Path
	if path == "" {
		path = "/"
	}
	net := node.Network
	if net == "" {
		net = "tcp"
	}
	extra := ""
	if node.SNI != "" {
		extra += "&sni=" + node.SNI
	}
	return fmt.Sprintf("anytls://%s@%s:%d?type=%s&security=tls&path=%s%s#%s",
		user.UUID, host, node.Port, net, path, extra, url.QueryEscape(node.Name))
}

func (s *SubscribeService) trojanLink(user *models.User, node *models.Node) string {
	host := node.Host
	if host == "" {
		host = node.SNI
	}
	sni := node.SNI
	if sni == "" {
		sni = host
	}
	return fmt.Sprintf("trojan://%s@%s:%d?sni=%s#%s",
		user.UUID, host, node.Port, sni, url.QueryEscape(node.Name))
}

func (s *SubscribeService) ssLink(user *models.User, node *models.Node) string {
	host := node.Host
	if host == "" {
		host = node.SNI
	}
	method := node.Cipher
	if method == "" {
		method = "aes-256-gcm"
	}
	// SIP002: ss://base64(method:password)@host:port#name
	userInfo := base64.RawURLEncoding.EncodeToString([]byte(method + ":" + user.UUID))
	return fmt.Sprintf("ss://%s@%s:%d#%s", userInfo, host, node.Port, url.QueryEscape(node.Name))
}

// ======================== Clash Meta / FlClash / mihomo format ========================
// Aligned with common CN user + mihomo best practices (2024–2026):
// - rule mode + fake-ip DNS + proxy-server-nameserver (resolve node domains via CN DNS)
// - sniffer for accurate domain routing
// - geodata-mode + auto-update via jsDelivr (avoids raw GitHub when possible)
// - rule order: LAN → ads → proxy lists → CN direct → MATCH proxy
// - no external rule-providers in payload (import must work offline / without GH)

func (s *SubscribeService) buildClash(user *models.User, nodes []models.Node) string {
	var proxies []map[string]any
	var names []string

	for _, node := range nodes {
		proxy := s.clashProxy(user, &node)
		if proxy != nil {
			proxies = append(proxies, proxy)
			names = append(names, node.Name)
		}
	}

	// 主选择组：策略 + 全部节点 + 直连
	selectProxies := make([]string, 0, len(names)+5)
	selectProxies = append(selectProxies, "♻️ 自动选择", "🆘 故障转移")
	selectProxies = append(selectProxies, names...)
	selectProxies = append(selectProxies, "DIRECT")

	autoProxies := names
	if len(autoProxies) == 0 {
		autoProxies = []string{"DIRECT"}
	}

	// 测速 URL：gstatic 在多数环境下可用
	const hcURL = "https://www.gstatic.com/generate_204"

	config := map[string]any{
		// —— 基础 ——
		"mixed-port":                7890,
		"allow-lan":                 true,
		"bind-address":              "*",
		"mode":                      "rule",
		"log-level":                 "info",
		"ipv6":                      false, // 国内家宽 IPv6 兼容问题较多，默认关
		"unified-delay":             true,
		"tcp-concurrent":            true,
		"find-process-mode":         "strict",
		"global-client-fingerprint": "chrome",
		"keep-alive-interval":       30,
		"external-controller":       "127.0.0.1:9090",

		// Geo 数据：客户端内置 + 可走 CDN 自动更新（无需 rule-providers 依赖 GitHub raw）
		"geodata-mode":        true,
		"geo-auto-update":     true,
		"geo-update-interval": 24,
		"geox-url": map[string]any{
			"geoip":   "https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.dat",
			"geosite": "https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geosite.dat",
			"mmdb":    "https://cdn.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/country.mmdb",
		},

		"profile": map[string]any{
			"store-selected": true,
			"store-fake-ip":  true,
		},

		// 嗅探：提升域名规则命中率（FlClash / mihomo 标配）
		"sniffer": map[string]any{
			"enable":              true,
			"force-dns-mapping":   true,
			"parse-pure-ip":       true,
			"override-destination": true,
			"sniff": map[string]any{
				"HTTP": map[string]any{
					"ports": []any{80, "8080-8880"},
				},
				"TLS": map[string]any{
					"ports": []any{443, 8443},
				},
			},
			"skip-domain": []string{
				"Mijia Cloud",
				"+.push.apple.com",
			},
		},

		// —— DNS：fake-ip + 国内 DoH + 污染回落（社区主流写法）——
		"dns": map[string]any{
			"enable":       true,
			"ipv6":         false,
			"prefer-h3":    false,
			"use-hosts":    true,
			"enhanced-mode": "fake-ip",
			"fake-ip-range": "198.18.0.1/16",
			"fake-ip-filter": []string{
				"*.lan",
				"*.local",
				"*.localhost",
				"+.local",
				"localhost.ptlogin2.qq.com",
				"+.stun.*.*",
				"+.stun.*.*.*",
				"+.stun.*.*.*.*",
				"lens.l.google.com",
				"*.srv.nintendo.net",
				"*.stun.playstation.net",
				"xbox.*.microsoft.com",
				"*.xboxlive.com",
				"*.battlenet.com.cn",
				"*.battlenet.com",
				"*.blzstatic.cn",
				"*.battle.net",
				"work.weixin.qq.com",
				"dns.msftncsi.com",
				"www.msftncsi.com",
				"www.msftconnecttest.com",
				"time.*.com",
				"ntp.*.com",
				"+.market.xiaomi.com",
			},
			// 引导解析（必须是 IP）
			"default-nameserver": []string{
				"223.5.5.5",
				"119.29.29.29",
			},
			// 默认走国内 DNS
			"nameserver": []string{
				"https://doh.pub/dns-query",
				"https://dns.alidns.com/dns-query",
				"223.5.5.5",
				"119.29.29.29",
			},
			// 非 CN 结果或污染时回落
			"fallback": []string{
				"https://1.1.1.1/dns-query",
				"https://dns.google/dns-query",
				"tls://8.8.4.4",
			},
			"fallback-filter": map[string]any{
				"geoip":      true,
				"geoip-code": "CN",
				"geosite":    []string{"gfw"},
				"ipcidr":     []string{"240.0.0.0/4", "0.0.0.0/32"},
			},
			// 节点域名解析用国内 DNS，避免节点域名被污染导致连不上
			"proxy-server-nameserver": []string{
				"https://doh.pub/dns-query",
				"https://dns.alidns.com/dns-query",
				"223.5.5.5",
			},
			"direct-nameserver": []string{
				"system",
				"223.5.5.5",
			},
		},

		"proxies": proxies,
		"proxy-groups": []map[string]any{
			{
				"name":    "🚀 节点选择",
				"type":    "select",
				"proxies": selectProxies,
			},
			{
				"name":      "♻️ 自动选择",
				"type":      "url-test",
				"proxies":   autoProxies,
				"url":       hcURL,
				"interval":  300,
				"tolerance": 50,
				"lazy":      true,
			},
			{
				"name":     "🆘 故障转移",
				"type":     "fallback",
				"proxies":  autoProxies,
				"url":      hcURL,
				"interval": 300,
				"lazy":     true,
			},
			{
				"name": "🌏 电报信息",
				"type": "select",
				"proxies": []string{
					"🚀 节点选择", "♻️ 自动选择", "🆘 故障转移", "DIRECT",
				},
			},
			{
				"name": "🤖 AI 平台",
				"type": "select",
				"proxies": []string{
					"🚀 节点选择", "♻️ 自动选择", "🆘 故障转移", "DIRECT",
				},
			},
			{
				"name": "📹 油管视频",
				"type": "select",
				"proxies": []string{
					"🚀 节点选择", "♻️ 自动选择", "DIRECT",
				},
			},
			{
				"name": "🎥 奈飞视频",
				"type": "select",
				"proxies": []string{
					"🚀 节点选择", "♻️ 自动选择", "DIRECT",
				},
			},
			{
				"name": "🌍 国外流量",
				"type": "select",
				"proxies": []string{
					"🚀 节点选择", "♻️ 自动选择", "🆘 故障转移", "DIRECT",
				},
			},
			{
				"name":    "🇨🇳 国内流量",
				"type":    "select",
				"proxies": []string{"DIRECT", "🚀 节点选择"},
			},
			{
				"name":    "🛑 广告拦截",
				"type":    "select",
				"proxies": []string{"REJECT", "DIRECT", "🚀 节点选择"},
			},
			{
				"name": "🐟 漏网之鱼",
				"type": "select",
				"proxies": []string{
					"🚀 节点选择", "♻️ 自动选择", "🆘 故障转移", "DIRECT",
				},
			},
		},

		// 规则顺序（国内机场/社区共识）：
		// 1 局域网  2 广告  3 需代理名单  4 国内直连  5 其余走代理
		"rules": []string{
			// 1) 局域网 / 私有
			"GEOSITE,private,DIRECT",
			"GEOIP,LAN,DIRECT,no-resolve",
			"GEOIP,private,DIRECT,no-resolve",
			"IP-CIDR,192.168.0.0/16,DIRECT,no-resolve",
			"IP-CIDR,10.0.0.0/8,DIRECT,no-resolve",
			"IP-CIDR,172.16.0.0/12,DIRECT,no-resolve",
			"IP-CIDR,127.0.0.0/8,DIRECT,no-resolve",
			"IP-CIDR,100.64.0.0/10,DIRECT,no-resolve",
			"IP-CIDR,224.0.0.0/4,DIRECT,no-resolve",
			"IP-CIDR6,fe80::/10,DIRECT,no-resolve",
			"IP-CIDR6,::1/128,DIRECT,no-resolve",

			// 2) 广告（可在分组里改成 DIRECT 放行）
			"GEOSITE,category-ads-all,🛑 广告拦截",

			// 3) 明确走代理（放在 CN 直连之前，避免被 cn 列表误伤）
			"GEOSITE,telegram,🌏 电报信息",
			"GEOSITE,openai,🤖 AI 平台",
			"GEOSITE,youtube,📹 油管视频",
			"GEOSITE,netflix,🎥 奈飞视频",
			"GEOSITE,spotify,🌍 国外流量",
			"GEOSITE,twitter,🌍 国外流量",
			"GEOSITE,facebook,🌍 国外流量",
			"GEOSITE,instagram,🌍 国外流量",
			"GEOSITE,discord,🌍 国外流量",
			"GEOSITE,github,🌍 国外流量",
			"GEOSITE,google,🌍 国外流量",
			"GEOSITE,gfw,🌍 国外流量",
			"GEOSITE,greatfire,🌍 国外流量",
			"GEOSITE,geolocation-!cn,🌍 国外流量",

			// 4) 国内直连（含 CDN / 运营商）
			"GEOSITE,cn,🇨🇳 国内流量",
			"GEOSITE,apple-cn,🇨🇳 国内流量",
			"GEOSITE,microsoft@cn,🇨🇳 国内流量",
			"GEOIP,CN,🇨🇳 国内流量",

			// 5) 其余默认代理（海外网站兜底）
			"MATCH,🐟 漏网之鱼",
		},
	}

	out, _ := yaml.Marshal(config)
	return string(out)
}

func (s *SubscribeService) clashProxy(user *models.User, node *models.Node) map[string]any {
	host := node.Host
	if host == "" {
		host = node.SNI
	}
	path := node.Path
	if path == "" {
		path = "/"
	}
	sni := node.SNI
	if sni == "" {
		sni = host
	}
	net := node.Network
	if net == "" {
		net = "ws"
	}

	switch node.NodeType {
	case "v2ray":
		p := map[string]any{
			"name":     node.Name,
			"type":     "vmess",
			"server":   host,
			"port":     node.Port,
			"uuid":     user.UUID,
			"alterId":  0,
			"cipher":   "auto",
			"network":  net,
		}
		if node.TLS >= 1 {
			p["tls"] = true
			p["servername"] = sni
		}
		if net == "ws" {
			p["ws-opts"] = map[string]any{
				"path": path,
				"headers": map[string]string{"Host": host},
			}
		} else if net == "grpc" {
			p["grpc-opts"] = map[string]any{"grpc-service-name": node.ServiceName}
		}
		return p

	case "vless":
		netLower := strings.ToLower(net)
		if netLower == "websocket" {
			netLower = "ws"
			net = "ws"
		}
		if netLower == "" {
			if node.TLS == 2 {
				net = "tcp"
				netLower = "tcp"
			} else {
				net = "ws"
				netLower = "ws"
			}
		}
		p := map[string]any{
			"name":               node.Name,
			"type":               "vless",
			"server":             host,
			"port":               node.Port,
			"uuid":               user.UUID,
			"network":            net,
			"udp":                true,
			"client-fingerprint": "chrome",
		}
		// Vision flow only when set and not XHTTP CDN
		if flow := strings.TrimSpace(node.Flow); flow != "" && !isXHTTPNet(netLower) {
			p["flow"] = flow
		}
		if node.TLS == 2 {
			p["tls"] = true
			fp := s.realityField(node, "fingerprint")
			if fp == "" {
				fp = "chrome"
			}
			p["client-fingerprint"] = fp
			ro := map[string]any{
				"public-key": s.realityField(node, "public_key"),
				"short-id":   s.realityField(node, "short_id"),
			}
			if v := strings.TrimSpace(s.realityField(node, "mldsa65_verify")); v != "" {
				ro["mldsa65-public-key"] = v
			}
			p["reality-opts"] = ro
			p["servername"] = sni
		} else if node.TLS == 1 {
			p["tls"] = true
			p["servername"] = sni
			p["client-fingerprint"] = "chrome"
		}
		if enc := utils.NormalizeVlessCrypto(node.VlessEncryption); enc != "" {
			p["encryption"] = enc
		}
		switch netLower {
		case "ws":
			p["ws-opts"] = map[string]any{
				"path":    path,
				"headers": map[string]string{"Host": host},
			}
		case "xhttp", "splithttp":
			// mihomo / FlClash: xhttp-opts
			p["xhttp-opts"] = map[string]any{
				"path": path,
				"host": host,
				"mode": "auto",
			}
		case "grpc":
			p["grpc-opts"] = map[string]any{"grpc-service-name": node.ServiceName}
		}
		return p

	case "anytls":
		// mihomo / FlClash anytls: password = user uuid
		p := map[string]any{
			"name":     node.Name,
			"type":     "anytls",
			"server":   host,
			"port":     node.Port,
			"password": user.UUID,
			"udp":      true,
			"sni":      sni,
		}
		if node.SNI != "" {
			p["sni"] = node.SNI
		}
		return p

	case "trojan":
		p := map[string]any{
			"name":       node.Name,
			"type":       "trojan",
			"server":     host,
			"port":       node.Port,
			"password":   user.UUID,
			"sni":        sni,
			"skip-cert-verify": false,
		}
		if net == "ws" {
			p["network"] = "ws"
			p["ws-opts"] = map[string]any{
				"path": path,
				"headers": map[string]string{"Host": host},
			}
		} else if net == "grpc" {
			p["network"] = "grpc"
			p["grpc-opts"] = map[string]any{"grpc-service-name": node.ServiceName}
		}
		return p

	case "shadowsocks":
		method := node.Cipher
		if method == "" {
			method = "aes-256-gcm"
		}
		return map[string]any{
			"name":          node.Name,
			"type":          "ss",
			"server":        host,
			"port":          node.Port,
			"cipher":        method,
			"password":      user.UUID,
			"plugin":        "",
			"plugin-opts":   map[string]any{},
		}
	}
	return nil
}

func (s *SubscribeService) realityField(node *models.Node, key string) string {
	if len(node.RealitySettings) == 0 {
		return ""
	}
	var rc map[string]any
	if err := json.Unmarshal(node.RealitySettings, &rc); err != nil {
		return ""
	}
	v, _ := rc[key].(string)
	return v
}

// ======================== Surge format ========================

func (s *SubscribeService) buildSurge(user *models.User, nodes []models.Node) string {
	var lines []string
	for _, node := range nodes {
		line := s.surgeLine(user, &node)
		if line != "" {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func (s *SubscribeService) surgeLine(user *models.User, node *models.Node) string {
	host := node.Host
	if host == "" {
		host = node.SNI
	}
	sni := node.SNI
	if sni == "" {
		sni = host
	}
	path := node.Path
	if path == "" {
		path = "/"
	}
	net := node.Network
	if net == "" {
		net = "ws"
	}

	switch node.NodeType {
	case "v2ray":
		tlsFlag := "false"
		if node.TLS >= 1 {
			tlsFlag = "true"
		}
		return fmt.Sprintf("%s = vmess, %s, %d, username=%s, ws=%s, tls=%s, ws-path=%s, sni=%s",
			node.Name, host, node.Port, user.UUID, "true", tlsFlag, path, sni)

	case "vless":
		// Surge 5+ VLESS (limited); emit best-effort line
		tlsFlag := "false"
		if node.TLS >= 1 {
			tlsFlag = "true"
		}
		return fmt.Sprintf("%s = vless, %s, %d, username=%s, tls=%s, sni=%s",
			node.Name, host, node.Port, user.UUID, tlsFlag, sni)

	case "trojan":
		return fmt.Sprintf("%s = trojan, %s, %d, password=%s, sni=%s",
			node.Name, host, node.Port, user.UUID, sni)

	case "anytls":
		return fmt.Sprintf("%s = anytls, %s, %d, password=%s, sni=%s, tls=true",
			node.Name, host, node.Port, user.UUID, sni)

	case "shadowsocks":
		method := node.Cipher
		if method == "" {
			method = "aes-256-gcm"
		}
		return fmt.Sprintf("%s = ss, %s, %d, encrypt-method=%s, password=%s",
			node.Name, host, node.Port, method, user.UUID)
	}
	return ""
}

// ======================== Shadowrocket format ========================

func (s *SubscribeService) buildShadowrocket(user *models.User, nodes []models.Node) string {
	var links []string
	for _, node := range nodes {
		link := s.buildNodeLink(user, &node)
		if link != "" {
			links = append(links, link)
		}
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n")))
}
