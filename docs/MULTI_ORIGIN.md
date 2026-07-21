# 多域名用户入口（影子注册站）部署说明

目标：**多个根域名只部署用户前端**，API / 订阅 / 支付 / 管理 / 数据库仍在 **www 主域**。  
客户端订阅地址**永不改变**。

## 架构

| 组件 | 位置 | 域名 |
|------|------|------|
| 用户 SPA (`web_user`) | 静态服务器 / CDN，可多份 | 任意影子域 `a.com` / `b.net` … |
| API + 管理端 + 订阅 + 支付 + PG + Redis | 面板主服务器 | **`https://www.主域`（钉死）** |

订阅链接始终为：

```text
{subscribe_url 或 site_url}/api/v1/client/subscribe?token=...
```

与用户从哪个影子站注册无关。

## 后台配置（最小清单）

1. **站点 URL** = `https://www.主域`（支付 notify 等，钉死）
2. **订阅域名** = `https://www.主域`（或与现网客户端一致的 origin，钉死）
3. **允许的用户端域名** = 每个影子入口一行，例如：

```text
https://user.example.com
https://shadow-a.com
https://portal-b.net
```

系统会自动把「站点 URL」「订阅域名」并入生效列表，无需重复填写。

保存后：

- CORS 允许这些 Origin 跨域调用 `https://www.主域/api/v1/*`
- 支付 `return_url` 允许跳回这些域名上的订单结果页

## 一键安装（影子机）

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/HenZenKuriRIP/K2board/main/deploy/install-user-portal.sh) \
  user.example.com https://www.example.com
```

- 脚本：[`deploy/install-user-portal.sh`](../deploy/install-user-portal.sh)  
- Nginx 模板：[`deploy/nginx-user-portal.conf`](../deploy/nginx-user-portal.conf)  
- 产物：`k2board-user-dist.tar.gz`（Release 资源）

## 用户前端 `API_BASE`

影子站浏览器必须把 API 指到 www，有两种方式（任选）：

### A. 构建时（推荐统一产物）

```bash
cd web_user
VITE_API_BASE=https://www.example.com npm run build
# 将 dist/ 部署到所有影子域
```

### B. 运行时（同一 dist，各站改 config）— 安装脚本默认方式

1. 构建时可不设 `VITE_API_BASE`（Release 默认如此）  
2. 各影子站 `dist/config.js`（`install-user-portal.sh` 自动写入）：

```js
window.__K2_API_BASE__ = 'https://www.example.com';
```

参考 `web_user/public/config.example.js`。

`index.html` 已加载 `/config.js`。

## Cloudflare

- **www.主域**：代理 → 面板 Origin（API / 管理后缀 / 订阅）
- **每个影子域**：代理 → 仅静态 Origin（nginx/caddy 托管 `web_user/dist`）
- 影子站 **不要** 反代管理路径或数据库
- 建议对 `POST /api/v1/user/login|register|send-code` 做 CF Rate limit（见运维说明）

## 对外文案（示例）

> 合法用户中心入口：`https://a.com`、`https://b.net`（列表以官网公告为准）。  
> **订阅链接以客户端内已保存的 www 地址为准，请勿手动改域名。**  
> 更换入口只需重新打开用户中心登录，无需更新订阅。

## 安全注意

| 项 | 说明 |
|----|------|
| 禁止 `*` Origin | 后台会拒绝通配 |
| 管理端 | 仅 www + 隐藏后缀（+ 建议 CF Access） |
| 仿冒站 | CORS 不放行未登记域；仍建议公告合法列表 |
| 限速 | 在 www API；与影子数量无关 |

## 验收

1. 影子域打开 → 登录/注册成功（浏览器 Network 可见 API 打到 www）  
2. 未加入白名单的域名 → 浏览器 CORS 失败 / 预检 403  
3. 支付完成能回到影子域订单结果页（或默认回 www，取决于 return_url）  
4. 订阅链接 host 仍为 www，客户端可更新节点  
5. 管理后缀与 `/api/v1/admin` 仅在 www 可用  
