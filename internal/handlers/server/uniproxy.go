package server

import (
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"

	"K2board/internal/middleware"
	"K2board/internal/models"
	"K2board/internal/queue"
	"K2board/internal/services"
	"K2board/internal/utils"
)

type UniProxyHandler struct {
	trafficSvc *services.TrafficService
}

func NewUniProxyHandler() *UniProxyHandler {
	return &UniProxyHandler{
		trafficSvc: services.NewTrafficService(),
	}
}

// === GetNodeInfo: GET /UniProxy/config ===
// Returns node configuration (VLESS / AnyTLS) for XrayR4u / xray-core 26.7.x.
//
// Compatibility: old nodes without PQ / XHTTP fields keep working.
// - min_client_ver defaults to "1.8.0" for REALITY
// - decryption defaults to "none" for VLESS
// - mldsa65_seed / show only when configured
// - dest / tls_settings.server_port = REALITY fallback (not listen port)
// - base_config / host / path / network_settings for CDN XHTTP (形态 B)

type UniProxyConfigResponse struct {
	ServerPort       int                      `json:"server_port"`
	Network          string                   `json:"network"`
	TLS              int                      `json:"tls"`
	Flow             string                   `json:"flow,omitempty"`
	Decryption       string                   `json:"decryption,omitempty"`
	Host             string                   `json:"host,omitempty"` // WS/XHTTP Host (CDN)
	Path             string                   `json:"path,omitempty"`
	TLSSettings      *UniProxyTLSSettings     `json:"tls_settings,omitempty"`
	NetworkSettings  *UniProxyNetworkSettings `json:"network_settings,omitempty"`
	BaseConfig       *UniProxyBaseConfig      `json:"base_config,omitempty"`
}

type UniProxyBaseConfig struct {
	PushInterval int `json:"push_interval"`
	PullInterval int `json:"pull_interval"`
}

type UniProxyNetworkSettings struct {
	Host        string `json:"host,omitempty"`
	Path        string `json:"path,omitempty"`
	Mode        string `json:"mode,omitempty"`         // xhttp: auto | packet-up | stream-up | …
	ServiceName string `json:"serviceName,omitempty"` // grpc
}

type UniProxyTLSSettings struct {
	ServerName   string `json:"server_name,omitempty"`
	Dest         string `json:"dest,omitempty"`
	ServerPort   string `json:"server_port,omitempty"` // dest port (string per UniProxy contract)
	PublicKey    string `json:"public_key,omitempty"`
	PrivateKey   string `json:"private_key,omitempty"`
	ShortID      string `json:"short_id,omitempty"`
	Fingerprint  string `json:"fingerprint,omitempty"`
	MinClientVer string `json:"min_client_ver,omitempty"`
	Mldsa65Seed  string `json:"mldsa65_seed,omitempty"`
	Show         *bool  `json:"show,omitempty"` // only set when true (debug)
}

func (h *UniProxyHandler) GetConfig(c *gin.Context) {
	node := middleware.GetCurrentNode(c)
	if node == nil {
		utils.InternalError(c, "node not found in context")
		return
	}
	c.JSON(200, buildUniProxyConfig(node))
}

