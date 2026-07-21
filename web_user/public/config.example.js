// Copy to config.js on each shadow-portal static host (same dist, different domain).
// MUST point at the panel API host (www) — never at this shadow domain itself.
// Also add this origin to Admin → 系统设置 → 允许的用户端域名.
//
// window.__K2_API_BASE__ = 'https://www.example.com';
//
// Optional: Crisp Website ID for THIS deployment only (Crisp 后台 → 网站设置).
// If unset, online chat FAB will not load. Do NOT commit your real ID into git.
// window.__K2_CRISP_WEBSITE_ID__ = 'xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx';
