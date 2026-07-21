# XrayR4u 按用户限速对接指南

## 背景

K2Board v1.2.34 起，`GET /UniProxy/user` 和 `GET /Deepbwork/user` 返回的每个用户数据中新增 `speed_limit` 字段（单位 Mbps，0=不限速）。XrayR4u 当前使用全局 `c.SpeedLimit` 对所有用户统一限速，需要改为按用户读取。

## API 响应变更

### UniProxy (Vless) — `GET /UniProxy/user`

```json
// 之前
{"users": [{"id": 4, "uuid": "xxx", "email": "u@t.com"}]}

// 现在
{"users": [{"id": 4, "uuid": "xxx", "email": "u@t.com", "speed_limit": 100}]}
//                                                         ↑ 100 Mbps
```

### Deepbwork (V2ray/VMess) — `GET /Deepbwork/user`

```json
// 之前
{"data": [{"id": 4, "v2ray_user": {"uuid": "xxx", "email": "u@t.com", "alter_id": 0}}]}

// 现在
{"data": [{"id": 4, "v2ray_user": {"uuid": "xxx", "email": "u@t.com", "alter_id": 0, "speed_limit": 100}}]}
//                                                                                        ↑ 100 Mbps
```

- `speed_limit` 为 0 时表示不限速
- 单位统一为 **Mbps**

---

## XrayR4u 需要修改的文件

### 文件: `api/v2board/v2board.go`

#### 修改 1: `ParseUniProxyUserResponse`

```go
func (c *V2board) ParseUniProxyUserResponse(body []byte) ([]api.UserInfo, error) {
    j, err := simplejson.NewJson(body)
    if err != nil {
        return nil, err
    }
    users := j.Get("users")
    userArr, _ := users.Array()
    var userList []api.UserInfo
    for _, u := range userArr {
        userItem := users.GetIndex(len(userList))
        // ... existing id/uuid/email parsing ...

        // === 新增: 按用户限速 ===
        user.SpeedLimit = uint64(c.SpeedLimit * 1000000 / 8) // 默认用全局值
        if sl, ok := userItem.CheckGet("speed_limit"); ok {
            if v, err := sl.Int64(); err == nil && v > 0 {
                user.SpeedLimit = uint64(v * 1000000 / 8) // Mbps → Bps
            }
        }
        // === 新增结束 ===

        userList = append(userList, user)
    }
    return userList, nil
}
```

#### 修改 2: `ParseV2rayUserResponse`

```go
func (c *V2board) ParseV2rayUserResponse(body []byte) ([]api.UserInfo, error) {
    // ... existing parsing ...

    for _, u := range userArr {
        // ... existing id/v2ray_user parsing ...

        // === 新增: 按用户限速 ===
        user.SpeedLimit = uint64(c.SpeedLimit * 1000000 / 8)
        if vu, ok := userItem.CheckGet("v2ray_user"); ok {
            if sl, ok := vu.CheckGet("speed_limit"); ok {
                if v, err := sl.Int64(); err == nil && v > 0 {
                    user.SpeedLimit = uint64(v * 1000000 / 8)
                }
            }
        }
        // === 新增结束 ===

        userList = append(userList, user)
    }
    return userList, nil
}
```

---

## 验证

```bash
# 1. 在 K2Board 面板设置用户限速为 50 Mbps
# 2. 重启 XrayR4u
# 3. 查看用户的 Xray-core 出站配置
journalctl -u xrayr -f | grep -i speed
# 预期: SpeedLimit=6250000 (50 Mbps / 8 = 6.25 MBps = 6250000 Bps)
```

## 兼容性

- 如果面板不返回 `speed_limit` 字段（老版本），
  `CheckGet` 返回 `false` → 走 `user.SpeedLimit = c.SpeedLimit` 全局默认值
- 完全向后兼容

## 单位换算

```
面板存储: Mbps（整数，如 100）
XrayR4u 转换: Mbps × 1,000,000 / 8 = Bps
Xray-core 期望: Bps（uint64）
```