// buildUniProxyConfig is pure for tests; keeps legacy nodes wire-compatible.
func buildUniProxyConfig(node *models.Node) UniProxyConfigResponse {
	network := strings.TrimSpace(node.Network)
	if network == "" {
		network = "tcp"
	}
	networkLower := strings.ToLower(network)

	// Normalize aliases used by some panels / clients
	switch networkLower {
	case "websocket":
		network = "ws"
		networkLower = "ws"
	case "httpupgrade":
		// keep as-is if node stores it
	}

	isAnyTLS := strings.EqualFold(node.NodeType, "anytls")
	isVLESS := strings.EqualFold(node.NodeType, "vless")

	response := UniProxyConfigResponse{
		ServerPort: node.Port,
		Network:    network,
		TLS:        node.TLS,
		BaseConfig: &UniProxyBaseConfig{
			PushInterval: 60,
			PullInterval: 60,
		},
	}

	// Flow: REALITY+Vision typical; CDN XHTTP should be empty (Vision off)
	if !isAnyTLS {
		flow := strings.TrimSpace(node.Flow)
		if isXHTTPFamily(networkLower) {
			// 形态 B: do not force Vision on CDN path
			response.Flow = flow // usually empty
		} else {
			response.Flow = flow
		}
	}

	// VLESS Encryption — empty DB → "none"; not used by AnyTLS
	if isVLESS {
		if d := utils.NormalizeVlessCrypto(node.VlessDecryption); d != "" {
			response.Decryption = d
		} else {
			response.Decryption = "none"
		}
	}

	// Transport host/path (WS / XHTTP / gRPC) — omit on pure tcp REALITY to match legacy shape
	hostHdr, pathVal := transportHostPath(node)
	if needsTransportMeta(networkLower) {
		if hostHdr != "" {
			response.Host = hostHdr
		}
		if pathVal != "" {
			response.Path = pathVal
		}
		ns := &UniProxyNetworkSettings{}
		if hostHdr != "" {
			ns.Host = hostHdr
		}
		if pathVal != "" {
			ns.Path = pathVal
		}
		if isXHTTPFamily(networkLower) {
			ns.Mode = "auto"
		}
		if networkLower == "grpc" && strings.TrimSpace(node.ServiceName) != "" {
			ns.ServiceName = strings.TrimSpace(node.ServiceName)
		}
		if ns.Host != "" || ns.Path != "" || ns.Mode != "" || ns.ServiceName != "" {
			response.NetworkSettings = ns
		}
	}

	if node.TLS < 1 {
		return response
	}

	sni := strings.TrimSpace(node.SNI)
	if sni == "" && hostHdr != "" {
		sni = hostHdr
	}
	tlsSettings := &UniProxyTLSSettings{
		ServerName: sni,
	}

	// REALITY only (tls=2). AnyTLS / plain TLS must not get REALITY key fields.
	if node.TLS == 2 && !isAnyTLS {
		// Default dest = SNI until reality.dest overrides
		destHost, destPort := utils.ParseDestHostPort("", sni)
		tlsSettings.Dest = destHost
		if destHost != "" {
			tlsSettings.ServerPort = destPort
		}

		if len(node.RealitySettings) > 0 {
			var rc struct {
				PublicKey         string `json:"public_key"`
				PrivateKey        string `json:"private_key"`
				ShortID           string `json:"short_id"`
				Fingerprint       string `json:"fingerprint"`
				Dest              string `json:"dest"`
				ServerName        string `json:"server_name"`
				MinClientVer      string `json:"min_client_ver"`
				MinClientVerCamel string `json:"minClientVer"`
				Mldsa65Seed       string `json:"mldsa65_seed"`
				Mldsa65SeedCamel  string `json:"mldsa65Seed"`
				Show              bool   `json:"show"`
			}
			if err := json.Unmarshal(node.RealitySettings, &rc); err == nil {
				if rc.PublicKey != "" {
					tlsSettings.PublicKey = rc.PublicKey
				}
				if rc.PrivateKey != "" {
					tlsSettings.PrivateKey = rc.PrivateKey
				}
				if rc.ShortID != "" {
					tlsSettings.ShortID = rc.ShortID
				}
				if rc.Fingerprint != "" {
					tlsSettings.Fingerprint = rc.Fingerprint
				}
				if sn := strings.TrimSpace(rc.ServerName); sn != "" {
					tlsSettings.ServerName = sn
					sni = sn
				}

				dh, dp := utils.ParseDestHostPort(rc.Dest, sni)
				tlsSettings.Dest = dh
				tlsSettings.ServerPort = dp

				mcv := strings.TrimSpace(rc.MinClientVer)
				if mcv == "" {
					mcv = strings.TrimSpace(rc.MinClientVerCamel)
				}
				if mcv == "" {
					mcv = "1.8.0"
				}
				tlsSettings.MinClientVer = mcv

				seed := strings.TrimSpace(rc.Mldsa65Seed)
				if seed == "" {
					seed = strings.TrimSpace(rc.Mldsa65SeedCamel)
				}
				if seed != "" {
					tlsSettings.Mldsa65Seed = seed
				}

				if rc.Show {
					t := true
					tlsSettings.Show = &t
				}
			}
		} else {
			tlsSettings.MinClientVer = "1.8.0"
		}
	}

	response.TLSSettings = tlsSettings
	return response
}

func isXHTTPFamily(network string) bool {
	switch network {
	case "xhttp", "splithttp":
		return true
	default:
		return false
	}
}

func needsTransportMeta(network string) bool {
	switch network {
	case "ws", "websocket", "xhttp", "splithttp", "httpupgrade", "grpc", "h2", "http":
		return true
	default:
		return false
	}
}

// transportHostPath: Host header / path for WS·XHTTP·gRPC.
// Prefer node.Host (CDN domain), fall back to SNI. Path defaults for xhttp/ws only when empty.
func transportHostPath(node *models.Node) (host, path string) {
	host = strings.TrimSpace(node.Host)
	if host == "" {
		host = strings.TrimSpace(node.SNI)
	}
	path = strings.TrimSpace(node.Path)
	net := strings.ToLower(strings.TrimSpace(node.Network))
	if path == "" && (net == "ws" || net == "websocket" || isXHTTPFamily(net) || net == "httpupgrade") {
		path = "/"
	}
	return host, path
}

// === GetUserList: GET /UniProxy/user ===

type UniProxyUserResponse struct {
	Users []UniProxyUser `json:"users"`
}

type UniProxyUser struct {
	ID          uint   `json:"id"`
	UUID        string `json:"uuid"`
	Email       string `json:"email"`
	SpeedLimit  int64  `json:"speed_limit"`
	DeviceLimit int    `json:"device_limit"`
}

func (h *UniProxyHandler) GetUser(c *gin.Context) {
	node := middleware.GetCurrentNode(c)
	if node == nil {
		utils.InternalError(c, "node not found in context")
		return
	}
	users, err := h.trafficSvc.GetActiveUsersByGroup(node.ID, node.GroupIDs)
	if err != nil {
		utils.InternalError(c, "failed to fetch users")
		return
	}

	response := UniProxyUserResponse{Users: []UniProxyUser{}}
	for _, user := range users {
		response.Users = append(response.Users, UniProxyUser{
			ID:          user.ID,
			UUID:        user.UUID,
			Email:       user.Email,
			SpeedLimit:  user.SpeedLimit,
			DeviceLimit: user.DeviceLimit,
		})
	}

	c.JSON(200, response)
}

// === ReportUserTraffic: POST /UniProxy/push ===

func (h *UniProxyHandler) PushTraffic(c *gin.Context) {
	node := middleware.GetCurrentNode(c)
	if node == nil {
		utils.InternalError(c, "node not found")
		return
	}

	var trafficMap map[uint][]int64
	if err := c.ShouldBindJSON(&trafficMap); err != nil {
		utils.BadRequest(c, "invalid request body")
		return
	}

	validUsers := ValidateTrafficUsers(node.ID, node.GroupIDs, trafficMap)
	for userID, stats := range trafficMap {
		if !validUsers[userID] {
			continue
		}
		if len(stats) < 2 {
			continue
		}
		if stats[0] < 0 || stats[1] < 0 || stats[0] > 1<<40 || stats[1] > 1<<40 {
			continue
		}
		queue.DefaultStore.Add(userID, node.ID, stats[0], stats[1])
	}

	c.JSON(200, gin.H{"status": "ok"})
}

// === ReportOnlineUsers: POST /UniProxy/alive ===

func (h *UniProxyHandler) AliveUsers(c *gin.Context) {
	node := middleware.GetCurrentNode(c)
	if node == nil {
		utils.InternalError(c, "node not found")
		return
	}

	var onlineMap map[uint][]string
	if err := c.ShouldBindJSON(&onlineMap); err != nil {
		utils.BadRequest(c, "invalid request body")
		return
	}

	// Same group binding as PushTraffic — compromised node cannot mark arbitrary users online.
	if err := h.trafficSvc.RecordNodeOnline(node.ID, node.GroupIDs, onlineMap); err != nil {
		utils.InternalError(c, "failed to record online")
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}
